# Quality Guidelines

## Overview

The project is standard-library-first Go. Prefer small concrete packages and
explicit contracts over frameworks or speculative abstractions.

## Forbidden Patterns

- Credentials in query parameters, logs, rules, fixtures, or image layers.
- Unbounded `io.ReadAll` on request or provider-controlled data.
- HTTP clients/transports created per request.
- Blind retries of provider POST requests or all status codes.
- Trusting arbitrary forwarded headers to generate public URLs.
- Returning provider JSON directly to Legado.
- Adding routers, DI containers, config frameworks, or database layers without
  a demonstrated requirement.

## Required Patterns

- Validate configuration once at startup and requests at the API boundary.
- Reuse one `http.Client`/`Transport`.
- Use contexts for queueing, rate waits, retries, and upstream calls.
- Close provider response bodies and detect limit overflow using `limit+1`.
- Use strict Bearer parsing and `crypto/subtle` comparison.
- Keep MiMo schemas isolated in `internal/upstream`.
- For MiMo TTS, keep the exact speakable text in the `assistant` message. Put
  speed, emotion, and delivery instructions only in the `user` message so
  control text cannot become spoken content.
- Emotion output uses ordered, non-overlapping Unicode rune `[start,end)` ranges
  bounded by the original paragraph. Validate range, style, count, and total
  instruction limits locally.
- Keep emotion calls single-flight. Queue waits use the request context and count
  toward the emotion timeout, independently of MiMo synthesis concurrency.
- Only unwrap a single `json` or unlabelled fence that encloses the entire trimmed
  emotion response; reject surrounding prose or nested fences.
## Testing Requirements

- Unit tests use `testing`, `httptest`, and fake `RoundTripper` implementations.
- Test auth, strict JSON, bounds, normalization, provider payloads, Base64,
  retry matrices, cancellation, response MIME, rule redaction, and log privacy.
- Emotion regressions must assert `assistant.content` equals the original text
  byte-for-byte and every generated style instruction appears only in
  `user.content`. Fenced JSON may be unwrapped only when the fence is the entire
  model content; surrounding prose remains invalid.
- Normal tests must not call the real MiMo API.

Required checks:

```text
gofmt -w ./cmd ./internal
go test ./...
go test -race ./...
go vet ./...
go mod verify
```

## Code Review Checklist

- Does raw audio remain `audio/mpeg` and every error remain non-audio?
- Can any text, token, key, or provider payload reach logs or rules?
- Are all externally controlled reads bounded?
- Are retries finite, cancellable, and limited to approved cases?
- Does Docker run non-root without baking secrets into layers?
