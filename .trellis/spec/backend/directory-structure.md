# Backend Directory Structure

## Overview

The backend is one Go service. Keep packages shallow and internal to the
application; add a package only when it owns a distinct boundary or behavior.

## Layout

```text
cmd/mimo-tts-adapter/main.go  Process wiring and lifecycle
internal/config/              Environment parsing and startup validation
internal/auth/                Bearer authentication
internal/limits/              Concurrency and rate admission
internal/api/                 Legado-facing HTTP contract
internal/upstream/            Xiaomi MiMo protocol and retries
```

Tests live beside their package as `*_test.go`. Deployment files and user
configuration examples remain at repository root.

## Conventions

- `cmd/` wires concrete dependencies; business behavior belongs in `internal/`.
- Provider-specific JSON and status handling stay in `internal/upstream`.
- HTTP parsing, validation, and public error responses stay in `internal/api`.
- Use short lowercase package and file names that describe ownership.
- Do not create `pkg/`, repositories, database layers, or framework-style
  controller/service hierarchies without a concrete second use case.

## Examples

- `internal/upstream/client.go` owns the complete MiMo request/response contract.
- `internal/api/handler.go` normalizes GET and POST into one synthesis request.
