package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandlerSolvesScenario(t *testing.T) {
	t.Setenv("SOLVER_REMOTE_WORKERS", "2")
	t.Setenv("SOLVER_REMOTE_DEFAULT_MAX_NODES", "1234")
	t.Setenv("SOLVER_REMOTE_MAX_NODES", "5000")

	body := testPayload(t, `{
		"items": {
			"scalemail": 1
		},
		"top": 1,
		"workers": 1,
		"max_nodes": 0
	}`)

	request := httptest.NewRequest(http.MethodPost, "/api/solve", strings.NewReader(body))
	response := httptest.NewRecorder()
	Handler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if response.Header().Get("X-Solver-Backend") != "vercel-go" {
		t.Fatalf("missing backend header: %v", response.Header())
	}
	if response.Header().Get("X-Solver-Workers") != "2" {
		t.Fatalf("expected worker override header, got %q", response.Header().Get("X-Solver-Workers"))
	}
	if response.Header().Get("X-Solver-Max-Nodes-Applied") != "1234" {
		t.Fatalf("expected remote max nodes header, got %q", response.Header().Get("X-Solver-Max-Nodes-Applied"))
	}

	var solutions []struct {
		Search struct {
			Backend         string `json:"backend"`
			RemoteWorkers   int    `json:"remote_workers"`
			MaxNodesApplied int64  `json:"max_nodes_applied"`
		} `json:"search"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &solutions); err != nil {
		t.Fatalf("invalid response JSON: %v\n%s", err, response.Body.String())
	}
	if len(solutions) == 0 {
		t.Fatal("expected at least one solution")
	}
	if solutions[0].Search.Backend != "vercel-go" || solutions[0].Search.RemoteWorkers != 2 || solutions[0].Search.MaxNodesApplied != 1234 {
		t.Fatalf("unexpected search metadata: %+v", solutions[0].Search)
	}
}

func TestHandlerRejectsInvalidPayload(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/solve", strings.NewReader(`{"scenario":{}}`))
	response := httptest.NewRecorder()
	Handler(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", response.Code)
	}
	var payload map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid error JSON: %v", err)
	}
	if payload["error"] == "" {
		t.Fatalf("expected error message, got %+v", payload)
	}
}

func TestHandlerPropagatesCanceledRequestContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest(http.MethodPost, "/api/solve", strings.NewReader(testPayload(t, `{
		"items": {"scalemail": 1},
		"top": 1,
		"max_nodes": 1
	}`))).WithContext(ctx)
	response := httptest.NewRecorder()

	Handler(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected canceled request to fail, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), context.Canceled.Error()) {
		t.Fatalf("expected cancellation error, got %s", response.Body.String())
	}
}

func testPayload(t *testing.T, scenario string) string {
	t.Helper()
	catalogContent, err := os.ReadFile(filepath.Join("..", "data", "catalog.json"))
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	input, err := json.Marshal(map[string]json.RawMessage{
		"catalog":  catalogContent,
		"scenario": json.RawMessage(scenario),
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	return string(input)
}
