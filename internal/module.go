package internal

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"

	"github.com/Muxcore-Media/core/pkg/contracts"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
	manifest "github.com/Muxcore-Media/worker-pool-memory"
	"github.com/Muxcore-Media/worker-pool-memory/internal/server"
	"github.com/Muxcore-Media/worker-pool-memory/internal/taskqueue"
)

type Module struct {
	queue    *taskqueue.Queue
	srv      *server.Server
	httpSrv  *http.Server
	lis      net.Listener
	id       string
	httpAddr string
}

type Config struct {
	ID       string
	HTTPAddr string
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "worker-pool-memory"
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":9300"
	}
	if v := os.Getenv("WORKER_POOL_HTTP_ADDR"); v != "" {
		cfg.HTTPAddr = v
	}
	return &Module{
		id:       cfg.ID,
		httpAddr: cfg.HTTPAddr,
	}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:           m.id,
		Name:         "Worker Pool Memory",
		Version:      modulesdk.ManifestVersion(manifest.ManifestJSON),
		Roles:        []string{"infrastructure"},
		Description:  "In-memory distributed worker pool for task execution",
		Author:       "MuxCore",
		Capabilities: []string{contracts.CapabilityWorkerPool},
		HTTPAddr:     m.httpAddr,
	}
}

func (m *Module) Init(ctx context.Context) error {
	var err error
	m.queue = taskqueue.New()
	m.srv = server.New(m.queue)
	m.lis, err = net.Listen("tcp", m.httpAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", m.httpAddr, err)
	}
	slog.Info("worker-pool initialized", "addr", m.httpAddr)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	m.httpSrv = &http.Server{Handler: m.srv.Handler()}
	go func() {
		slog.Info("worker-pool HTTP started", "addr", m.httpAddr)
		if err := m.httpSrv.Serve(m.lis); err != nil && err != http.ErrServerClosed {
			slog.Error("worker-pool HTTP error", "error", err)
		}
	}()
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.httpSrv != nil {
		_ = m.httpSrv.Shutdown(ctx)
	}
	slog.Info("worker-pool stopped")
	return nil
}

func (m *Module) Health(ctx context.Context) error {
	return nil
}
