# 07 — Testing strategy

TDD throughout, plus a godog acceptance suite that drives the real service against a
mock Bluelink API and asserts on recorded MQTT output.

## Interfaces enabling tests

- **`Publisher`** interface (publish state / discovery / availability). Prod backend =
  `autopaho`; test backend = a **recording fake** (`map[topic]payload`, last-write
  wins, tracks retain/QoS). No broker needed in tests.
- **`TokenStore`** interface. Tests use `memoryStore`.
- **`Clock`** / injected `now` so schedules are deterministic (`testing/synctest`).

## Mock Bluelink server (`internal/mock`)

Implements the EU endpoints with canned **Inster CCS2** fixtures, shared by godog and
the standalone `mock` subcommand:

- `GET /auth/api/v2/user/oauth2/authorize` → sets a cookie, 200.
- `GET /auth/api/v1/accounts/certs` → JWK for a **test RSA key** generated at startup
  (so the server can, if needed, decrypt; the service only needs a valid-looking key).
- `POST /auth/account/signin` → 302 with `?code=<test>` in `Location`.
- `POST /auth/api/v2/user/oauth2/token` → `{token_type, access_token, refresh_token,
  expires_in}`; supports both `authorization_code` and `refresh_token` grants (the
  refresh grant returns a rotated refresh token and a fresh access token).
- `POST notifications/register` → `{resMsg:{deviceId}}`.
- `GET vehicles` → one Inster with `ccuCCS2ProtocolSupport != 0`.
- `GET vehicles/{id}/ccs2/carstatus/latest` → cached CCS2 fixture.
- `GET vehicles/{id}/ccs2/carstatus` → force CCS2 fixture (can differ, e.g. fresher
  battery %, to prove force refresh took effect).
- `GET vehicles/{id}/location/park` → `{resMsg:{coord:{lat,lon}, time}}`.

Fixtures live under `internal/bluelink/testdata/` (captured from the real API by the
probe harness, credentials/VIN redacted; hand-authored until the probe runs).
Configurable knobs let scenarios flip plugged-in / charging / expired-token.

## godog scenarios (`features/*.feature`)

Drive the real service with config pointed at the mock base URL; assert on recorded
MQTT topics/payloads and readiness:

1. **Startup** — authenticate, register device, publish discovery configs (assert the
   expected `homeassistant/.../config` topics exist, retained).
2. **Cached poll** — publish battery/charging state to the retained state topic.
3. **Token refresh** — access token expired ⇒ refresh grant used ⇒ poll still succeeds
   ⇒ rotated refresh token saved to the store.
4. **Daily force refresh** — fires at the scheduled time (driven via injected clock);
   uses the force endpoint; **skipped when unplugged** if the plugged-in gate is on.
5. **Graceful degradation** — API error keeps the last published state and, after the
   threshold, flips `/readyz` to 503; recovery flips it back.
6. **Graceful shutdown** — SIGTERM publishes `offline` (retained) before disconnect.

## Unit tests (stdlib `testing`)

- `stamp.go` — known CFB + fixed timestamp → expected base64 (truncation behaviour).
- `jwk.go` — JWK(n,e) → `rsa.PublicKey`; round-trip encrypt/decrypt with a test key.
- `parse_ccs2.go` — table tests over the captured fixture: battery %, range+unit,
  charging/plugged booleans, 12V normalisation (incl. `255` sentinel → nil), temp
  unit conversion, odometer, lock AND-reduction, tire OR-reduction, location override.
- `internal/homeassistant` — discovery payload building: topics, `~`/abbreviations,
  device block, availability, device_class/state_class per entity.
- `internal/config` — validation (missing required, bad duration/time/tz, redaction).

## Commands

- `go build ./... && go vet ./...` clean.
- `make lint` — `golangci-lint` (default linters + gofmt) clean.
- `go test ./...` — unit green.
- `go test ./features/...` — godog acceptance green.
- The `mock` subcommand + `serve` with `MODE=mock` against a local Mosquitto for a real
  end-to-end smoke test into Home Assistant (`.env.dist` ships `MODE=mock`).
