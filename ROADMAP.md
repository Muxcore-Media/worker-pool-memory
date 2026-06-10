# worker-pool-memory — Implementation Roadmap

**Priority:** P0 — Required for distributed operation and failover.

## Phases

### Phase 1: Core Task Queue (minimum viable) ✅
- [x] Project scaffold
- [x] `go mod init` with core dependency
- [x] In-memory task store with full state machine (`internal/taskqueue`)
- [x] HTTP API: submit, status, cancel, list, assign, complete, fail, reassign
- [x] IdempotencyKey support (deduplication)
- [x] Retry with configurable max retries
- [x] Sidecar entry point (`cmd/module/main.go`)
- [x] Unit tests: task queue state machine (12+ test cases)
- [x] Unit tests: HTTP API (7 test scenarios)

### Phase 2: Executor Discovery & Dispatch
- [ ] Discover executor modules via core registry
- [ ] Task dispatch to executors via gRPC mesh
- [ ] Result collection and status update
- [ ] Cluster event monitoring for node liveness

### Phase 3: Cluster & Failover
- [ ] WorkerPool.Reassign on node heartbeat timeout
- [ ] Graceful drain on shutdown
- [ ] Prometheus metrics
- [ ] Integration test with multi-node cluster

## Design Decisions

1. **In-memory only for MVP** — No persistence. Tasks are lost on module restart.
   Persistent backend (Redis, database) is a future enhancement.
2. **Heap-based priority queue** — Simple, predictable performance.
3. **Polling for executor discovery** — Simpler than subscribing to registry
   events. 15-second poll interval is acceptable for MVP.
4. **Reassign via cluster events** — Watch `cluster.node.left` events to
   detect dead nodes and reassign their tasks.
