# Worker Pool Memory

> **Deprecated.** Prefer MuxCore **core’s built-in worker pool**. Do not invest in Phases 2–3 of this sidecar unless the built-in pool is proven insufficient. Kept in the official spool catalog only for compatibility (`deprecated: true`).

In-memory distributed worker pool sidecar for MuxCore (historical).

Schedules tasks across cluster nodes with failover reassignment. New deployments should use the core built-in pool instead of this module.

## How It Works

```
Module submits task via WorkerPool.Submit()
        │
        ▼
worker-pool-memory enqueues task
        │
        ▼
Task assigned to available node (by capability match)
        │
        ▼
Node's executor module picks up task via gRPC
        │
        ▼
Executor calls back with result → status updated
```

### Task Lifecycle

```
pending ──→ assigned ──→ running ──→ completed
                  │                   
                  └──→ running ──→ failed (retry if MaxRetries > 0)
                            
running ──→ pending (reassigned on node heartbeat timeout)
```

### Executor Discovery

Worker modules implement `contracts.Executor` and register with capabilities
matching the task types they handle. The worker pool discovers them via
`Registry.FindByCapability`:

```go
type Executor interface {
    CanHandle(taskType string) bool
    Execute(ctx context.Context, task WorkerTask) ([]byte, error)
}
```

## Configuration

### CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--queue-capacity` | `10000` | Maximum pending tasks |
| `--heartbeat-timeout` | `30s` | Node heartbeat timeout before reassign |
| `--max-retries` | `3` | Default max retry attempts per task |
| `--rebalance-interval` | `60s` | How often to check for idle nodes |

## Implementation

- Registers with capability: `"worker.pool"`
- Implements `contracts.WorkerPool` (Submit, Status, Cancel, List, Reassign)
- In-memory priority queue (heap-based)
- Watches cluster events for node liveness
- Executor discovery via registry polling
- Supports `IdempotencyKey` for exactly-once execution
- Published events: `worker.task.*`
