# Logging Guidelines

## Overview

The service uses the standard library `log/slog` with `JSONHandler`. Logging is
operational metadata only; book text and credentials are sensitive.

## Log Levels

- `Info`: startup, graceful shutdown, and one access record per request.
- `Warn`: recoverable provider/retry conditions when added.
- `Error`: invalid startup configuration, listener failure, or failed graceful
  shutdown.
- Avoid debug payload logging even in development.

## Structured Logging

Access logs use stable fields such as `request_id`, `method`, `path`, `status`,
`bytes`, and `duration_ms`. Use fixed URL paths, never raw URLs or query strings.

## What to Log

- Process lifecycle and listen address.
- Request status, duration, and response byte count.
- Sanitized provider category/status and retry count when useful.

## What NOT to Log

- `Authorization`, `ADAPTER_AUTH_TOKEN`, or `MIMO_API_KEY`.
- Paragraph text, request bodies, query strings, or voice-clone material.
- Upstream request/response JSON, Base64 data, or audio bytes.
- Complete endpoint URLs if they may carry credentials.
