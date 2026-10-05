# Changelog

## [Unreleased]

## [0.1.1] - 2026-10-05

### Changed
- CI runs on GitHub-hosted runners from the umbrella template; retired-origin workflows removed.
- Dependencies resolve from published GitHub tags (no filesystem `replace`); requires core v0.6.0.

## [0.1.0]

### Added

- Initial in-memory `worker.pool` sidecar scaffold.

### Deprecated

- Entire module: prefer MuxCore core’s built-in worker pool. Spool catalog marks `deprecated: true`. No further Phase 2–3 investment planned unless the built-in pool is insufficient.
