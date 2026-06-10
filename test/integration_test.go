package test

import (
	"context"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWorkerPool_HTTP(t *testing.T) {
	bin := buildModule(t)
	addr := ":19401"
	baseURL := "http://127.0.0.1" + addr

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "--http-addr", addr)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer cmd.Process.Kill()

	time.Sleep(500 * time.Millisecond)

	// Submit a task.
	body, _ := json.Marshal(map[string]any{
		"type": "transcode", "payload": []byte(`{"file":"test.mkv"}`), "max_retries": 3,
	})
	resp, err := http.Post(baseURL+"/submit", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /submit: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var result struct{ TaskID string `json:"task_id"` }
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()
	if result.TaskID == "" {
		t.Fatal("expected non-empty task_id")
	}

	// Check status.
	resp2, err := http.Get(baseURL + "/status/" + result.TaskID)
	if err != nil {
		t.Fatalf("GET /status: %v", err)
	}
	if resp2.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}
	var task struct {
		ID     string `json:"id"`
		Type   string `json:"type"`
		Status string `json:"status"`
	}
	json.NewDecoder(resp2.Body).Decode(&task)
	resp2.Body.Close()
	if task.Status != "pending" {
		t.Errorf("expected pending, got %s", task.Status)
	}

	// Assign and complete.
	assignBody, _ := json.Marshal(map[string]string{"node_id": "worker-1"})
	resp3, err := http.Post(baseURL+"/assign/"+result.TaskID, "application/json", bytes.NewReader(assignBody))
	if err != nil {
		t.Fatalf("POST /assign: %v", err)
	}
	if resp3.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp3.StatusCode)
	}
	resp3.Body.Close()

	resp4, err := http.Post(baseURL+"/complete/"+result.TaskID, "application/json", nil)
	if err != nil {
		t.Fatalf("POST /complete: %v", err)
	}
	if resp4.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp4.StatusCode)
	}
	resp4.Body.Close()

	// Verify completed status.
	resp5, _ := http.Get(baseURL + "/status/" + result.TaskID)
	json.NewDecoder(resp5.Body).Decode(&task)
	resp5.Body.Close()
	if task.Status != "completed" {
		t.Errorf("expected completed, got %s", task.Status)
	}
}

func buildModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "worker-pool-memory")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/module")
	cmd.Dir = findRepoRoot(t)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build: %v", err)
	}
	return bin
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").CombinedOutput()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	dir, _ := os.Getwd()
	for dir != "/" {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("cannot find repo root")
	return ""
}

func init() {
	fmt.Fprintln(os.Stderr, "integration tests: building module binary...")
}
