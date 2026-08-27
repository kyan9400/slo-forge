package spec

import (
	"errors"
	"strings"
	"testing"
)

const validDocument = `apiVersion: sloforge.dev/v1alpha1
kind: ServiceLevelObjective
metadata:
  name: availability
  labels:
    team: checkout
spec:
  service: checkout-api
  description: Successful checkout requests.
  target: 99.9
  window: 30d
  indicator:
    badQuery: sum(rate(http_requests_total{service="checkout-api",code=~"5.."}[{{ .window }}]))
    totalQuery: sum(rate(http_requests_total{service="checkout-api"}[{{ .window }}]))
  runbookURL: https://example.com/runbook
`

func TestLoadValidDocument(t *testing.T) {
	document, err := Load([]byte(validDocument))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if document.Metadata.Name != "availability" {
		t.Fatalf("metadata.name = %q", document.Metadata.Name)
	}
	if document.Spec.Target != 99.9 {
		t.Fatalf("spec.target = %f", document.Spec.Target)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	_, err := Load([]byte(validDocument + "  mystery: true\n"))
	if err == nil || !strings.Contains(err.Error(), "field mystery not found") {
		t.Fatalf("Load() error = %v, want unknown field error", err)
	}
}

func TestLoadRejectsMultipleDocuments(t *testing.T) {
	_, err := Load([]byte(validDocument + "---\n" + validDocument))
	if err == nil || !strings.Contains(err.Error(), "multiple YAML documents") {
		t.Fatalf("Load() error = %v, want multiple document error", err)
	}
}

func TestValidationReportsAllProblems(t *testing.T) {
	document := Document{
		APIVersion: "wrong",
		Kind:       "Wrong",
		Metadata:   Metadata{Name: "Not Valid"},
		Spec: SLOSpec{
			Service:    "",
			Target:     100,
			Window:     "7d",
			Indicator:  Indicator{BadQuery: "up", TotalQuery: ""},
			RunbookURL: "http://example.com",
		},
	}
	problems := document.Validate()
	if len(problems) != 10 {
		t.Fatalf("Validate() returned %d problems: %v", len(problems), problems)
	}
	if !strings.Contains(strings.Join(problems, "\n"), "metadata.labels.team") {
		t.Fatalf("Validate() problems = %v, want team ownership problem", problems)
	}
	_, err := Load([]byte("apiVersion: wrong\nkind: Wrong\n"))
	var validationError ValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf("Load() error = %T, want ValidationError", err)
	}
}
