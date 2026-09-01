package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Muxcore-Media/worker-pool-memory/internal/taskqueue"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	return New(taskqueue.New(100))
}

func newAuthedServer(t *testing.T, token string) *Server {
	t.Helper()
	return NewWithConfig(Config{Queue: taskqueue.New(100), APIToken: token})
}

func TestSubmit(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(map[string]any{
		"type":        "transcode",
		"payload":     []byte(`{"file":"movie.mkv"}`),
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
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
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

func TestSubmit_DeniedWithoutToken(t *testing.T) {
	srv := newAuthedServer(t, "secret")
	body, _ := json.Marshal(map[string]any{"type": "work"})
	req := httptest.NewRequest(http.MethodPost, "/submit", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestSubmit_AllowedWithToken(t *testing.T) {
	srv := newAuthedServer(t, "secret")
	body, _ := json.Marshal(map[string]any{"type": "work"})
	req := httptest.NewRequest(http.MethodPost, "/submit", bytes.NewReader(body))
	req.Header.Set("X-Worker-Pool-Token", "secret")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestMutatingRoutes_DeniedWithoutToken(t *testing.T) {
	srv := newAuthedServer(t, "secret")
	if _, err := srv.queue.Submit("work", nil, 0, nil, "", nil); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/cancel/task-1", ""},
		{http.MethodPost, "/assign/task-1", `{"node_id":"n1"}`},
		{http.MethodPost, "/complete/task-1", ""},
		{http.MethodPost, "/fail/task-1", `{"error":"boom"}`},
		{http.MethodPost, "/reassign/task-1", ""},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			var body *bytes.Reader
			if tc.body == "" {
				body = bytes.NewReader(nil)
			} else {
				body = bytes.NewReader([]byte(tc.body))
			}
			req := httptest.NewRequest(tc.method, tc.path, body)
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", w.Code)
			}
		})
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
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
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
	if err := json.NewDecoder(w2.Body).Decode(&task); err != nil {
		t.Fatalf("decode task: %v", err)
	}
	if task.Type != "test" {
		t.Errorf("Type = %q, want %q", task.Type, "test")
	}
}

func TestStatus_EmptyID(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/status/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCancel_EmptyID(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/cancel/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
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
	if err := json.NewDecoder(w.Body).Decode(&tasks); err != nil {
		t.Fatalf("decode tasks: %v", err)
	}
	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(tasks))
	}

	req2 := httptest.NewRequest(http.MethodGet, "/list?type=a", nil)
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)
	if err := json.NewDecoder(w2.Body).Decode(&tasks); err != nil {
		t.Fatalf("decode filtered tasks: %v", err)
	}
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
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
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
	body, _ := json.Marshal(map[string]any{"type": "work", "max_retries": 1})
	req := httptest.NewRequest(http.MethodPost, "/submit", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("submit: %d, body: %s", w.Code, w.Body.String())
	}
	var resp struct {
		TaskID string `json:"task_id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.TaskID == "" {
		t.Fatal("submit: empty task_id")
	}

	assignBody, _ := json.Marshal(map[string]string{"node_id": "worker-1"})
	req2 := httptest.NewRequest(http.MethodPost, "/assign/"+resp.TaskID, bytes.NewReader(assignBody))
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("assign: %d", w2.Code)
	}

	req3 := httptest.NewRequest(http.MethodPost, "/complete/"+resp.TaskID, nil)
	w3 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("complete: %d", w3.Code)
	}
}

func TestFail(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(map[string]any{"type": "work", "max_retries": 1})
	req := httptest.NewRequest(http.MethodPost, "/submit", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	var resp struct {
		TaskID string `json:"task_id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	assignBody, _ := json.Marshal(map[string]string{"node_id": "worker-1"})
	req2 := httptest.NewRequest(http.MethodPost, "/assign/"+resp.TaskID, bytes.NewReader(assignBody))
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("assign: %d", w2.Code)
	}

	failBody, _ := json.Marshal(map[string]string{"error": "boom"})
	req3 := httptest.NewRequest(http.MethodPost, "/fail/"+resp.TaskID, bytes.NewReader(failBody))
	w3 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("fail: %d body=%s", w3.Code, w3.Body.String())
	}
}

func TestMetrics(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(map[string]any{"type": "metrics"})
	req := httptest.NewRequest(http.MethodPost, "/submit", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("submit: %d", w.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("GET /metrics: %d", w2.Code)
	}
	if !strings.Contains(w2.Body.String(), "worker_pool_tasks_pending") {
		t.Fatalf("metrics body missing pending gauge: %s", w2.Body.String())
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
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.TaskID == "" {
		t.Fatal("submit: empty task_id")
	}

	req2 := httptest.NewRequest(http.MethodPost, "/reassign/"+resp.TaskID, nil)
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("reassign: %d", w2.Code)
	}
}

func TestIsLoopbackBind(t *testing.T) {
	if !IsLoopbackBind("127.0.0.1:9300") {
		t.Fatal("127.0.0.1 should be loopback")
	}
	if IsLoopbackBind(":9300") {
		t.Fatal(":9300 should not be loopback")
	}
}
