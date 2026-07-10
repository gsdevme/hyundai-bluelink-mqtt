# 04 — Polling & scheduling

`internal/scheduler` owns two time-driven behaviours: a **cached poll loop** and a
**daily force refresh**. Both feed parsed `VehicleState` to the publisher.

## Cached poll loop (default: never wakes the car)

- Interval `POLL_INTERVAL`, default **30m**. 15m is documented as risky (more load /
  closer to rate limits); anything below is discouraged.
- Fires immediately once on startup, then every interval.
- Each poll does **2 API calls**: cached CCS2 status
  (`ccs2/carstatus/latest`) + park location (`location/park`), merges them into one
  `VehicleState`, publishes the retained JSON state document.
- **Transient errors** (network, 5xx, context deadline that is not shutdown) retry
  **2–3×** with exponential backoff (e.g. 1s, 2s, 4s) before the poll counts as a
  failure. Auth `401`/expired-token triggers a token refresh (or full login) and one
  retry, out of band from the transient backoff.
- A failed poll does **not** clear the last published state (graceful degradation):
  HA keeps the last-known values; readiness reacts (see `06-lifecycle-health.md`).

## Daily force refresh (wakes the car)

- At `FORCE_REFRESH_AT` (default `05:00`, `HH:MM`) in timezone `FORCE_REFRESH_TZ`
  (default `UTC`; e.g. `Europe/London`), resolved via `time.LoadLocation`. The binary
  imports `_ "time/tzdata"` so zone data is available inside distroless.
- Computes the next occurrence of that wall-clock time in the zone, sleeps until then
  (context-aware), fires, then reschedules for the next day. DST is handled by
  recomputing against the zone each day rather than adding a fixed 24h.
- Force refresh calls `ccs2/carstatus` (**GET**, wakes the car) + `location/park`,
  publishes the resulting state.
- **Plugged-in gate:** if `FORCE_REFRESH_ONLY_WHEN_PLUGGED_IN=true`, first do a
  **cached** read; if `PluggedIn` is not true, **skip** the force refresh this day
  (log it) and reschedule. This avoids waking/draining a car that is parked and
  unplugged.

## Concurrency & clock

- One goroutine per behaviour, both selecting on the service `context.Context` for
  shutdown. A shared mutex (or single-owner actor) serialises API calls so a force
  refresh and a cached poll never overlap on the HTTP client / token refresh.
- Time is injected (a `Clock`/`now func() time.Time` or `time.After`) so tests can use
  `testing/synctest` to drive the schedule deterministically without real sleeps.

## Publishing outcome

Every successful fetch (cached or forced) results in exactly one retained state publish.
The first successful publish flips `/readyz` to ready. Consecutive poll failures past
`READY_FAILURE_THRESHOLD` flip it back to not-ready.
