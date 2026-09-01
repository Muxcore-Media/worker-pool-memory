# Worker Pool Memory

> **Deprecated.** Prefer MuxCore **core's built-in worker pool**. This sidecar is a simple in-memory HTTP task store kept for compatibility (`deprecated: true` in `muxcore.json`). It does **not** implement `contracts.WorkerPool` — no mesh/gRPC submit/status/cancel RPCs, no executor discovery, no cluster heartbeat reaper.

## What it actually is

An optional HTTP sidecar that stores task records in memory. Clients call REST endpoints directly; workers poll or get assigned via HTTP. State is lost on restart.

```
Client POST /submit  →  in-memory map  →  worker POST /assign, /complete, /fail
```

This is **not** a drop-in replacement for core's worker pool. Enabling both registers two different implementations of the same role unless you take care:

| Component | Role |
|-----------|------|
| **core built-in worker pool** | Default when `MVP_ENABLE_WORKER_POOL` is unset |
| **worker-pool-memory** | Optional sidecar when `MVP_ENABLE_WORKER_POOL=1` in `_mvp/run-host.sh` |

Do **not** set `WORKER_POOL_ADVERTISE_CAPABILITY=1` unless you intentionally want this module to claim `worker.pool` on the mesh (collides with core). Default registration omits that capability.

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `WORKER_POOL_HTTP_ADDR` | `127.0.0.1:9300` | HTTP listen address |
| `WORKER_POOL_API_TOKEN` | (empty) | Required when binding a non-loopback address; send as `X-Worker-Pool-Token` or `Authorization: Bearer` |
| `WORKER_POOL_QUEUE_CAPACITY` | `10000` | Maximum tasks held in memory; `Submit` rejects when full |
| `WORKER_POOL_ADVERTISE_CAPABILITY` | unset | Set to `1` to advertise `worker.pool` on registration (discouraged) |
| `MVP_ENABLE_WORKER_POOL` | `0` | `_mvp/run-host.sh` — start this sidecar alongside core |
| `MUXCORE_GRPC_ADDR` | (SDK default) | Core mesh address for sidecar registration |
| `MUXCORE_MODULE_ID` | `worker-pool-memory` | Module identity |
| `MUXCORE_INSECURE_DISABLE_TLS` | unset | Dev-only: disable TLS to core |

## HTTP API

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/submit` | yes | Create a task (`type` required; optional `payload`, `max_retries`, `capabilities`, `idempotency_key`, `meta`) |
| `GET` | `/status/{id}` | no | Fetch task snapshot |
| `POST` | `/cancel/{id}` | yes | Cancel pending/assigned task |
| `GET` | `/list` | no | List tasks (`?status=`, `?type=`) |
| `POST` | `/assign/{id}` | yes | Assign pending task (`node_id` required) |
| `POST` | `/complete/{id}` | yes | Complete assigned/running task |
| `POST` | `/fail/{id}` | yes | Fail assigned/running task (`error` optional); retries reset to pending when under `max_retries` |
| `POST` | `/reassign/{id}` | yes | Move task back to pending |
| `GET` | `/health` | no | Liveness |
| `GET` | `/metrics` | no | Prometheus gauges for pending/running/completed/failed |

### Task lifecycle

```
pending → assigned → running → completed
              │          │
              └──────────┴→ failed (retry → pending when retries remain)
pending/assigned → cancelled
```

Idempotency: when `idempotency_key` is set, duplicate submits return the existing task id for all non-failed tasks (exactly-once submit semantics; failed tasks may be resubmitted with the same key).

## Build & test

```bash
cd worker-pool-memory
go test -race ./...
golangci-lint run
```

## Operator notes

- Default bind is loopback-only. For LAN/container exposure set `WORKER_POOL_HTTP_ADDR` **and** `WORKER_POOL_API_TOKEN`.
- `_mvp/run-host.sh` uses `127.0.0.1:9300` when `MVP_ENABLE_WORKER_POOL=1`.
- Port reference: `_mvp/PORTS.md` (`9300`).
