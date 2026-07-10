# Requirements

Stable, traceable requirement IDs. `/spec-reconcile` maps each `REQ-*` to its
implementation and flags drift. Prefix meanings: `BL` Bluelink client, `DM` domain
model, `HA` MQTT/Home Assistant, `SC` scheduling, `CF` config, `LC` lifecycle/health,
`TK` tokens, `TS` testing, `DP` deployment, `AS` Claude assets.

## Bluelink client (`internal/bluelink`)

- **REQ-BL-01** Brand constants (hosts, `ccsp-service-id`, `client_secret`, `app id`,
  CFB, `redirect_uri`, push type) match the reference lib exactly. → `constants.go`
- **REQ-BL-02** Headless OAuth2 login: authorize → certs → signin (302 code) → token,
  with cookie jar and the `_CCS_APP_AOS` form UA. → `auth.go`
- **REQ-BL-03** JWK `{n,e}` → `rsa.PublicKey` via `math/big` (base64url). → `jwk.go`
- **REQ-BL-04** Password encrypted with RSA PKCS#1 v1.5, hex-encoded. → `auth.go`
- **REQ-BL-05** Stamp = base64(CFB XOR `{APP_ID}:{unix}`), byte-wise, truncating. → `stamp.go`
- **REQ-BL-06** Token refresh via `grant_type=refresh_token`; persists rotated refresh
  token; falls back to full login on failure. → `auth.go`
- **REQ-BL-07** `ConsentRequiredError` surfaced when signin redirects to
  `/web/v1/user/authorization`. → `auth.go`
- **REQ-BL-08** Device registration `POST notifications/register` → `deviceId`. → `client.go`
- **REQ-BL-09** Vehicle list `GET vehicles`; capture `ccuCCS2ProtocolSupport` and
  identity fields; select target by VIN or first. → `client.go`
- **REQ-BL-10** Cached status `GET ccs2/carstatus/latest` (no wake). → `status.go`
- **REQ-BL-11** Force status `GET ccs2/carstatus` (GET, wakes car). → `status.go`
- **REQ-BL-12** Park location `GET location/park`; overrides embedded stale location. → `status.go`
- **REQ-BL-13** Authenticated SPA headers on every call (Authorization, service-id,
  application-id, Stamp, device-id, Ccuccs2protocolsupport, Host, UA). → `client.go`
- **REQ-BL-14** Protocol branch on `ccuCCS2ProtocolSupport`: `!= 0` → CCS2
  (`fetchStatus`), `== 0` → CCS1 (`fetchStatusCCS1`). Both map into `VehicleState` and
  publish identically. → `status.go`, `parse_ccs1.go`
- **REQ-BL-15** Read-only: no control/command endpoints or control-token flow exist.
- **REQ-BL-16** CCS1 cached status `GET vehicles/{id}/status/latest` (no wake, envelope
  `resMsg.vehicleStatusInfo.{vehicleStatus, vehicleLocation, odometer}` with embedded
  location) and force status `GET vehicles/{id}/status` (wakes car, `resMsg` *is* the
  vehicleStatus with no wrapper/location/odometer). The force path resolves location from
  `/location/park` (`resMsg.gpsDetail`); odometer stays nil until the next cached poll.
  → `status.go`

## Domain model (`internal/bluelink`)

- **REQ-DM-01** `Vehicle` type with identity + `CCS2ProtocolSupport`/`IsCCS2()`. → `model.go`
- **REQ-DM-02** `VehicleState` type with the fields in `02-domain-model.md`, using
  optional/pointer fields to distinguish unknown from zero. → `model.go`
- **REQ-DM-03** CCS2 → `VehicleState` mapping per the field table. → `parse_ccs2.go`
- **REQ-DM-04** Charging bool derived from `Charging.RemainTime > 0`. → `parse_ccs2.go`
- **REQ-DM-05** Plugged-in from `ConnectorFastening.State`. → `parse_ccs2.go`
- **REQ-DM-06** 12V SoC normalisation (reliability flag, range guard, `255` → nil). → `parse_ccs2.go`
- **REQ-DM-07** Distance/temperature unit index normalisation (km/mi, °C). → `parse_ccs2.go`
- **REQ-DM-08** Lock = AND of door locks; tire warning = OR of per-axle + all flags. → `parse_ccs2.go`
- **REQ-DM-09** Safe dotted-path lookup that never panics on absent keys. → `parse_ccs2.go`
- **REQ-DM-10** CCS1 → `VehicleState` mapping at full parity, reusing the CCS2 path
  helpers; fields CCS1 does not expose (SoH, charge-port door, charging power, measured
  temperatures) stay `nil`. → `parse_ccs1.go`

## MQTT & Home Assistant (`internal/mqtt`, `internal/homeassistant`, `internal/publisher`)

- **REQ-HA-01** Per-entity discovery topics `homeassistant/<component>/{vin}_{key}/config`,
  retained, QoS 1. → `homeassistant/discovery.go`
- **REQ-HA-02** Single retained JSON state topic per vehicle; entities read via
  `value_template`. → `publisher.go`, `homeassistant/discovery.go`
- **REQ-HA-03** Shared `device` block (VIN identifier, Hyundai, Inster) + `~` base-topic
  abbreviation. → `homeassistant/discovery.go`
- **REQ-HA-04** Entity catalogue (sensors, binary_sensors, device_tracker) with correct
  `device_class`/`state_class`/units/categories per `03-mqtt-ha-discovery.md`. → `homeassistant/entities.go`
