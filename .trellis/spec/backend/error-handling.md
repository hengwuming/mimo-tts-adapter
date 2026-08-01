# Error Handling

## Overview

Errors cross two trust boundaries: untrusted Legado requests and the external
MiMo provider. Validate at the boundary, preserve internal categories, and
return stable sanitized client errors.

## Error Types

- Validation errors are generated in `internal/api` and return 400/413.
- Authentication errors always return the same 401 response.
- `internal/upstream.Error` carries provider status, an internal category, and
  whether the operation may be retried.
- Context cancellation and deadline errors remain recognizable through
  `errors.Is`.

## Error Handling Patterns

- Wrap internal causes with `%w` or `Unwrap`; do not parse error strings.
- Retry only explicit transient provider categories.
- Read upstream bodies with hard limits and never return raw provider bodies.
- Complete provider validation before writing a successful audio response.
- Close every upstream response body.

## API Error Responses

Errors are JSON with a stable code/message and local request ID. They must use a
non-audio content type. Never include secrets, paragraph text, provider JSON, or
Base64 audio.

## Common Mistakes

- Retrying all 5xx/4xx responses instead of the documented retry matrix.
- Writing HTTP 200 before Base64 decoding completes.
- Returning a provider error page with `audio/mpeg`.
- Treating a context cancellation as a retryable transport failure.
