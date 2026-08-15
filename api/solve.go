package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"

	"backpack-brawl-solver/pkg/websolve"
)

const (
	defaultRemoteMaxNodes = int64(10_000_000)
	hardRemoteMaxNodes    = int64(50_000_000)
	maxRequestBytes       = int64(16 << 20)
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.Header().Set("Allow", "POST, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "request body is too large")
		return
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		writeError(w, http.StatusBadRequest, "request body is required")
		return
	}

	workers := envInt("SOLVER_REMOTE_WORKERS", runtime.NumCPU())
	if workers < 1 {
		workers = 1
	}
	defaultMaxNodes := envInt64("SOLVER_REMOTE_DEFAULT_MAX_NODES", defaultRemoteMaxNodes)
	maxNodesCap := envInt64("SOLVER_REMOTE_MAX_NODES", hardRemoteMaxNodes)

	result, err := websolve.SolveScenarioJSONWithOptions(body, websolve.Options{
		WorkerOverride:  workers,
		DefaultMaxNodes: defaultMaxNodes,
		MaxNodesCap:     maxNodesCap,
		Backend:         "vercel-go",
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Solver-Backend", result.Metadata.Backend)
	w.Header().Set("X-Solver-Server-Ms", strconv.FormatInt(result.Metadata.ServerElapsedMS, 10))
	w.Header().Set("X-Solver-Workers", strconv.Itoa(result.Metadata.Workers))
	w.Header().Set("X-Solver-Max-Nodes-Applied", strconv.FormatInt(result.Metadata.MaxNodesApplied, 10))
	if result.Metadata.MaxNodesCapped {
		w.Header().Set("X-Solver-Max-Nodes-Capped", "true")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(result.JSON)
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}

func envInt64(key string, fallback int64) int64 {
	value := strings.TrimSpace(getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}

var getenv = os.Getenv

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprint(message)})
}
