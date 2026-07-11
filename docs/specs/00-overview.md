# 00 — Overview

## Purpose

A small, stdlib-first Go service that polls a **Hyundai Inster (2026)** EV via the
**Hyundai Bluelink EU** API and republishes selected metrics to **MQTT** with
**Home Assistant (HA) MQTT autodiscovery**, so the car appears in HA as a device with
battery, range, charging and location entities — no manual HA configuration.

Deployed to Kubernetes (manifests live in a **separate repo**; none in this repo).

## Principles

- **Spec-driven.** These `docs/specs/*` are authored first and are the source of truth.
  Code is reconciled against `REQUIREMENTS.md` via `/spec-reconcile`.
- **stdlib-first.** Latest Go (**go1.26.2**). Only unavoidable external deps:
  MQTT client (`paho.golang`), CLI (`cobra`), `.env` loader (`godotenv`), in-cluster
  token store (`client-go`) and BDD runner (`godog`, test-only). All core logic —
  HTTP, crypto, JSON, big-int, scheduling, logging — is stdlib.
- **Read-only.** The service issues only read/status endpoints. It never sends remote
  commands (no lock/unlock, climate, or charge control). No control-token flow.
- **Never wake the car by default.** Regular polling reads **cached** state only.
  Waking the car (a force refresh) happens at most once per day on a schedule.
- **Kube-friendly.** HTTP health/readiness probes, env-based config, structured
  logging (`log/slog`), graceful shutdown, non-root minimal (distroless) image.
- **CCS2 and CCS1 both supported.** Vehicles report their protocol via
  `ccuCCS2ProtocolSupport` (newer report CCS2, older report CCS1). Status parsing is
  isolated per protocol; both map into the same `VehicleState` and publish identically.

## High-level flow

1. Load + validate config from environment (`godotenv` loads a local `.env`).
2. `internal/bluelink` authenticates (headless OAuth2) or restores tokens from the
   configured `TokenStore`, registers a device, lists vehicles, selects the target
   (by `VIN` or first) and records its CCS protocol flag.
3. Publish HA **discovery** configs once on startup (retained, QoS 1).
4. `internal/scheduler` polls **cached** status + location every `POLL_INTERVAL`
   (default 45m) and publishes a retained JSON state document; a daily **force**
   refresh runs at `FORCE_REFRESH_AT` (optionally gated on plugged-in).
5. `internal/server` exposes an HTML status page on `/` plus `/healthz` (liveness)
   and `/readyz` (readiness).
6. On `SIGTERM`, publish `offline` availability (retained), `Disconnect()` cleanly,
   exit 0.

## Non-goals

- Remote commands / vehicle control of any kind.
- Multiple vehicles per process (single target vehicle; multi-vehicle is out of scope).
- Kubernetes manifests, Helm charts, or MQTT broker provisioning.
- Non-EU Bluelink regions and non-EV drivetrains (ICE/HEV parsing is out of scope,
  though the vehicle list tolerates their presence).

## Reference

API behaviour is verified against the Python reference library
`Hyundai-Kia-Connect/hyundai_kia_connect_api` (`KiaUvoApiEU.py`, `ApiImplType1.py`,
`utils.py`, `const.py`). Brand constants are copied verbatim into
`internal/bluelink/constants.go`.

See the sibling specs:
- `01-bluelink-api.md` — EU API endpoints, auth, stamp, protocol branch.
- `02-domain-model.md` — `VehicleState` and CCS2 field mapping.
- `03-mqtt-ha-discovery.md` — MQTT topics and HA discovery design.
- `04-polling-scheduling.md` — poll loop and daily force refresh.
- `05-config.md` — environment variables.
- `06-lifecycle-health.md` — probes, readiness, graceful shutdown.
- `07-testing.md` — mock server, godog, unit tests.
- `08-deployment.md` — container, token persistence, RBAC.
- `REQUIREMENTS.md` — traceable requirement IDs (`REQ-*`).
