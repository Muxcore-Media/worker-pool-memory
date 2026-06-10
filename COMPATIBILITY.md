# Compatibility

## Core Version

Requires MuxCore v1.0.0 or later.

## Capabilities

Registers with capability: `worker.pool`

## Contract Dependencies

- `github.com/Muxcore-Media/core/pkg/contracts` — WorkerPool, Executor interfaces
- gRPC ModuleRegistration service for sidecar registration
- gRPC DiscoveryService for executor discovery
- gRPC ModuleMesh for task dispatch
- Cluster events for node liveness
