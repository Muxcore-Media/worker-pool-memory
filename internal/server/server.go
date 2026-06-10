package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Muxcore-Media/worker-pool-memory/internal/taskqueue"
)

// Server provides the HTTP API for the worker pool.
type Server struct {
	queue *taskqueue.Queue
	mux   *http.ServeMux
}

// New creates an HTTP server backed by the task queue.
func New(q *taskqueue.Queue) *Server {
	s := &Server{
		queue: q,
		mux:   http.NewServeMux(),
	}
	s.mux.HandleFunc("/submit", s.handleSubmit)
	s.mux.HandleFunc("/status/", s.handleStatus)
	s.mux.HandleFunc("/cancel/", s.handleCancel)
	s.mux.HandleFunc("/list", s.handleList)
	s.mux.HandleFunc("/assign/", s.handleAssign)
	s.mux.HandleFunc("/complete/", s.handleComplete)
	s.mux.HandleFunc("/fail/", s.handleFail)
	s.mux.HandleFunc("/reassign/", s.handleReassign)
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/metrics", s.handleMetrics)
	return s
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	stats := s.queue.Stats()
	fmt.Fprintf(w, "# HELP worker_pool_tasks_pending Number of pending tasks\n")
	fmt.Fprintf(w, "# TYPE worker_pool_tasks_pending gauge\n")
	fmt.Fprintf(w, "worker_pool_tasks_pending %d\n", stats["pending"])
	fmt.Fprintf(w, "# HELP worker_pool_tasks_running Number of running tasks\n")
	fmt.Fprintf(w, "# TYPE worker_pool_tasks_running gauge\n")
	fmt.Fprintf(w, "worker_pool_tasks_running %d\n", stats["running"])
	fmt.Fprintf(w, "# HELP worker_pool_tasks_completed_total Total completed tasks\n")
	fmt.Fprintf(w, "# TYPE worker_pool_tasks_completed_total counter\n")
	fmt.Fprintf(w, "worker_pool_tasks_completed_total %d\n", stats["completed"])
	fmt.Fprintf(w, "# HELP worker_pool_tasks_failed_total Total failed tasks\n")
	fmt.Fprintf(w, "# TYPE worker_pool_tasks_failed_total counter\n")
	fmt.Fprintf(w, "worker_pool_tasks_failed_total %d\n", stats["failed"])
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	return s.mux
}

type submitRequest struct {
	Type           string         `json:"type"`
	Payload        []byte         `json:"payload,omitempty"`
	MaxRetries     int            `json:"max_retries"`
	Capabilities   []string       `json:"capabilities,omitempty"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	Meta           map[string]any `json:"meta,omitempty"`
}

func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req submitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Type == "" {
		writeError(w, http.StatusBadRequest, "type is required")
		return
	}

	id, err := s.queue.Submit(req.Type, req.Payload, req.MaxRetries, req.Capabilities, req.IdempotencyKey, req.Meta)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"task_id": id})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	id := r.URL.Path[len("/status/"):]
	task, err := s.queue.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	id := r.URL.Path[len("/cancel/"):]
	if err := s.queue.Cancel(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	statusFilter := r.URL.Query().Get("status")
	typeFilter := r.URL.Query().Get("type")
	tasks := s.queue.List(statusFilter, typeFilter)
	writeJSON(w, http.StatusOK, tasks)
}

func (s *Server) handleAssign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	id := r.URL.Path[len("/assign/"):]
	var req struct {
		NodeID string `json:"node_id"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.NodeID == "" {
		writeError(w, http.StatusBadRequest, "node_id is required")
		return
	}
	if err := s.queue.Assign(id, req.NodeID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "assigned"})
}

func (s *Server) handleComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	id := r.URL.Path[len("/complete/"):]
	if err := s.queue.Complete(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "completed"})
}

func (s *Server) handleFail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	id := r.URL.Path[len("/fail/"):]
	var req struct {
		Error string `json:"error"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if err := s.queue.Fail(id, req.Error); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "failed"})
}

func (s *Server) handleReassign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	id := r.URL.Path[len("/reassign/"):]
	if err := s.queue.Reassign(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reassigned"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

