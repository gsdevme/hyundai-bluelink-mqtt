---
name: go-1.26
description: Use when writing Go for this repo to apply current-toolchain (go1.26) idioms — iterators/range-over-func, slices/maps, log/slog, testing/synctest, t.Context, generics. Validated against the installed go1.26.2.
---

# Go 1.26 toolchain idioms

Validated against `go1.26.2` (installed). Prefer these over older patterns.

## Iterators (range-over-func)

- Functions returning `iter.Seq[T]` / `iter.Seq2[K,V]` are `range`-able:
  `for v := range seq { … }`. Use for streaming without materialising slices.
- Bridge to slices with `slices.Collect(seq)`, `slices.Sorted(seq)`,
  `maps.Keys(m)`/`maps.Values(m)` (both return iterators — wrap with
  `slices.Sorted`/`slices.Collect` to get a slice).

## `slices` and `maps`

- `slices.Contains`, `ContainsFunc`, `IndexFunc`, `SortFunc`, `SortedFunc`,
  `Concat`, `Equal`, `Clone`, `Compact`, `BinarySearch`.
- `maps.Clone`, `maps.Copy`, `maps.Equal`, `maps.Keys`/`maps.Values` (iterators).
- Builtins `min`, `max`, `clear` are available — use `min(len(a), len(b))` for the
  truncating stamp XOR rather than a hand-rolled loop bound.

## `log/slog` (structured logging)

- Default to `slog`. Build a handler from config:
  `slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})` (or `TextHandler`).
- `slog.SetDefault(slog.New(handler))`; then `slog.InfoContext(ctx, "polled",
  "vin", vin, "battery", pct)`.
- Prefer typed attrs for hot paths: `slog.Int`, `slog.String`, `slog.Duration`.
- Never log secrets — pass a redacted config value.

## Context in tests

- `t.Context()` returns a context cancelled just before cleanup — use it instead of
  `context.Background()` in tests so goroutines stop deterministically.

## `testing/synctest` (deterministic time)

- `synctest.Test(t, func(t *testing.T){ … })` runs the body in a **bubble** with a
  **fake clock** starting 2000-01-01 UTC. `time.Sleep`, `time.After`, `time.Ticker`,
  and `context` timers all use the fake clock.
- `synctest.Wait()` blocks until every goroutine in the bubble is durably blocked —
  use it to advance past scheduled work without real sleeps.
- Ideal for the scheduler: assert the daily force-refresh fires at the right wall-clock
  time and the cached poll ticks on interval, with zero wall-clock delay.
- Keep bubbles self-contained: no real network/processes; use the mock server via an
  in-bubble `httptest`-style fake or inject the client.

## Generics

- Use type params for small utilities (`Optional[T]`, `ptr[T any](v T) *T`), not for
  everything. A concrete type is clearer when there's one caller.
- Constraints from `cmp.Ordered` for comparisons; `any` for pass-through.

## JSON

- Stick with stdlib `encoding/json` (struct tags, `json.RawMessage` for deferred
  parsing, `json.Number` for numeric precision). `encoding/json/v2` is a GOEXPERIMENT,
  **not** enabled by default here — do not import it.
- For the CCS2 tree, unmarshal into `map[string]any` and walk with a safe dotted-path
  helper; reserve typed structs for the small, stable envelope (`retCode`, `resMsg`).

## Misc

- `errors.Join` to combine multiple failures; `errors.Is`/`As` for inspection.
- `context.WithTimeoutCause`/`WithCancelCause` when a specific cancellation reason helps
  debugging.
- Run `go vet ./...` — it catches slog key/value mismatches and loop/context misuse.
