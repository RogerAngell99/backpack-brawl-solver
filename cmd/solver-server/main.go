package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"backpack-brawl-solver/internal/render"
	"backpack-brawl-solver/internal/solver"
	"backpack-brawl-solver/pkg/websolve"
)

const (
	statusRunning  = "running"
	statusDone     = "done"
	statusError    = "error"
	statusCanceled = "canceled"

	defaultMaxBodyBytes = int64(16 << 20)
)

type server struct {
	authToken     string
	allowedOrigin map[string]bool
	workerCount   int
	maxBodyBytes  int64

	mu         sync.Mutex
	jobs       map[string]*job
	activeJobs int
}

type job struct {
	ID               string
	Status           string
	CreatedAt        time.Time
	StartedAt        time.Time
	FinishedAt       time.Time
	Progress         *progressPayload
	Solutions        json.RawMessage
	PartialSolutions json.RawMessage
	Error            string
	Metadata         *websolve.Metadata

	cancel context.CancelFunc
}

type progressPayload struct {
	Phase          string  `json:"phase"`
	NodesExplored  int64   `json:"nodes_explored"`
	NodesTotal     int64   `json:"nodes_total,omitempty"`
	Percent        float64 `json:"percent,omitempty"`
	ElapsedMS      int64   `json:"elapsed_ms"`
	NodesPerSecond float64 `json:"nodes_per_second,omitempty"`
	EtaMS          int64   `json:"eta_ms,omitempty"`
}

