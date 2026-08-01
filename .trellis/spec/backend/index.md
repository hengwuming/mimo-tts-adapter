# Backend Development Guidelines

> Project-specific conventions for the Go MiMo TTS adapter.

## Guidelines Index

| Guide | Description | Status |
|-------|-------------|--------|
| [Directory Structure](./directory-structure.md) | Go package ownership and layout | Active |
| [Database Guidelines](./database-guidelines.md) | No database in the current service | Not applicable |
| [Error Handling](./error-handling.md) | Provider and API error contracts | Active |
| [Quality Guidelines](./quality-guidelines.md) | Go checks, tests, and forbidden patterns | Active |
| [Logging Guidelines](./logging-guidelines.md) | `slog` fields and privacy rules | Active |

## Pre-Development Checklist

- Read directory structure before adding a package.
- Read error handling before changing provider or HTTP behavior.
- Read logging guidelines before adding log fields.
- Read quality guidelines before implementation and review.
- Read shared cross-layer and code-reuse guides for contract changes.

**Language**: All specification documentation is written in English.
