# Worker Pool Memory

In-memory distributed worker pool for MuxCore.

Schedules tasks across cluster nodes with failover reassignment. Without this
module, there is no distributed task execution, no module failover after node
death, and no cross-node work distribution.

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
