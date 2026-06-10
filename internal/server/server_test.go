package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Muxcore-Media/worker-pool-memory/internal/taskqueue"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	return New(taskqueue.New())
}

func TestSubmit(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(map[string]any{
		"type":       "transcode",
		"payload":    []byte(`{"file":"movie.mkv"}`),
		"max_retries": 3,
	})
	req := httptest.NewRequest(http.MethodPost, "/submit", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("POST /submit: %d, body: %s", w.Code, w.Body.String())
	}
	var resp struct {
		TaskID string `json:"task_id"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.TaskID == "" {
		t.Fatal("expected non-empty task_id")
	}
}

func TestSubmit_MissingType(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(map[string]any{})
	req := httptest.NewRequest(http.MethodPost, "/submit", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestStatus(t *testing.T) {
	srv := newTestServer(t)

	body, _ := json.Marshal(map[string]any{"type": "test"})
	req := httptest.NewRequest(http.MethodPost, "/submit", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("submit: %d, body: %s", w.Code, w.Body.String())
	}
	var resp struct {
		TaskID string `json:"task_id"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.TaskID == "" {
		t.Fatal("submit: empty task_id")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/status/"+resp.TaskID, nil)
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("GET /status: %d", w2.Code)
	}
	var task taskqueue.Task
	json.NewDecoder(w2.Body).Decode(&task)
	if task.Type != "test" {
		t.Errorf("Type = %q, want %q", task.Type, "test")
	}
}

func TestList(t *testing.T) {
	srv := newTestServer(t)
	for _, typ := range []string{"a", "b", "a"} {
		body, _ := json.Marshal(map[string]any{"type": typ})
		req := httptest.NewRequest(http.MethodPost, "/submit", bytes.NewReader(body))
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("submit %s: %d", typ, w.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/list", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	var tasks []any
	json.NewDecoder(w.Body).Decode(&tasks)
	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(tasks))
	}

	req2 := httptest.NewRequest(http.MethodGet, "/list?type=a", nil)
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)
	json.NewDecoder(w2.Body).Decode(&tasks)
	if len(tasks) != 2 {
		t.Errorf("expected 2 type-a tasks, got %d", len(tasks))
	}
}

func TestCancel(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(map[string]any{"type": "cancel-me"})
	req := httptest.NewRequest(http.MethodPost, "/submit", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("submit: %d, body: %s", w.Code, w.Body.String())
	}
	var resp struct {
		TaskID string `json:"task_id"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.TaskID == "" {
		t.Fatal("submit: empty task_id")
	}

	req2 := httptest.NewRequest(http.MethodPost, "/cancel/"+resp.TaskID, nil)
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("POST /cancel: %d", w2.Code)
	}
}

func TestAssignCompleteFail(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(map[string]any{"type": "work"})
	req := httptest.NewRequest(http.MethodPost, "/submit", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("submit: %d, body: %s", w.Code, w.Body.String())
	}
	var resp struct {
		TaskID string `json:"task_id"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.TaskID == "" {
		t.Fatal("submit: empty task_id")
	}

	// Assign.
	assignBody, _ := json.Marshal(map[string]string{"node_id": "worker-1"})
	req2 := httptest.NewRequest(http.MethodPost, "/assign/"+resp.TaskID, bytes.NewReader(assignBody))
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("assign: %d", w2.Code)
	}

	// Complete.
	req3 := httptest.NewRequest(http.MethodPost, "/complete/"+resp.TaskID, nil)
	w3 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("complete: %d", w3.Code)
	}
}

func TestReassign(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(map[string]any{"type": "work"})
	req := httptest.NewRequest(http.MethodPost, "/submit", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("submit: %d, body: %s", w.Code, w.Body.String())
	}
	var resp struct {
		TaskID string `json:"task_id"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.TaskID == "" {
		t.Fatal("submit: empty task_id")
	}

	// Reassign (task is pending, should work).
	req2 := httptest.NewRequest(http.MethodPost, "/reassign/"+resp.TaskID, nil)
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("reassign: %d", w2.Code)
	}
}
