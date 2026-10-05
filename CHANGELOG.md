# Changelog

## [0.1.3] - 2026-10-05

### Changed
- Built on core v0.6.14 / sdk/go/module v0.6.4: unregisters on shutdown and re-registers after core restarts (ADR-0022).

## [0.1.2] - 2026-10-05


### Changed
- Reported version comes from muxcore.json (ADR-0021); built on core v0.6.12 / sdk/go/module v0.6.3 (mesh enrollment, ADR-0017).

## [0.1.1] - 2026-10-05

### Changed
- CI runs on GitHub-hosted runners from the umbrella template; retired-origin workflows removed.
- Dependencies resolve from published GitHub tags (no filesystem `replace`); requires core v0.6.0.

## [0.1.0]

### Added

- Initial in-memory `worker.pool` sidecar scaffold.

### Deprecated

- Entire module: prefer MuxCore core’s built-in worker pool. Spool catalog marks `deprecated: true`. No further Phase 2–3 investment planned unless the built-in pool is insufficient.
