# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A stdlib-first Go service (go1.26.2) that polls a **Hyundai Inster (2026)** EV via the
**Hyundai Bluelink EU** API and republishes battery/range/charging/location metrics to
**MQTT** with **Home Assistant autodiscovery**. Read-only — it never sends remote
commands and never wakes the car except via an optional once-daily force refresh.
Deployed to Kubernetes; manifests live in a **separate repo** (none here).

## Spec-driven workflow (read this first)

`docs/specs/*.md` are authored first and are the **source of truth**; code is reconciled
against them. `docs/specs/REQUIREMENTS.md` holds stable `REQ-*` IDs (prefixes: `BL`
bluelink client, `DM` domain model, `HA` mqtt/HA, `SC` scheduling, `CF` config, `LC`
lifecycle, `TK` tokens, `TS` testing, `DP` deployment), each pointing at its
implementation file. When you change behaviour, update the relevant spec and `REQ-*`
entry in the same change. The `/spec-reconcile` skill maps `REQ-*` to code and flags
drift — run it after non-trivial changes.

API behaviour is verified against the Python reference lib
`Hyundai-Kia-Connect/hyundai_kia_connect_api`; brand constants (hosts, service id,
secrets, CFB) are copied **verbatim** into `internal/bluelink/constants.go` — do not
"clean them up."

## Commands

```sh
make build        # -> ./bin/hyundai-bluelink-mqtt
make run          # build + serve (poll -> MQTT; honours MODE from .env)
make run-mock     # build + run the standalone mock Bluelink API (add --ccs1 by editing the target)
make test         # unit + integration (excludes the godog features suite)
make test-e2e     # godog acceptance suite (./features/...)
make lint         # golangci-lint (installs pinned binary into ./bin on first use)

gofmt -l $(git ls-files '*.go' | grep -v vendor)   # must be empty
go vet ./... && go build ./...

go test ./internal/config -run TestModeMockCustomURL   # single unit test
go test ./features/...                                  # the whole godog suite (TestFeatures)
```

`go test ./...` runs everything including `./features/...`; `make test` deliberately
excludes features so unit runs stay fast.

## Local run modes (MODE=live|mock)

`internal/config` resolves a single `MODE` switch instead of raw URL overrides:
- `MODE=live` (code + image default): leaves the Bluelink hosts empty so `bluelink.New()`
  falls back to the real Hyundai hosts. Requires `BLUELINK_USERNAME`/`BLUELINK_PASSWORD`.
- `MODE=mock`: points both hosts at `MOCK_URL` (default `http://localhost:8090`, must
  match the `mock` command's `--addr`) and supplies dummy credentials.

`.env.dist` ships `MODE=mock`, so `cp .env.dist .env` + `make run-mock` (terminal 1) +
`make run` (terminal 2, needs an MQTT broker at `:1883`) runs the full pipeline against
the bundled mock with no real credentials. `MQTT_BROKER_URL` is required in both modes.

## Architecture

**CLI (`cmd/main.go` → `internal/cmd`).** Cobra root installs a `SIGTERM`/`SIGINT`
context and loads `.env` via godotenv. Subcommands: `serve` (the service), `mock`
(standalone mock API), `dump` (hidden diagnostic that captures raw API responses).

**Startup flow (`internal/cmd/serve.go`).** config → build `TokenStore` → `bluelink.New`
(auth or token restore, register device, select vehicle by VIN or first) → publish HA
discovery → start `scheduler` → `server` health endpoints. Graceful shutdown publishes a
retained `offline` availability, disconnects MQTT cleanly, then exits.

**`internal/bluelink` — the API client and domain model.** Headless OAuth2 login
(`auth.go`: authorize → certs → RSA-encrypt password → signin 302 → token), request
signing (`stamp.go`, `jwk.go`), token refresh with rotation, and status fetching
(`status.go`). The central branch: a vehicle's `ccuCCS2ProtocolSupport` selects the
parser — `!= 0` → CCS2 (`parse_ccs2.go`), `== 0` → CCS1 (`parse_ccs1.go`). **Both
protocols map into the same `VehicleState` (`model.go`) and publish identically** — when
touching one parser, keep parity in mind. `VehicleState` uses pointer/optional fields to
distinguish "unknown" from "zero"; fields a protocol can't provide stay `nil`.

**`internal/scheduler`.** Cached poll loop at `POLL_INTERVAL` (immediate first run) =
2 API calls (status + location) → one retained state publish. A daily force refresh at
`FORCE_REFRESH_AT`/`FORCE_REFRESH_TZ` (DST-safe recompute, optionally gated on
plugged-in) is the only thing that wakes the car. Cached poll and force refresh are
serialised — never overlap.

**MQTT/HA (`internal/mqtt`, `internal/homeassistant`, `internal/publisher`).** One
retained JSON **state topic** per vehicle; per-entity discovery configs reference it via
`value_template`. `homeassistant/entities.go` is the entity catalogue (device_class /
state_class / units / `entity_category`). Availability is LWT-based (retained `offline`,
`online` on connect). `Publisher` is an interface: autopaho backend in prod, a recording
fake in tests (no broker needed).

**Tokens (`internal/bluelink/tokenstore*.go`).** `TOKEN_STORE=memory` (local) or `kube`
(in-cluster get/patch of a named Secret with optimistic-concurrency retry). The image
defaults to `kube`.

**Testing (`internal/mock`, `features`).** `internal/mock` implements the EU endpoints
with canned Inster fixtures (from `internal/bluelink/testdata/`) and is shared by both
the `mock` subcommand and the godog suite. `features/steps_test.go` (`TestFeatures`)
drives the real `serve` wiring against the mock and the recording `Publisher`, asserting
on MQTT topics/payloads and readiness — this is the end-to-end safety net.

## Conventions

- **stdlib-first.** The only permitted external deps are cobra, autopaho (`paho.golang`),
  godotenv, client-go, and godog (test-only). Reach for stdlib (net/http, crypto, big,
  log/slog) before adding anything.
- Structured logging via `log/slog`; config `String()` redacts secrets — never log
  credentials or full VINs (VIN is masked to last 4 in the status page).
- `_ "time/tzdata"` is imported so `time.LoadLocation` works in the distroless image;
  keep it.