type jobResponse struct {
	ID               string             `json:"id"`
	Status           string             `json:"status"`
	CreatedAt        string             `json:"created_at"`
	StartedAt        string             `json:"started_at,omitempty"`
	FinishedAt       string             `json:"finished_at,omitempty"`
	Progress         *progressPayload   `json:"progress,omitempty"`
	Solutions        json.RawMessage    `json:"solutions,omitempty"`
	PartialSolutions json.RawMessage    `json:"partial_solutions,omitempty"`
	Error            string             `json:"error,omitempty"`
	Metadata         *websolve.Metadata `json:"metadata,omitempty"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func main() {
	addr := flag.String("addr", envString("SOLVER_ADDR", "127.0.0.1:8080"), "HTTP listen address")
	flag.Parse()

	srv := &server{
		authToken:     os.Getenv("SOLVER_AUTH_TOKEN"),
		allowedOrigin: parseAllowedOrigins(envString("SOLVER_ALLOWED_ORIGINS", "https://backpack-brawl-solver.vercel.app,http://localhost:5173,http://localhost:4173")),
		workerCount:   envInt("SOLVER_REMOTE_WORKERS", runtime.NumCPU()),
		maxBodyBytes:  envInt64("SOLVER_MAX_BODY_BYTES", defaultMaxBodyBytes),
		jobs:          map[string]*job{},
	}
	if srv.workerCount < 1 {
		srv.workerCount = 1
	}
	if srv.authToken == "" {
		log.Printf("warning: SOLVER_AUTH_TOKEN is empty; solve endpoints are unauthenticated")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", srv.handleHealth)
	mux.HandleFunc("/api/jobs", srv.handleJobs)
	mux.HandleFunc("/api/jobs/", srv.handleJob)

	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           srv.withCORS(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("solver-server listening on %s with %d worker(s)", *addr, srv.workerCount)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func (srv *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"backend": "oci-vm",
		"workers": srv.workerCount,
	})
}

func (srv *server) handleJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !srv.authorized(r) {
		writeError(w, http.StatusUnauthorized, "missing or invalid solver token")
		return
	}

	srv.mu.Lock()
	if srv.activeJobs >= 1 {
		srv.mu.Unlock()
		writeError(w, http.StatusConflict, "another solve is already running")
		return
	}
	srv.activeJobs++
	srv.mu.Unlock()

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, srv.maxBodyBytes))
	if err != nil {
		srv.mu.Lock()
		srv.activeJobs--
		srv.mu.Unlock()
		writeError(w, http.StatusRequestEntityTooLarge, "request body is too large")
		return
	}
	if len(body) == 0 {
		srv.mu.Lock()
		srv.activeJobs--
		srv.mu.Unlock()
		writeError(w, http.StatusBadRequest, "request body is required")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	jobID, err := randomID()
	if err != nil {
		cancel()
		srv.mu.Lock()
		srv.activeJobs--
		srv.mu.Unlock()
		writeError(w, http.StatusInternalServerError, "could not create job id")
		return
	}
	nextJob := &job{
		ID:        jobID,
		Status:    statusRunning,
		CreatedAt: time.Now().UTC(),
		StartedAt: time.Now().UTC(),
		cancel:    cancel,
		Progress: &progressPayload{
			Phase:     solver.ProgressPhaseSearch,
			ElapsedMS: 0,
		},
	}
	srv.mu.Lock()
	srv.jobs[jobID] = nextJob
	srv.mu.Unlock()

	go srv.runJob(ctx, nextJob, body)
	writeJSON(w, http.StatusAccepted, srv.responseForJob(nextJob))
}

func (srv *server) handleJob(w http.ResponseWriter, r *http.Request) {
	if !srv.authorized(r) {
		writeError(w, http.StatusUnauthorized, "missing or invalid solver token")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
	if rest == "" {
		writeError(w, http.StatusNotFound, "job id is required")
		return
	}
	if strings.HasSuffix(rest, "/cancel") {
		jobID := strings.TrimSuffix(rest, "/cancel")
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		srv.cancelJob(w, jobID)
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	current, ok := srv.getJob(rest)
	if !ok {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	writeJSON(w, http.StatusOK, srv.responseForJob(current))
}

func (srv *server) runJob(ctx context.Context, current *job, body []byte) {
	defer func() {
		srv.mu.Lock()
		srv.activeJobs--
		srv.mu.Unlock()
	}()

	reporter := func(snapshot solver.ProgressSnapshot) {
		srv.mu.Lock()
		defer srv.mu.Unlock()
		if stored := srv.jobs[current.ID]; stored != nil {
			stored.Progress = progressFromSnapshot(snapshot)
			if len(snapshot.PartialSolutions) > 0 {
				if content, err := render.SolutionsJSON(snapshot.PartialSolutions); err == nil {
					stored.PartialSolutions = append(json.RawMessage(nil), content...)
				}
			}
		}
	}
	result, err := websolve.SolveScenarioJSONWithOptions(body, websolve.Options{
		ProgressReporter: reporter,
		WorkerOverride:   srv.workerCount,
		Backend:          "oci-vm",
		Context:          ctx,
	})

	srv.mu.Lock()
	defer srv.mu.Unlock()
	stored := srv.jobs[current.ID]
	if stored == nil {
		return
	}
	stored.FinishedAt = time.Now().UTC()
	stored.cancel = nil
	if err != nil {
		if errors.Is(err, context.Canceled) {
			stored.Status = statusCanceled
			stored.Error = ""
		} else {
			stored.Status = statusError
			stored.Error = err.Error()
		}
		return
	}
	stored.Status = statusDone
	stored.Solutions = append(json.RawMessage(nil), result.JSON...)
	stored.Metadata = &result.Metadata
}

func (srv *server) cancelJob(w http.ResponseWriter, jobID string) {
	srv.mu.Lock()
	current := srv.jobs[jobID]
	if current == nil {
		srv.mu.Unlock()
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	if current.Status == statusRunning && current.cancel != nil {
		current.Status = statusCanceled
		current.FinishedAt = time.Now().UTC()
		current.cancel()
	}
	response := srv.responseForJobLocked(current)
	srv.mu.Unlock()
	writeJSON(w, http.StatusOK, response)
}

func (srv *server) getJob(jobID string) (*job, bool) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	current := srv.jobs[jobID]
	if current == nil {
		return nil, false
	}
	copied := *current
	if current.Progress != nil {
		progress := *current.Progress
		copied.Progress = &progress
	}
	if current.Solutions != nil {
		copied.Solutions = append(json.RawMessage(nil), current.Solutions...)
	}
	if current.PartialSolutions != nil {
		copied.PartialSolutions = append(json.RawMessage(nil), current.PartialSolutions...)
	}
	return &copied, true
}

func (srv *server) responseForJob(current *job) jobResponse {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	return srv.responseForJobLocked(current)
}

func (srv *server) responseForJobLocked(current *job) jobResponse {
	response := jobResponse{
		ID:        current.ID,
		Status:    current.Status,
		CreatedAt: current.CreatedAt.Format(time.RFC3339Nano),
		Progress:  current.Progress,
		Error:     current.Error,
		Metadata:  current.Metadata,
	}
	if !current.StartedAt.IsZero() {
		response.StartedAt = current.StartedAt.Format(time.RFC3339Nano)
	}
	if !current.FinishedAt.IsZero() {
		response.FinishedAt = current.FinishedAt.Format(time.RFC3339Nano)
	}
	if current.Status == statusDone && len(current.Solutions) > 0 {
		response.Solutions = current.Solutions
	}
	if len(current.PartialSolutions) > 0 {
		response.PartialSolutions = current.PartialSolutions
	}
	return response
}

func (srv *server) authorized(r *http.Request) bool {
	if srv.authToken == "" {
		return true
	}
	token := r.Header.Get("X-Solver-Token")
	if token == "" {
		if value := r.Header.Get("Authorization"); strings.HasPrefix(value, "Bearer ") {
			token = strings.TrimPrefix(value, "Bearer ")
		}
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(srv.authToken)) == 1
}

func (srv *server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (srv.allowedOrigin["*"] || srv.allowedOrigin[origin]) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Solver-Token, Authorization")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func progressFromSnapshot(snapshot solver.ProgressSnapshot) *progressPayload {
	return &progressPayload{
		Phase:          snapshot.Phase,
		NodesExplored:  snapshot.NodesExplored,
		NodesTotal:     snapshot.NodesTotal,
		Percent:        snapshot.Percent,
		ElapsedMS:      snapshot.ElapsedMS,
		NodesPerSecond: snapshot.NodesPerSecond,
		EtaMS:          snapshot.EtaMS,
	}
}

func randomID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}

func parseAllowedOrigins(value string) map[string]bool {
	out := map[string]bool{}
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out[part] = true
		}
	}
	return out
}

func envString(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func envInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt64(name string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write json failed: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func (response jobResponse) String() string {
	return fmt.Sprintf("%s:%s", response.ID, response.Status)
}
