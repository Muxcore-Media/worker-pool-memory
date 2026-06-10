# worker-pool-memory — Implementation Roadmap

**Priority:** P0 — Required for distributed operation and failover.

## Phases

### Phase 1: Core Queue (minimum viable)
- [x] Project scaffold (this repo)
- [ ] `go mod init` with core dependency
- [ ] In-memory priority queue (heap-based, goroutine-safe)
- [ ] `WorkerPool.Submit` — enqueue a task
- [ ] `WorkerPool.Status` — query task state
- [ ] `WorkerPool.Cancel` — abort a queued task
- [ ] `WorkerPool.List` — query tasks by filter
- [ ] Task state machine (pending → assigned → running → completed/failed)
- [ ] Sidecar entry point (`cmd/module/main.go`)
- [ ] Unit tests: queue ops, state transitions (50+ tests)

### Phase 2: Executor Discovery & Dispatch
- [ ] Discover `Executor` modules via registry (`FindByCapability`)
- [ ] Dispatch tasks to executor modules via gRPC mesh
- [ ] Result collection and status update
- [ ] Retry with backoff (configurable per task via MaxRetries)
- [ ] `IdempotencyKey` support (check before dispatch)
- [ ] Unit tests: dispatch, retry, idempotency (30+ tests)

### Phase 3: Cluster & Failover
- [ ] Node liveness tracking via cluster events
- [ ] `WorkerPool.Reassign` — task migration after node failure
- [ ] Heartbeat timeout handling (30s default)
- [ ] Integration test with multi-node cluster
- [ ] Graceful drain on shutdown (finish running tasks)
- [ ] Published events: `worker.task.started`, `.completed`, `.failed`, `.cancelled`

### Phase 4: Operational
- [ ] Prometheus metrics (queue depth, active tasks, failure rate)
- [ ] Health endpoint
- [ ] Audit logging of task lifecycle
- [ ] GitHub CI (build + lint + test)

## Design Decisions

1. **In-memory only for MVP** — No persistence. Tasks are lost on module restart.
   Persistent backend (Redis, database) is a future enhancement.
2. **Heap-based priority queue** — Simple, predictable performance.
3. **Polling for executor discovery** — Simpler than subscribing to registry
   events. 15-second poll interval is acceptable for MVP.
4. **Reassign via cluster events** — Watch `cluster.node.left` events to
   detect dead nodes and reassign their tasks.
