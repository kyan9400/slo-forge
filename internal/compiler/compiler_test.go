package compiler

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kyan9400/slo-forge/internal/spec"
	"gopkg.in/yaml.v3"
)

const testDocument = `apiVersion: sloforge.dev/v1alpha1
kind: ServiceLevelObjective
metadata:
  name: availability
  labels:
    team: checkout
    tier: "1"
spec:
  service: checkout-api
  description: Successful checkout requests.
  target: 99.9
  window: 30d
  indicator:
    badQuery: sum(rate(http_requests_total{service="checkout-api",code=~"5.."}[{{ .window }}]))
    totalQuery: sum(rate(http_requests_total{service="checkout-api"}[{{ .window }}]))
  runbookURL: https://example.com/runbook
  dashboardURL: https://example.com/dashboard
`

func TestCompileProducesDeterministicArtifacts(t *testing.T) {
	document, err := spec.Load([]byte(testDocument))
	if err != nil {
		t.Fatalf("spec.Load() error = %v", err)
	}
	first, err := Compile(document)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	second, err := Compile(document)
	if err != nil {
		t.Fatalf("Compile() second error = %v", err)
	}
	if !bytes.Equal(first.PrometheusRules, second.PrometheusRules) ||
		!bytes.Equal(first.PrometheusRule, second.PrometheusRule) ||
		!bytes.Equal(first.GrafanaDashboard, second.GrafanaDashboard) ||
		!bytes.Equal(first.Explanation, second.Explanation) {
		t.Fatal("Compile() output is not deterministic")
	}
}

func TestCompileIncludesMultiWindowAlerts(t *testing.T) {
	document, err := spec.Load([]byte(testDocument))
	if err != nil {
		t.Fatalf("spec.Load() error = %v", err)
	}
	output, err := Compile(document)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	var parsed ruleFile
	if err := yaml.Unmarshal(output.PrometheusRules, &parsed); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}
	if len(parsed.Groups) != 1 {
		t.Fatalf("group count = %d, want 1", len(parsed.Groups))
	}
	if got, want := len(parsed.Groups[0].Rules), 20; got != want {
		t.Fatalf("rule count = %d, want %d", got, want)
	}
	rules := string(output.PrometheusRules)
	for _, expected := range []string{
		"alert: SLOBudgetBurnPageFast",
		"alert: SLOBudgetBurnTicketSlow",
		"slo:burn_rate:rate5m",
		"runbook_url: https://example.com/runbook",
	} {
		if !strings.Contains(rules, expected) {
			t.Errorf("Prometheus rules missing %q", expected)
		}
	}
	if strings.Contains(rules, "0.000999999") {
		t.Fatal("Prometheus rules contain a floating-point precision artifact")
	}
}

func TestCompileProducesValidDashboardJSON(t *testing.T) {
	document, err := spec.Load([]byte(testDocument))
	if err != nil {
		t.Fatalf("spec.Load() error = %v", err)
	}
	output, err := Compile(document)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	var dashboard map[string]any
	if err := json.Unmarshal(output.GrafanaDashboard, &dashboard); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if dashboard["uid"] != "checkout-api-availability" {
		t.Fatalf("dashboard uid = %v", dashboard["uid"])
	}
	if !strings.Contains(string(output.Explanation), "43m 12s") {
		t.Fatalf("explanation missing calculated error budget: %s", output.Explanation)
	}
}
