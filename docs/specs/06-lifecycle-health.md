# 06 — Lifecycle & health

## HTTP probes (`internal/health`)

A stdlib `net/http` server on `HEALTH_ADDR` (default `:8080`) exposes:

- `GET /healthz` — **liveness**. Returns `200 OK` while the process is running and the
  HTTP server is up. It does not depend on MQTT or the Bluelink API (a liveness probe
  must not restart the pod for transient upstream outages).
- `GET /readyz` — **readiness**. Returns `200` when the service is ready to be counted
  as healthy, `503` otherwise. Semantics:
  - Starts **not-ready**.
  - Flips **ready** after the **first successful state publish** (auth + one poll +
    MQTT publish all succeeded).
  - Flips **not-ready** after `READY_FAILURE_THRESHOLD` (default 3) **consecutive**
    poll failures; flips ready again on the next success.

Readiness state is a small concurrency-safe object updated by the scheduler/publisher
and read by the handler.

### Threshold vs poll interval & restart policy

With `POLL_INTERVAL=30m` and threshold `3`, `/readyz` stays ready through ~90m of
upstream trouble before signalling not-ready — deliberately tolerant so Kubernetes does
not thrash a pod during a transient Bluelink outage. Operators size the readiness probe
`periodSeconds`/`failureThreshold` and any restart policy in the **deploy repo** with
this in mind; the app documents the tradeoff but does not self-restart.

## Structured logging

`log/slog` with `LOG_LEVEL`/`LOG_FORMAT`. JSON by default for cluster log ingestion.
No secrets in logs (config redacts). Each poll/force/refresh logs at info/debug with
vehicle id (not VIN in full where avoidable) and outcome.

## Startup sequence

1. Parse+validate config (exit non-zero on error).
2. Build logger, health server (listening immediately so probes work during init).
3. Init `TokenStore`; restore tokens or perform headless login + device registration.
4. List vehicles, select target, record CCS protocol; refuse to start only on fatal
   auth/config errors (a CCS1-only vehicle logs a warning and the service idles without
   publishing state — documented degradation).
5. Connect MQTT (with LWT), publish discovery configs (retained).
6. Start scheduler (immediate first cached poll).

## Graceful shutdown (SIGTERM/SIGINT)

1. Cancel the root context (stops scheduler loops).
2. **Explicitly publish `offline`** to the availability topic (retained), so HA marks
   the device unavailable immediately rather than waiting for LWT/keepalive.
3. `Disconnect()` the MQTT client cleanly (this suppresses the LWT — the explicit
   publish is the intended signal).
4. Shut down the health HTTP server.
5. Persist the latest tokens if dirty.
6. Exit `0` within the shutdown grace period.

LWT remains configured as a backstop for **unexpected** termination (OOM, node loss).
