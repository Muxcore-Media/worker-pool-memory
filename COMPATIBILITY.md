# Compatibility

## Status

**Deprecated.** Prefer the worker pool built into MuxCore `core`. This sidecar remains for compatibility only.

## Core Version

Pinned modules use `core` ≥ 0.5.0. Older “Requires MuxCore v1.0.0” language is historical and inaccurate for this pre-1.0 sidecar.

## Capabilities

Registers with capability: `worker.pool`

Do **not** run alongside core’s built-in pool when both would advertise the same role — pick one.

## Contract Dependencies

- `github.com/Muxcore-Media/core/pkg/contracts` — WorkerPool, Executor interfaces
- gRPC ModuleRegistration for sidecar registration
