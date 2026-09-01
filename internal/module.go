package internal

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Muxcore-Media/core/pkg/contracts"
	"github.com/Muxcore-Media/worker-pool-memory/internal/server"
	"github.com/Muxcore-Media/worker-pool-memory/internal/taskqueue"
)

const (
	defaultHTTPAddr       = "127.0.0.1:9300"
	deprecatedDescription = "DEPRECATED — prefer core built-in worker pool. Historical in-memory HTTP task store (not contracts.WorkerPool)."
)

type Module struct {
	queue               *taskqueue.Queue
	srv                 *server.Server
	httpSrv             *http.Server
	lis                 net.Listener
	id                  string
	httpAddr            string
	apiToken            string
	queueCapacity       int
	advertiseCapability bool
}

type Config struct {
	ID                  string
	HTTPAddr            string
	APIToken            string
	QueueCapacity       int
	AdvertiseCapability *bool
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "worker-pool-memory"
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = defaultHTTPAddr
	}
	if v := os.Getenv("WORKER_POOL_HTTP_ADDR"); v != "" {
		cfg.HTTPAddr = v
	}
	apiToken := strings.TrimSpace(cfg.APIToken)
	if apiToken == "" {
		apiToken = strings.TrimSpace(os.Getenv("WORKER_POOL_API_TOKEN"))
	}
	queueCapacity := cfg.QueueCapacity
	if queueCapacity <= 0 {
		queueCapacity = taskqueue.DefaultCapacity
		if v := strings.TrimSpace(os.Getenv("WORKER_POOL_QUEUE_CAPACITY")); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				queueCapacity = n
			}
		}
	}
	advertise := false
	if cfg.AdvertiseCapability != nil {
		advertise = *cfg.AdvertiseCapability
	} else {
		advertise = os.Getenv("WORKER_POOL_ADVERTISE_CAPABILITY") == "1"
	}
	return &Module{
		id:                  cfg.ID,
		httpAddr:            cfg.HTTPAddr,
		apiToken:            apiToken,
		queueCapacity:       queueCapacity,
		advertiseCapability: advertise,
	}
}

func (m *Module) Info() contracts.ModuleInfo {
	var caps []string
	if m.advertiseCapability {
		caps = []string{contracts.CapabilityWorkerPool}
	}
	return contracts.ModuleInfo{
		ID:           m.id,
		Name:         "Worker Pool Memory",
		Version:      "0.1.0",
		Roles:        []string{"infrastructure"},
		Description:  deprecatedDescription,
		Author:       "MuxCore",
		Capabilities: caps,
		HTTPAddr:     m.httpAddr,
	}
}

func (m *Module) Init(ctx context.Context) error {
	if !server.IsLoopbackBind(m.httpAddr) && m.apiToken == "" {
		return fmt.Errorf("WORKER_POOL_API_TOKEN is required when WORKER_POOL_HTTP_ADDR=%q is not loopback-only", m.httpAddr)
	}

	var err error
	m.queue = taskqueue.New(m.queueCapacity)
	m.srv = server.NewWithConfig(server.Config{Queue: m.queue, APIToken: m.apiToken})
	m.lis, err = net.Listen("tcp", m.httpAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", m.httpAddr, err)
	}
	slog.Info("worker-pool initialized", "addr", m.httpAddr, "capacity", m.queueCapacity, "advertise_capability", m.advertiseCapability)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	if m.lis == nil || m.srv == nil {
		return fmt.Errorf("worker-pool-memory not initialized")
	}
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
		m.httpSrv = nil
	}
	if m.lis != nil {
		_ = m.lis.Close()
		m.lis = nil
	}
	slog.Info("worker-pool stopped")
	return nil
}

func (m *Module) Health(ctx context.Context) error {
	if m.lis == nil || m.httpSrv == nil {
		return fmt.Errorf("worker-pool-memory HTTP not serving")
	}
	d := net.Dialer{Timeout: 2 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", m.lis.Addr().String())
	if err != nil {
		return fmt.Errorf("listener unavailable: %w", err)
	}
	_ = conn.Close()
	return nil
}
