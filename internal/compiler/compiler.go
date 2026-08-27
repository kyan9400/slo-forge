package compiler

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/kyan9400/slo-forge/internal/spec"
	"gopkg.in/yaml.v3"
)

type Output struct {
	PrometheusRules  []byte
	PrometheusRule   []byte
	GrafanaDashboard []byte
	Explanation      []byte
}

type ruleFile struct {
	Groups []ruleGroup `yaml:"groups"`
}

type prometheusRule struct {
	APIVersion string             `yaml:"apiVersion"`
	Kind       string             `yaml:"kind"`
	Metadata   kubernetesMetadata `yaml:"metadata"`
	Spec       ruleFile           `yaml:"spec"`
}

type kubernetesMetadata struct {
	Name   string            `yaml:"name"`
	Labels map[string]string `yaml:"labels"`
}

type ruleGroup struct {
	Name     string `yaml:"name"`
	Interval string `yaml:"interval"`
	Rules    []rule `yaml:"rules"`
}

type rule struct {
	Record      string            `yaml:"record,omitempty"`
	Alert       string            `yaml:"alert,omitempty"`
	Expr        string            `yaml:"expr"`
	For         string            `yaml:"for,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

type alertProfile struct {
	Name            string
	LongWindow      string
	ShortWindow     string
	BurnRate        float64
	For             string
	Severity        string
	BudgetConsumed  int
	DetectionWindow string
}

var recordingWindows = []string{"5m", "30m", "1h", "2h", "6h", "1d", "3d", "30d"}

var alertProfiles = []alertProfile{
	{Name: "SLOBudgetBurnPageFast", LongWindow: "1h", ShortWindow: "5m", BurnRate: 14.4, For: "2m", Severity: "page", BudgetConsumed: 2, DetectionWindow: "1 hour"},
	{Name: "SLOBudgetBurnPageSlow", LongWindow: "6h", ShortWindow: "30m", BurnRate: 6, For: "15m", Severity: "page", BudgetConsumed: 5, DetectionWindow: "6 hours"},
	{Name: "SLOBudgetBurnTicketFast", LongWindow: "1d", ShortWindow: "2h", BurnRate: 3, For: "1h", Severity: "ticket", BudgetConsumed: 10, DetectionWindow: "1 day"},
	{Name: "SLOBudgetBurnTicketSlow", LongWindow: "3d", ShortWindow: "6h", BurnRate: 1, For: "3h", Severity: "ticket", BudgetConsumed: 10, DetectionWindow: "3 days"},
}

func Compile(document spec.Document) (Output, error) {
	if problems := document.Validate(); len(problems) > 0 {
		return Output{}, spec.ValidationError{Problems: problems}
	}

	group := buildRuleGroup(document)
	native, err := yaml.Marshal(ruleFile{Groups: []ruleGroup{group}})
	if err != nil {
		return Output{}, fmt.Errorf("marshal Prometheus rules: %w", err)
	}

	operator := prometheusRule{
		APIVersion: "monitoring.coreos.com/v1",
		Kind:       "PrometheusRule",
		Metadata: kubernetesMetadata{
			Name: document.Spec.Service + "-" + document.Metadata.Name,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "slo-forge",
				"slo-forge.dev/service":        document.Spec.Service,
			},
		},
		Spec: ruleFile{Groups: []ruleGroup{group}},
	}
	operatorYAML, err := yaml.Marshal(operator)
	if err != nil {
		return Output{}, fmt.Errorf("marshal PrometheusRule: %w", err)
	}

	dashboard, err := json.MarshalIndent(buildDashboard(document), "", "  ")
	if err != nil {
		return Output{}, fmt.Errorf("marshal Grafana dashboard: %w", err)
	}
	dashboard = append(dashboard, '\n')

	return Output{
		PrometheusRules:  native,
		PrometheusRule:   operatorYAML,
		GrafanaDashboard: dashboard,
		Explanation:      []byte(buildExplanation(document)),
	}, nil
}

func buildRuleGroup(document spec.Document) ruleGroup {
	baseLabels := map[string]string{
		"service": document.Spec.Service,
		"slo":     document.Metadata.Name,
		"team":    document.Metadata.Labels["team"],
	}
	budgetRatio := errorBudgetRatio(document.Spec.Target)
	rules := make([]rule, 0, len(recordingWindows)*2+len(alertProfiles)+1)

	for _, window := range recordingWindows {
		labels := cloneLabels(baseLabels)
		rules = append(rules, rule{
			Record: "slo:bad_ratio:rate" + window,
			Expr:   badRatioExpression(document, window),
			Labels: labels,
		})
		if window != "30d" {
			rules = append(rules, rule{
				Record: "slo:burn_rate:rate" + window,
				Expr: fmt.Sprintf(
					`slo:bad_ratio:rate%s{service=%q,slo=%q} / %s`,
					window,
					document.Spec.Service,
					document.Metadata.Name,
					formatFloat(budgetRatio),
				),
				Labels: cloneLabels(baseLabels),
			})
		}
	}

	rules = append(rules, rule{
		Record: "slo:error_budget:remaining",
		Expr: fmt.Sprintf(
			`1 - (slo:bad_ratio:rate30d{service=%q,slo=%q} / %s)`,
			document.Spec.Service,
			document.Metadata.Name,
			formatFloat(budgetRatio),
		),
		Labels: cloneLabels(baseLabels),
	})

	for _, profile := range alertProfiles {
		labels := cloneLabels(baseLabels)
		labels["severity"] = profile.Severity
		labels["window_pair"] = profile.LongWindow + "_" + profile.ShortWindow
		annotations := map[string]string{
			"summary": fmt.Sprintf("%s is burning its %s SLO budget too quickly", document.Spec.Service, document.Metadata.Name),
			"description": fmt.Sprintf(
				"The %s and %s windows are both above a %.1fx burn rate. Approximately %d%% of the 30-day error budget can be consumed in %s.",
				profile.LongWindow,
				profile.ShortWindow,
				profile.BurnRate,
				profile.BudgetConsumed,
				profile.DetectionWindow,
			),
			"runbook_url": document.Spec.RunbookURL,
		}
		if document.Spec.DashboardURL != "" {
			annotations["dashboard_url"] = document.Spec.DashboardURL
		}
		rules = append(rules, rule{
			Alert: profile.Name,
			Expr: fmt.Sprintf(
				`(slo:burn_rate:rate%s{service=%q,slo=%q} > %s) and (slo:burn_rate:rate%s{service=%q,slo=%q} > %s)`,
				profile.LongWindow,
				document.Spec.Service,
				document.Metadata.Name,
				formatFloat(profile.BurnRate),
				profile.ShortWindow,
				document.Spec.Service,
				document.Metadata.Name,
				formatFloat(profile.BurnRate),
			),
			For:         profile.For,
			Labels:      labels,
			Annotations: annotations,
		})
	}

	return ruleGroup{
		Name:     "slo-forge." + document.Spec.Service + "." + document.Metadata.Name,
		Interval: "30s",
		Rules:    rules,
	}
}

func badRatioExpression(document spec.Document, window string) string {
	bad := strings.ReplaceAll(document.Spec.Indicator.BadQuery, "{{ .window }}", window)
	total := strings.ReplaceAll(document.Spec.Indicator.TotalQuery, "{{ .window }}", window)
	return fmt.Sprintf("(%s) / clamp_min((%s), 1)", bad, total)
}

func cloneLabels(source map[string]string) map[string]string {
	copy := make(map[string]string, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}

func formatFloat(value float64) string {
	return fmt.Sprintf("%g", value)
}

func buildDashboard(document spec.Document) map[string]any {
	selector := fmt.Sprintf(`service=%q,slo=%q`, document.Spec.Service, document.Metadata.Name)
	title := fmt.Sprintf("%s · %s SLO", document.Spec.Service, document.Metadata.Name)
	return map[string]any{
		"annotations":  map[string]any{"list": []any{}},
		"editable":     true,
		"graphTooltip": 1,
		"panels": []any{
			map[string]any{
				"datasource":  map[string]any{"type": "prometheus", "uid": "${datasource}"},
				"fieldConfig": map[string]any{"defaults": map[string]any{"decimals": 3, "max": 100, "min": 0, "unit": "percent"}},
				"gridPos":     map[string]any{"h": 8, "w": 8, "x": 0, "y": 0},
				"id":          1, "title": "30-day availability", "type": "stat",
				"targets": []any{map[string]any{"expr": fmt.Sprintf(`(1 - slo:bad_ratio:rate30d{%s}) * 100`, selector), "refId": "A"}},
			},
			map[string]any{
				"datasource":  map[string]any{"type": "prometheus", "uid": "${datasource}"},
				"fieldConfig": map[string]any{"defaults": map[string]any{"decimals": 1, "max": 100, "min": -100, "unit": "percent"}},
				"gridPos":     map[string]any{"h": 8, "w": 8, "x": 8, "y": 0},
				"id":          2, "title": "Error budget remaining", "type": "gauge",
				"targets": []any{map[string]any{"expr": fmt.Sprintf(`slo:error_budget:remaining{%s} * 100`, selector), "refId": "A"}},
			},
			map[string]any{
				"datasource":  map[string]any{"type": "prometheus", "uid": "${datasource}"},
				"fieldConfig": map[string]any{"defaults": map[string]any{"decimals": 2, "unit": "short"}},
				"gridPos":     map[string]any{"h": 8, "w": 8, "x": 16, "y": 0},
				"id":          3, "title": "Current burn rate", "type": "stat",
				"targets": []any{map[string]any{"expr": fmt.Sprintf(`slo:burn_rate:rate1h{%s}`, selector), "refId": "A"}},
			},
			map[string]any{
				"datasource":  map[string]any{"type": "prometheus", "uid": "${datasource}"},
				"fieldConfig": map[string]any{"defaults": map[string]any{"decimals": 2, "unit": "short"}},
				"gridPos":     map[string]any{"h": 10, "w": 24, "x": 0, "y": 8},
				"id":          4, "title": "Multi-window burn rate", "type": "timeseries",
				"targets": []any{
					map[string]any{"expr": fmt.Sprintf(`slo:burn_rate:rate5m{%s}`, selector), "legendFormat": "5m", "refId": "A"},
					map[string]any{"expr": fmt.Sprintf(`slo:burn_rate:rate1h{%s}`, selector), "legendFormat": "1h", "refId": "B"},
					map[string]any{"expr": fmt.Sprintf(`slo:burn_rate:rate6h{%s}`, selector), "legendFormat": "6h", "refId": "C"},
					map[string]any{"expr": fmt.Sprintf(`slo:burn_rate:rate3d{%s}`, selector), "legendFormat": "3d", "refId": "D"},
				},
			},
		},
		"refresh":       "30s",
		"schemaVersion": 39,
		"tags":          []string{"slo", "sre", "slo-forge", document.Spec.Service},
		"templating": map[string]any{"list": []any{map[string]any{
			"current": map[string]any{"text": "Prometheus", "value": "Prometheus"},
			"label":   "Prometheus", "name": "datasource", "query": "prometheus", "type": "datasource",
		}}},
		"time":     map[string]any{"from": "now-30d", "to": "now"},
		"timezone": "browser",
		"title":    title,
		"uid":      document.Spec.Service + "-" + document.Metadata.Name,
		"version":  1,
	}
}

func buildExplanation(document spec.Document) string {
	budgetRatio := errorBudgetRatio(document.Spec.Target)
	allowed := time.Duration(math.Round(float64(30*24*time.Hour) * budgetRatio))
	labels := make([]string, 0, len(document.Metadata.Labels))
	for key := range document.Metadata.Labels {
		labels = append(labels, key)
	}
	sort.Strings(labels)
	var labelPairs []string
	for _, key := range labels {
		labelPairs = append(labelPairs, fmt.Sprintf("`%s=%s`", key, document.Metadata.Labels[key]))
	}

	var builder strings.Builder
	fmt.Fprintf(&builder, "# %s / %s\n\n", document.Spec.Service, document.Metadata.Name)
	if document.Spec.Description != "" {
		fmt.Fprintf(&builder, "%s\n\n", document.Spec.Description)
	}
	fmt.Fprintf(&builder, "- Objective: **%.4g%%** over a rolling **30-day** window\n", document.Spec.Target)
	fmt.Fprintf(&builder, "- Error budget: **%.4g%%** (approximately **%s** of bad time per 30 days)\n", budgetRatio*100, humanDuration(allowed))
	fmt.Fprintf(&builder, "- Ownership: %s\n", strings.Join(labelPairs, ", "))
	fmt.Fprintf(&builder, "- Runbook: %s\n\n", document.Spec.RunbookURL)
	builder.WriteString("## Alert policy\n\n")
	builder.WriteString("| Route | Long window | Short window | Burn rate | Hold |\n")
	builder.WriteString("|---|---:|---:|---:|---:|\n")
	for _, profile := range alertProfiles {
		fmt.Fprintf(&builder, "| %s | %s | %s | %.1fx | %s |\n", profile.Severity, profile.LongWindow, profile.ShortWindow, profile.BurnRate, profile.For)
	}
	builder.WriteString("\nBoth the long and short windows must breach before an alert fires. This keeps detection fast while filtering brief spikes.\n\n")
	builder.WriteString("## Generated artifacts\n\n")
	builder.WriteString("- `prometheus-rules.yaml`: native recording and alerting rules.\n")
	builder.WriteString("- `prometheusrule.yaml`: the same rules packaged for Prometheus Operator.\n")
	builder.WriteString("- `grafana-dashboard.json`: an importable overview of availability and burn rate.\n")
	builder.WriteString("- `explanation.md`: this reviewable policy summary.\n")
	return builder.String()
}

func humanDuration(duration time.Duration) string {
	if duration < time.Hour {
		minutes := int(duration / time.Minute)
		seconds := int((duration % time.Minute) / time.Second)
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	hours := int(duration / time.Hour)
	minutes := int((duration % time.Hour) / time.Minute)
	seconds := int((duration % time.Minute) / time.Second)
	return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
}

func errorBudgetRatio(targetPercent float64) float64 {
	const precision = 1e12
	return math.Round(((100-targetPercent)/100)*precision) / precision
}
