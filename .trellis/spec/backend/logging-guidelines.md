# Logging Guidelines

## Overview

The ordinary stdout stream uses the standard library `log/slog` with
`JSONHandler`. It contains operational metadata only; book text and credentials
are sensitive. The separate, explicitly enabled response file below is the sole
full-text exception.

## Log Levels

- `Info`: startup, graceful shutdown, and one access record per request.
- `Warn`: recoverable provider/retry conditions when added.
- `Error`: invalid startup configuration, listener failure, or failed graceful
  shutdown.
- Avoid debug payload logging even in development.

## Structured Logging

Access logs use stable fields such as `request_id`, `method`, `path`, `status`,
`bytes`, and `duration_ms`. Use fixed URL paths, never raw URLs or query strings.
Emotion and MiMo stage logs may add only request ID, status, duration, and a
sanitized error category so latency can be separated without exposing payloads.

## Optional Sensitive Response Log

Text-model response logging is a deliberate, default-off exception. When
`EMOTION_RESPONSE_LOG_FILE` is configured:

- open the file at startup in append mode with `0600`, failing startup if it
  cannot be opened;
- write one JSON object per line with timestamp, optional request ID, status,
  stable `error_category` on failure, attempts, duration, extracted model content,
  and verified `style_instruction` only on success; for non-2xx provider replies,
  it may also contain the numeric provider status and a bounded standard
  `error.message`, but never an unstructured raw error page;
- keep annotation and TTS running if a later write fails, and never copy the
  sensitive entry to stdout;
- never record credentials, complete provider envelopes, MiMo payloads, Base64,
  or audio bytes.

When emotion annotation fails, use one of the stable categories `timeout`,
`cancelled`, `provider_status`, `response_too_large`, `provider_json`,
`content_json`, `invalid_range`, or `invalid_style` when applicable. Categories
come from typed/sentinel errors; never classify by parsing error text.

When the path is empty, no full-text log is created. The operator owns directory
permissions, mounting, rotation, retention, and access.

## What to Log

- Process lifecycle and listen address.
- Request status, duration, and response byte count.
- Sanitized provider category/status and retry count when useful.

## What NOT to Log

- `Authorization`, `ADAPTER_AUTH_TOKEN`, or `MIMO_API_KEY`.
- Paragraph text, request bodies, query strings, or voice-clone material in
  stdout or ordinary operational logs. The only exception is the explicitly
  configured sensitive response file above.
- Upstream request/response JSON, Base64 data, or audio bytes.
- Complete endpoint URLs if they may carry credentials.
