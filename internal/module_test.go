package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/Muxcore-Media/core/pkg/contracts"
)

func TestModuleInfo_OmitsWorkerPoolCapabilityByDefault(t *testing.T) {
	m := NewModule(Config{})
	info := m.Info()
	for _, c := range info.Capabilities {
		if c == contracts.CapabilityWorkerPool {
			t.Fatal("worker.pool capability must not be advertised by default")
		}
	}
	if info.Description != deprecatedDescription {
		t.Fatalf("description=%q", info.Description)
	}
}

func TestModuleInfo_AdvertisesCapabilityWhenEnabled(t *testing.T) {
	t.Setenv("WORKER_POOL_ADVERTISE_CAPABILITY", "1")
	m := NewModule(Config{})
	info := m.Info()
	found := false
	for _, c := range info.Capabilities {
		if c == contracts.CapabilityWorkerPool {
			found = true
		}
	}
	if !found {
		t.Fatal("expected worker.pool capability when WORKER_POOL_ADVERTISE_CAPABILITY=1")
	}
}

func TestModule_InitRejectsNonLoopbackWithoutToken(t *testing.T) {
	m := NewModule(Config{HTTPAddr: ":9301"})
	if err := m.Init(context.Background()); err == nil {
		t.Fatal("expected error without API token on wildcard bind")
	}
}

func TestModuleLifecycle_HTTP(t *testing.T) {
	m := NewModule(Config{HTTPAddr: "127.0.0.1:0", QueueCapacity: 100})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.Health(ctx); err == nil {
		t.Fatal("expected Health error before Start")
	}
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	baseURL := "http://" + m.lis.Addr().String()

	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /health: %d", resp.StatusCode)
	}

	submitBody, _ := json.Marshal(map[string]any{"type": "lifecycle-test"})
	submitResp, err := http.Post(baseURL+"/submit", "application/json", bytes.NewReader(submitBody))
	if err != nil {
		t.Fatalf("POST /submit: %v", err)
	}
	defer func() { _ = submitResp.Body.Close() }()
	if submitResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(submitResp.Body)
		t.Fatalf("POST /submit: %d body=%s", submitResp.StatusCode, b)
	}
	var out struct {
		TaskID string `json:"task_id"`
	}
	if err := json.NewDecoder(submitResp.Body).Decode(&out); err != nil {
		t.Fatalf("decode submit: %v", err)
	}
	if out.TaskID == "" {
		t.Fatal("empty task_id")
	}

	if err := m.Health(ctx); err != nil {
		t.Fatalf("Health while serving: %v", err)
	}
	if err := m.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := m.Health(ctx); err == nil {
		t.Fatal("expected Health error after Stop")
	}
}

func TestModule_DualRegistrationDocumentedInREADME(t *testing.T) {
	data, err := os.ReadFile("../README.md")
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	body := string(data)
	for _, needle := range []string{"MVP_ENABLE_WORKER_POOL", "core built-in worker pool", "WORKER_POOL_ADVERTISE_CAPABILITY"} {
		if !bytes.Contains(data, []byte(needle)) {
			t.Fatalf("README missing %q", needle)
		}
	}
	_ = body
}
