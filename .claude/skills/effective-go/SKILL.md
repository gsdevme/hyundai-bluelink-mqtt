---
name: effective-go
description: Use when writing or reviewing idiomatic Go in this repo — package boundaries, accept-interfaces/return-structs, error wrapping, context propagation, concurrency, zero-value design, table tests.
---

# Effective Go (project conventions)

Idioms this codebase follows. When in doubt, prefer the smaller, more boring option.

## Package & API design

- **Accept interfaces, return structs.** Constructors return concrete `*Client`;
  functions take narrow interfaces (`Publisher`, `TokenStore`) so tests substitute fakes.
- Define interfaces **where they are consumed**, not where implemented. `Publisher`
  lives with `publisher`, not with the autopaho backend.
- Keep interfaces small (1–3 methods). A one-method interface is often ideal.
- Exported identifiers get doc comments starting with the identifier name.
- Package names are short, lower-case, no underscores; the name is part of the API
  (`bluelink.Client`, not `bluelink.BluelinkClient`).

## Errors

- Wrap with context: `fmt.Errorf("fetch cached status: %w", err)`. Keep the wrapped
  chain so callers can `errors.Is`/`errors.As`.
- Define sentinel errors for branchable conditions: `var ErrCCS1NotImplemented = …`,
  `ErrConsentRequired`. Compare with `errors.Is`.
- Don't log-and-return the same error; do one. Log at the top; return below.
- Never `panic` for expected failures (missing JSON fields, HTTP non-200).

## Context

- First parameter is `ctx context.Context` for anything doing I/O.
- Propagate it into every `http.Request` (`http.NewRequestWithContext`).
- Derive per-attempt deadlines with `context.WithTimeout`; always `defer cancel()`.
- Select on `ctx.Done()` in loops (poll loop, scheduler) for prompt shutdown.

## Concurrency

- Prefer a single owner goroutine + channels over shared mutable state; use a `sync.Mutex`
  for small shared flags (readiness, last state).
- The scheduler serialises API calls so a force refresh and a cached poll never overlap.
- Don't start goroutines you can't stop: everything hangs off the root `context`.
- Use `errgroup` only if it earns its keep; a plain `sync.WaitGroup` is often enough.

## Zero values & construction

- Design so the zero value is useful where possible (a `sync.Mutex` needs no init).
- Validate in constructors (`New...`) and return an error rather than a half-built value.
- Use pointer fields / a small `Optional[T]` to distinguish "absent" from a real zero
  in `VehicleState` (a `0.0` battery reading is not the same as "unknown").

## HTTP clients

- One `*http.Client` with a sensible `Timeout`, reused (not per-request).
- Always `defer resp.Body.Close()`; read/discard the body before closing to reuse conns.
- Set an explicit `User-Agent`; build headers in one helper.

## Testing

- Table-driven tests with subtests (`t.Run(name, …)`); name cases meaningfully.
- Use `t.Helper()` in assertion helpers. Prefer `testing`+ stdlib over new deps.
- Golden/fixture files under `testdata/`. Compare structs with `reflect.DeepEqual` or
  field-by-field; for floats compare with tolerance.
- `t.Context()` (Go 1.24+) for a test-scoped context; `testing/synctest` for time.

## Style

- `gofmt`/`go vet` clean; no unused params (use `_`); early returns over deep nesting.
- Keep functions short; extract parsing helpers (`getPath`, unit converters).
- Comments explain **why**, not what. Match the density of surrounding code.