- **REQ-HA-05** `device_tracker` uses `home/not_home/None` state + `json_attributes_topic`
  with `source_type: gps` and lat/lon. → `homeassistant/entities.go`, `publisher.go`
- **REQ-HA-06** Availability via LWT (retained `offline`); `online` published on connect;
  every entity references the availability topic. → `mqtt/client.go`, `homeassistant/discovery.go`
- **REQ-HA-07** `entity_category: diagnostic` on SoH, 12V, charge limits, charge-port,
  last-updated. → `homeassistant/entities.go`
- **REQ-HA-08** All discovery/state/availability publishes retained + QoS 1. → `mqtt/client.go`, `publisher.go`
- **REQ-HA-09** `Publisher` interface with autopaho prod backend + recording fake for
  tests. → `publisher.go`, `mqtt/client.go`

## Scheduling (`internal/scheduler`)

- **REQ-SC-01** Cached poll loop at `POLL_INTERVAL` (default 30m), immediate first run. → `scheduler.go`
- **REQ-SC-02** Each cached poll = 2 API calls (status + location) → one state publish. → `scheduler.go`
- **REQ-SC-03** Transient errors retry up to `POLL_MAX_RETRIES` with exponential backoff. → `scheduler.go`
- **REQ-SC-04** Daily force refresh at `FORCE_REFRESH_AT`/`FORCE_REFRESH_TZ` via
  `time.LoadLocation`; DST-safe daily recompute. → `scheduler.go`
- **REQ-SC-05** Force refresh optionally gated on plugged-in (cached pre-check). → `scheduler.go`
- **REQ-SC-06** `_ "time/tzdata"` imported so zones work in distroless. → `main.go`/`scheduler.go`
- **REQ-SC-07** Cached poll and force refresh never overlap (serialised). → `scheduler.go`

## Config (`internal/config`)

- **REQ-CF-01** All env vars in `05-config.md` bound with defaults. → `config.go`
- **REQ-CF-02** Fail-fast validation (required present; duration/time/tz/URL parse). → `config.go`
- **REQ-CF-03** `POLL_INTERVAL` floor enforced. → `config.go`
- **REQ-CF-04** Secrets redacted in logs/String(). → `config.go`
- **REQ-CF-05** `godotenv` loads local `.env`; `.env.dist` template committed. → `cmd/root.go`, `.env.dist`

## Lifecycle & health (`internal/server`, `cmd`, `main.go`)

- **REQ-LC-01** `/healthz` liveness always-ok while running. → `internal/server`
- **REQ-LC-02** `/readyz` ready after first successful publish. → `internal/server`
- **REQ-LC-03** `/readyz` not-ready after `READY_FAILURE_THRESHOLD` consecutive
  failures; recovers on success. → `internal/server`, `scheduler.go`
- **REQ-LC-04** Structured `log/slog` logging; level/format configurable; no secrets. → `cmd/root.go`
- **REQ-LC-05** Graceful shutdown: explicit retained `offline` publish, clean
  `Disconnect()`, then exit. → `cmd/serve.go`
- **REQ-LC-06** Status/health server listens during init so probes work at startup. → `cmd/serve.go`
- **REQ-LC-07** Root `/` HTML status page: service, readiness, uptime, vehicle
  (VIN masked to last 4), schedule, Go version; always `200`, no secrets. → `internal/server`

## Tokens (`internal/bluelink/tokenstore.go`)

- **REQ-TK-01** `TokenStore` interface `Load`/`Save`. → `tokenstore.go`
- **REQ-TK-02** `memoryStore` backend. → `tokenstore.go`
- **REQ-TK-03** `kubeSecretStore` backend: in-cluster get/patch of the named Secret. → `tokenstore.go`
- **REQ-TK-04** Optimistic concurrency via `resourceVersion`; retry once on conflict. → `tokenstore.go`
- **REQ-TK-05** Tokens loaded on startup; saved after each refresh. → `client.go`/`auth.go`
- **REQ-TK-06** Backend selected by `TOKEN_STORE`. → `tokenstore.go`/`config.go`

## Testing (`internal/mock`, `features`)

- **REQ-TS-01** Mock server implements all EU endpoints with Inster CCS2 fixtures. → `mock/server.go`
- **REQ-TS-02** Mock is shared by godog and the standalone `mock` subcommand. → `mock/server.go`, `cmd/mock.go`
- **REQ-TS-03** godog scenarios: startup discovery, cached poll state, token refresh,
  daily force refresh (+ unplugged skip), graceful degradation, graceful shutdown. → `features/*.feature`
- **REQ-TS-04** Unit tests for `stamp`, `jwk`, `parse_ccs2`, discovery, config. → `*_test.go`
- **REQ-TS-05** Recording fake `Publisher` — no broker in tests. → `features/steps_test.go`

## Deployment (`Dockerfile`, `go.mod`)

- **REQ-DP-01** Multi-stage Dockerfile: static `CGO_ENABLED=0`, distroless nonroot. → `Dockerfile`
- **REQ-DP-02** Image default env `TOKEN_STORE=kube`; default `CMD serve`. → `Dockerfile`
- **REQ-DP-03** Module path `github.com/gsdevme/hyundai-bluelink-mqtt`. → `go.mod`

## Claude assets

- **REQ-AS-01** Skill `home-assistant-mqtt-discovery`. → `.claude/skills/...`
- **REQ-AS-02** Skill `effective-go`. → `.claude/skills/...`
- **REQ-AS-03** Skill `go-1.26`. → `.claude/skills/...`
- **REQ-AS-04** Command `/spec-reconcile`. → `.claude/commands/spec-reconcile.md`
