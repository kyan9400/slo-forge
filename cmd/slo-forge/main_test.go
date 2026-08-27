package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const cliDocument = `apiVersion: sloforge.dev/v1alpha1
kind: ServiceLevelObjective
metadata:
  name: availability
  labels:
    team: checkout
spec:
  service: checkout-api
  target: 99.9
  window: 30d
  indicator:
    badQuery: sum(rate(requests_total{status="error"}[{{ .window }}]))
    totalQuery: sum(rate(requests_total[{{ .window }}]))
  runbookURL: https://example.com/runbook
`

func TestRenderCommand(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "slo.yaml")
	if err := os.WriteFile(input, []byte(cliDocument), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	outputDirectory := filepath.Join(directory, "generated")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run([]string{"render", "-f", input, "-o", outputDirectory}, &stdout, &stderr); err != nil {
		t.Fatalf("run(render) error = %v, stderr = %s", err, stderr.String())
	}
	for _, name := range []string{"prometheus-rules.yaml", "prometheusrule.yaml", "grafana-dashboard.json", "explanation.md"} {
		if _, err := os.Stat(filepath.Join(outputDirectory, name)); err != nil {
			t.Errorf("generated %s: %v", name, err)
		}
	}
	if !strings.Contains(stdout.String(), "rendered 4 artifacts") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestVersionCommand(t *testing.T) {
	var stdout bytes.Buffer
	if err := run([]string{"version"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("run(version) error = %v", err)
	}
	if strings.TrimSpace(stdout.String()) != version {
		t.Fatalf("version output = %q", stdout.String())
	}
}
