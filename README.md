# hyundai-bluelink-mqtt

Read-only Go service that polls a **Hyundai Inster (2026)** EV via the **Hyundai
Bluelink EU** API and republishes battery, range, charging and location metrics to
**MQTT** with **Home Assistant autodiscovery**. Designed for Kubernetes (manifests live
in a separate repo).

- **stdlib-first**, latest Go (`go1.26.2`). Only unavoidable deps: cobra, autopaho,
  godotenv, client-go, godog (test-only).
- **Read-only** — no remote commands. Regular polling reads **cached** state only and
  never wakes the car; an optional daily force-refresh wakes it once.
- **CCS2 and CCS1** protocols; the vehicle's `ccuCCS2ProtocolSupport` selects the parser
  and both publish the same state.

See [`docs/specs/`](docs/specs) for the full design and
[`docs/specs/REQUIREMENTS.md`](docs/specs/REQUIREMENTS.md) for traceable requirement IDs.

## Quick start (local)

`.env.dist` ships `MODE=mock`, so a fresh clone runs the full pipeline against the
bundled mock — no Bluelink credentials needed:

```sh
cp .env.dist .env                # ships MODE=mock (credentials ignored)
go run ./cmd mock                # terminal 1: mock Bluelink API (add --ccs1 for CCS1)
go run ./cmd serve               # terminal 2: poll -> MQTT (needs a broker at :1883)
```

Set `MODE=live` (and real `BLUELINK_USERNAME`/`BLUELINK_PASSWORD`) in `.env` to poll the
real car instead. For a non-default mock port, run `mock --addr :PORT` and set
`MOCK_URL` to match.

## Configuration

All via environment variables (a local `.env` is loaded automatically). See
[`docs/specs/05-config.md`](docs/specs/05-config.md). `MQTT_BROKER_URL` is always
required; `BLUELINK_USERNAME`/`BLUELINK_PASSWORD` are required only when `MODE=live`.
`MODE` selects the target (`live` real API / `mock` local mock).

## Status & health

Served by `internal/server` on `HEALTH_ADDR` (default `:8080`):

- `GET /` — HTML status page (service, readiness, uptime, selected vehicle with the
  VIN masked to its last 4, schedule, Go version). Never shows credentials.
- `GET /healthz` — liveness (always OK while running).
- `GET /readyz` — readiness (ready after first successful publish; not-ready after
  `READY_FAILURE_THRESHOLD` consecutive poll failures).

## Tokens

Stateless-friendly: tokens persist in a Kubernetes Secret the pod mutates
(`TOKEN_STORE=kube`, `TOKEN_SECRET_NAME`), or in-process for local dev
(`TOKEN_STORE=memory`). See [`docs/specs/08-deployment.md`](docs/specs/08-deployment.md).

## Tests

```sh
go test ./...              # unit + integration
go test ./features/...     # godog acceptance suite
```

## Probe harness

`_probe/` (git-ignored) hits the **real** API with your `.env` credentials to verify the
flow and capture raw JSON for fixtures:

```sh
go run ./_probe
```

Review/redact the captured responses under `_probe/captured/` before promoting any to
`internal/bluelink/testdata/`.

## Container

Multi-stage build → `gcr.io/distroless/static:nonroot`, static `CGO_ENABLED=0` binary
with embedded tzdata, runs as non-root.

```sh
docker build -t hyundai-bluelink-mqtt .
```
