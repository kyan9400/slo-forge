package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const validRequest = `apiVersion: sloforge.dev/v1alpha1
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
    badQuery: sum(rate(http_requests_total{code=~"5.."}[{{ .window }}]))
    totalQuery: sum(rate(http_requests_total[{{ .window }}]))
  runbookURL: https://example.com/runbook
`

func TestHealthAndMetrics(t *testing.T) {
	api := New("test")
	server := httptest.NewServer(api.Handler())
	defer server.Close()

	response, err := http.Get(server.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz error = %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /healthz status = %d", response.StatusCode)
	}

	response, err = http.Get(server.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics error = %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics status = %d", response.StatusCode)
	}
}

func TestRender(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/v1/render", strings.NewReader(validRequest))
	request.Header.Set("Content-Type", "application/yaml")
	recorder := httptest.NewRecorder()
	New("test").Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("POST /v1/render status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !strings.Contains(response["prometheusRules"], "SLOBudgetBurnPageFast") {
		t.Fatal("render response missing alert rules")
	}
	if !strings.Contains(response["grafanaDashboard"], "checkout-api") {
		t.Fatal("render response missing dashboard")
	}
}

func TestRenderRejectsInvalidInput(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/v1/render", strings.NewReader("kind: Wrong\n"))
	request.Header.Set("Content-Type", "application/yaml")
	recorder := httptest.NewRecorder()
	New("test").Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("POST /v1/render status = %d, want %d", recorder.Code, http.StatusUnprocessableEntity)
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json", contentType)
	}
}
