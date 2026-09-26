# 05 — Configuration

All configuration is via **environment variables**, bound in `internal/config` with
stdlib `os.Getenv` + validation. For local dev, `godotenv` loads a `.env` file (a
no-op in prod where the file is absent). `.env.dist` is the committed template;
`.env` is git-ignored and holds real secrets.

## Variables

| Env var | Default | Required | Description |
| --- | --- | --- | --- |
| `MODE` | `live` | no | `live` targets the real Hyundai hosts; `mock` targets the local `mock` server. |
| `MOCK_URL` | `http://localhost:8090` | no | Mock server base URL; used only when `MODE=mock`. Must match the `mock` command's `--addr`. |
| `BLUELINK_USERNAME` | — | if `live` | Hyundai Bluelink EU account email. Ignored in `mock` (a dummy is used). |
| `BLUELINK_PASSWORD` | — | if `live` | Account password (RSA-encrypted before transit). Ignored in `mock`. |
| `BLUELINK_VIN` | — | no | Target vehicle VIN. If empty, the first vehicle is used. |
| `BLUELINK_PIN` | — | no | Account PIN (not needed for read-only status). |
| `POLL_INTERVAL` | `45m` | no | Cached poll cadence (`time.ParseDuration`). Min guard ~5m; 15m documented risky. |
| `FORCE_REFRESH_AT` | `05:00` | no | Daily force-refresh wall-clock time `HH:MM`. Empty disables it. |
| `FORCE_REFRESH_TZ` | `UTC` | no | IANA timezone for `FORCE_REFRESH_AT`. |
| `FORCE_REFRESH_ONLY_WHEN_PLUGGED_IN` | `false` | no | Gate the daily force refresh on plugged-in state. |
| `READY_FAILURE_THRESHOLD` | `3` | no | Consecutive poll failures before `/readyz` flips not-ready. |
| `POLL_MAX_RETRIES` | `3` | no | Transient-error retries per poll before it counts as failed. |
| `MQTT_BROKER_URL` | — | yes | e.g. `mqtt://mosquitto:1883` or `tls://host:8883`. |
| `MQTT_USERNAME` | — | no | MQTT auth username. |
| `MQTT_PASSWORD` | — | no | MQTT auth password. |
| `MQTT_CLIENT_ID` | `hyundai-bluelink-mqtt` | no | MQTT client id. |
| `MQTT_TOPIC_PREFIX` | `hyundai_bluelink` | no | Base topic prefix (VIN appended). |
| `HA_DISCOVERY_PREFIX` | `homeassistant` | no | HA discovery prefix. |
| `DISTANCE_UNIT` | `km` | no | `km` or `mi`. Sets the HA label **and** the published value for both `ev_range` and `odometer` (converted from the API's reported unit, rounded to 1 dp). Controls what is published; HA re-converts imperial distance sensors on a Metric instance — see [03-mqtt-ha-discovery.md](03-mqtt-ha-discovery.md). |
| `TOKEN_STORE` | `memory` (local) / `kube` (image) | no | `memory` or `kube`. |
| `TOKEN_SECRET_NAME` | — | if `kube` | Name of the K8s Secret holding tokens. |
| `HEALTH_ADDR` | `:8080` | no | Listen address for `/healthz` and `/readyz`. |
| `LOG_LEVEL` | `info` | no | `debug`/`info`/`warn`/`error` (`log/slog`). |
| `LOG_FORMAT` | `json` | no | `json` or `text`. |

## Validation rules

- Fail fast (exit non-zero with a clear message) when a required var is missing or a
  duration/integer/boolean/time/timezone/URL fails to parse or an enum value is unknown.
- `MODE` must be `live` or `mock`. In `mock`, both Bluelink hosts resolve to `MOCK_URL`
  and the Bluelink credential requirement is dropped (dummies supplied); in `live`, the
  hosts stay empty so `bluelink.New()` uses the real Hyundai hosts.
- Integer vars (`READY_FAILURE_THRESHOLD`, `POLL_MAX_RETRIES`) must parse with
  `strconv.Atoi` and boolean vars (`FORCE_REFRESH_ONLY_WHEN_PLUGGED_IN`) with
  `strconv.ParseBool`; a malformed value is an error, never a silent fallback to the default.
- `MQTT_BROKER_URL` must be an absolute URL with both a scheme and a host
  (e.g. `mqtt://host:1883`); a bare `host:port` is rejected. Any password in the URL is
  redacted from the error.
- `LOG_LEVEL` must be one of `debug`/`info`/`warn`/`error` and `LOG_FORMAT` one of
  `json`/`text` (case-insensitive).
- `FORCE_REFRESH_AT` must be strict two-digit `HH:MM` (00–23:00–59) when set; `5:00` is
  rejected.
- `FORCE_REFRESH_TZ` must load via `time.LoadLocation`.
- `POLL_INTERVAL` below a floor (default 5m) is rejected to protect against
  rate-limiting and accidental car-wake pressure.
- When `TOKEN_STORE=kube`, `TOKEN_SECRET_NAME` is required and the namespace is read
  from `/var/run/secrets/kubernetes.io/serviceaccount/namespace`.
- Secrets (`BLUELINK_PASSWORD`, `MQTT_PASSWORD`) are never logged; the config's
  `String()`/log representation redacts them.

## Defaults differ by surface

The container image sets `TOKEN_STORE=kube` and `MODE=live` in its default env; local
runs default to `memory` and (in code) `live`. The committed `.env.dist` ships
`MODE=mock` so a fresh `cp .env.dist .env` runs against the local mock out of the box —
the code default and the image stay `live`, so production is unaffected. Everything else
shares defaults across surfaces.
