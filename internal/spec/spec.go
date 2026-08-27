package spec

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	APIVersion = "sloforge.dev/v1alpha1"
	Kind       = "ServiceLevelObjective"
	Window     = "30d"
)

var dnsLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

type Document struct {
	APIVersion string   `yaml:"apiVersion" json:"apiVersion"`
	Kind       string   `yaml:"kind" json:"kind"`
	Metadata   Metadata `yaml:"metadata" json:"metadata"`
	Spec       SLOSpec  `yaml:"spec" json:"spec"`
}

type Metadata struct {
	Name   string            `yaml:"name" json:"name"`
	Labels map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
}

type SLOSpec struct {
	Service      string    `yaml:"service" json:"service"`
	Description  string    `yaml:"description,omitempty" json:"description,omitempty"`
	Target       float64   `yaml:"target" json:"target"`
	Window       string    `yaml:"window" json:"window"`
	Indicator    Indicator `yaml:"indicator" json:"indicator"`
	RunbookURL   string    `yaml:"runbookURL" json:"runbookURL"`
	DashboardURL string    `yaml:"dashboardURL,omitempty" json:"dashboardURL,omitempty"`
}

type Indicator struct {
	BadQuery   string `yaml:"badQuery" json:"badQuery"`
	TotalQuery string `yaml:"totalQuery" json:"totalQuery"`
}

func Load(data []byte) (Document, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	var document Document
	if err := decoder.Decode(&document); err != nil {
		return Document{}, fmt.Errorf("decode SLO: %w", err)
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Document{}, errors.New("decode SLO: multiple YAML documents are not supported")
		}
		return Document{}, fmt.Errorf("decode SLO: %w", err)
	}

	if problems := document.Validate(); len(problems) > 0 {
		return Document{}, ValidationError{Problems: problems}
	}
	return document, nil
}

func (d Document) Validate() []string {
	var problems []string
	if d.APIVersion != APIVersion {
		problems = append(problems, fmt.Sprintf("apiVersion must be %q", APIVersion))
	}
	if d.Kind != Kind {
		problems = append(problems, fmt.Sprintf("kind must be %q", Kind))
	}
	if !dnsLabel.MatchString(d.Metadata.Name) {
		problems = append(problems, "metadata.name must be a lowercase DNS label")
	}
	if !dnsLabel.MatchString(d.Spec.Service) {
		problems = append(problems, "spec.service must be a lowercase DNS label")
	}
	if strings.TrimSpace(d.Metadata.Labels["team"]) == "" {
		problems = append(problems, "metadata.labels.team is required")
	}
	if d.Spec.Target <= 0 || d.Spec.Target >= 100 {
		problems = append(problems, "spec.target must be greater than 0 and less than 100")
	}
	if d.Spec.Window != Window {
		problems = append(problems, fmt.Sprintf("spec.window must be %q in v1alpha1", Window))
	}
	validateQuery := func(field, query string) {
		if strings.TrimSpace(query) == "" {
			problems = append(problems, field+" is required")
			return
		}
		if !strings.Contains(query, "{{ .window }}") {
			problems = append(problems, field+" must contain the {{ .window }} placeholder")
		}
	}
	validateQuery("spec.indicator.badQuery", d.Spec.Indicator.BadQuery)
	validateQuery("spec.indicator.totalQuery", d.Spec.Indicator.TotalQuery)
	validateHTTPS := func(field, raw string, required bool) {
		if raw == "" && !required {
			return
		}
		parsed, err := url.ParseRequestURI(raw)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			problems = append(problems, field+" must be an absolute HTTPS URL")
		}
	}
	validateHTTPS("spec.runbookURL", d.Spec.RunbookURL, true)
	validateHTTPS("spec.dashboardURL", d.Spec.DashboardURL, false)

	for key := range d.Metadata.Labels {
		if strings.TrimSpace(key) == "" {
			problems = append(problems, "metadata.labels cannot contain an empty key")
		}
	}
	sort.Strings(problems)
	return problems
}

type ValidationError struct {
	Problems []string
}

func (e ValidationError) Error() string {
	return "invalid SLO: " + strings.Join(e.Problems, "; ")
}
