---
name: go-1.27
description: Use when writing Go for this repo to apply current-toolchain (go1.27) idioms — iterators/range-over-func, slices/maps, log/slog, testing/synctest, t.Context, generics incl. generic methods, stdlib uuid, v2-backed encoding/json. Validated against the installed go1.27.1.
---

# Go 1.27 toolchain idioms

Validated against `go1.27.1` (installed). Prefer these over older patterns.

## Iterators (range-over-func)

- Functions returning `iter.Seq[T]` / `iter.Seq2[K,V]` are `range`-able:
  `for v := range seq { … }`. Use for streaming without materialising slices.
- Bridge to slices with `slices.Collect(seq)`, `slices.Sorted(seq)`,
  `maps.Keys(m)`/`maps.Values(m)` (both return iterators — wrap with
  `slices.Sorted`/`slices.Collect` to get a slice).
- `strings.SplitSeq`/`FieldsSeq` (and `bytes` equivalents) when the parts are only
  iterated once — no intermediate slice.

## `slices` and `maps`

- `slices.Contains`, `ContainsFunc`, `IndexFunc`, `SortFunc`, `SortedFunc`,
  `Concat`, `Equal`, `Clone`, `Compact`, `BinarySearch`.
- `maps.Clone`, `maps.Copy`, `maps.Equal`, `maps.Keys`/`maps.Values` (iterators).
- Builtins `min`, `max`, `clear` are available — use `min(len(a), len(b))` for the
  truncating stamp XOR rather than a hand-rolled loop bound.

## Loops and goroutines

- `for i := range n` for counted loops (`for range n` when the index is unused).
- `sync.WaitGroup.Go(f)` instead of `Add(1)` + `go func(){ defer Done(); … }()`.

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
  use it to settle work that is already runnable.
- `synctest.Sleep(d)` ≡ `time.Sleep(d); synctest.Wait()` — the idiom for "advance the
  clock and let everything scheduled in that window run".
- Ideal for the scheduler: assert the daily force-refresh fires at the right wall-clock
  time and the cached poll ticks on interval, with zero wall-clock delay.
- Keep bubbles self-contained: no real network/processes. For an HTTP dependency use
  `httptest.NewTestServer(t, h)` — an in-memory server that is safe inside a bubble —
  or inject the client.

## Generics

- Use type params for small utilities (`Optional[T]`), not for everything. A concrete
  type is clearer when there's one caller.
- Methods may declare their own type parameters (`func (s S) Map[T any](…)`); still
  prefer a plain generic function unless method syntax reads better at the call site.
- Constraints from `cmp.Ordered` for comparisons; `any` for pass-through.

## Pointer to a value (`new(expr)`)

- `new` accepts any expression, so `new(87.0)` yields a `*float64` — do not add
  `ptr[T]`, `fp`, `bp` or `boolPtr` helpers; the ones this repo had were removed.
- Mind untyped constants: `new(62)` is `*int`, write `new(62.0)` for `*float64`
  and `new(int64(62))` for `*int64`.

## JSON

- `encoding/json` is now implemented on top of `encoding/json/v2`. Behaviour is
  unchanged but **error text may differ** — never assert on json error strings.
- `encoding/json/v2` and `encoding/json/jsontext` are stable, but this repo stays on
  `encoding/json`: the fixed HA payload struct and the `map[string]any` CCS2 walker
  gain nothing from v2. Do not mix the two APIs in one package.
- v2 strictness (rejects invalid UTF-8, duplicate keys, case-sensitive field match)
  applies only to the v2 API; v1 keeps its lenient defaults.
- For the CCS2 tree, unmarshal into `map[string]any` and walk with a safe dotted-path
  helper; reserve typed structs for the small, stable envelope (`retCode`, `resMsg`).

## stdlib additions

- `uuid.NewV4()` / `NewV7()` / `Parse()` — no hand-rolled uuid generators.
- `url.URL.Clone()` and `url.Values.Clone()` for safe copies.
- `strings.CutLast` / `bytes.CutLast` — split on the last separator.
- `http.Server.MaxHeaderValueCount` caps repeated header values (default applies).
- HTTP/1 `Response.Body.Close()` auto-drains a small remainder so the connection can
  be reused; keep an explicit drain where the cap may not cover the body.
- `time` channels are always unbuffered (`asynctimerchan` setting is gone); a stopped
  or reset timer never delivers a stale value.

## Misc

- `errors.Join` to combine multiple failures; `errors.Is`/`As` for inspection.
- `context.WithTimeoutCause`/`WithCancelCause` when a specific cancellation reason helps
  debugging.
- Run `go vet ./...` — it catches slog key/value mismatches and loop/context misuse.
  `go test` also runs the `stdversion` vet check against the `go.mod` version, so a
  newer API than the declared version fails the test run.
- After a toolchain bump run `go fix ./...`: it applies the modernizers and inlines
  `//go:fix inline` helpers — delete the helpers it leaves unused before linting.
- macOS 13 (Ventura) or newer is required to build and run.
