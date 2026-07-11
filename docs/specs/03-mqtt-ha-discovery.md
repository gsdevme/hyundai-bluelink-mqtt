# 03 — MQTT & Home Assistant autodiscovery

The `home-assistant-mqtt-discovery` skill is the authoritative source of the rules
below; `internal/homeassistant` implements them.

## Topic design

- **Discovery prefix** `homeassistant` (configurable via `HA_DISCOVERY_PREFIX`).
- **Base topic** per vehicle: `hyundai_bluelink/{vin}` (configurable via `MQTT_TOPIC_PREFIX`).
- **Single retained JSON state topic** per vehicle: `{base}/state`. All numeric/boolean
  entities read from it via `value_template` (`{{ value_json.<key> }}`). This is the
  HA-recommended shared-state pattern: fewer topics, atomic updates.
- **Availability topic**: `{base}/availability` — payload `online`/`offline`, retained.
- **Per-entity discovery** topics: `homeassistant/<component>/{vin}_{key}/config`,
  retained. `unique_id` and `object_id` = `{vin}_{key}`.
- **Device-tracker** needs two extra topics:
  - `{base}/tracker/state` — `home` / `not_home` / `None`.
  - `{base}/tracker/attributes` — JSON `{latitude, longitude, gps_accuracy, source_type}`.

All discovery, state, availability, and tracker publishes are **`retain=true`, QoS 1**
so HA survives a broker restart without a pod restart.

**Rejected alternative:** HA 2023.8+ device-based (single) discovery — no
`device_tracker` support and less mature tooling. We use per-entity discovery with a
shared state topic and a shared `device` block instead.

## Shared discovery fields

Every config payload includes:
- `~` (abbreviation for base topic) = `hyundai_bluelink/{vin}`, so `state_topic` can be
  `~/state`, `availability_topic` `~/availability`.
- `device` block: `identifiers: [vin]`, `manufacturer: "Hyundai"`,
  `model: "Inster"` (from `Vehicle.Model`), `name: <nickname or "Hyundai Inster">`.
- `availability_topic`, `payload_available: online`, `payload_not_available: offline`.
- `unique_id`, `object_id`, `name`.

## Entity catalogue

`sensor`:

| key | name | device_class | state_class | unit | category |
| --- | --- | --- | --- | --- | --- |
| `ev_battery_percentage` | Battery | `battery` | `measurement` | `%` | — |
| `ev_range` | Range | `distance` | `measurement` | `km`/`mi` | — |
| `charging_power` | Charging power | `power` | `measurement` | `kW` | — |
| `battery_12v` | 12V battery | `battery` | `measurement` | `%` | diagnostic |
| `odometer` | Odometer | `distance` | `total` | `km`/`mi` | — |
| `charge_limit_ac` | AC charge limit | — | `measurement` | `%` | diagnostic |
| `charge_limit_dc` | DC charge limit | — | `measurement` | `%` | diagnostic |
| `est_charge_time` | Charge time remaining | `duration` | `measurement` | `min` | — |
| `outside_temp` | Outside temperature | `temperature` | `measurement` | `°C` | — |
| `ev_battery_soh` | Battery health | — | `measurement` | `%` | diagnostic |
| `last_updated` | Last updated | `timestamp` | — | — | diagnostic |

Both `ev_range` and `odometer` units are set from `DISTANCE_UNIT` (`km`/`mi`, default
`km`). For `ev_range` it is a label only — the value is published as the car reports it,
unconverted. For `odometer` the published value is **converted** to `DISTANCE_UNIT`
(rounded to 1 dp) because the API reports the odometer in km regardless of the driver's
display unit, so a label-only change would mislabel the number.

`odometer` uses `state_class: total` (not `total_increasing`) to tolerate resets.

`binary_sensor`:

| key | name | device_class | payload_on/off | category |
| --- | --- | --- | --- | --- |
| `charging` | Charging | `battery_charging` | `true`/`false` | — |
| `plugged_in` | Plugged in | `plug` | `true`/`false` | — |
| `locked` | Doors | `lock` | see note | — |
| `tire_warning` | Tire pressure | `problem` | `true`/`false` | — |
| `charge_port_door` | Charge port | `door` | `true`/`false` | diagnostic |

Binary sensors read booleans from the JSON state topic via `value_template`
(`{{ 'ON' if value_json.<key> else 'OFF' }}`), `payload_on: ON`, `payload_off: OFF`.
For `locked`, HA `lock` device_class expects `ON`=unlocked/`OFF`=locked — the template
inverts (`ON` when not locked).

`device_tracker` (key `location`):
- `state_topic: ~/tracker/state` (`home`/`not_home`/`None`; a real value, not coords).
- `json_attributes_topic: ~/tracker/attributes` with `source_type: gps`, `latitude`,
  `longitude`. HA derives home/away from the coordinates against its zones; we publish
  `not_home` as a neutral default and let attributes drive the map position.

`entity_category: diagnostic` on: `battery_12v`, `charge_limit_ac`, `charge_limit_dc`,
`ev_battery_soh`, `last_updated`, `charge_port_door`.

## Availability (LWT)

- The MQTT client registers a **Last Will** on `{base}/availability` = `offline`
  (retained), so an unexpected disconnect flips every entity unavailable.
- After a successful connect, publish `online` (retained).
- On graceful shutdown, **explicitly** publish `offline` (retained) then
  `Disconnect()` — LWT is a backstop, not the normal path.

## Payloads

- The state document is a single JSON object with the `VehicleState` keys above (snake
  case matching the entity `value_template`s). Absent/unknown fields are omitted or
  `null` (HA renders "unknown").
- Discovery payloads use HA **abbreviations** where standard (`~`, `stat_t`, `avty_t`,
  `dev`, `uniq_id`, `dev_cla`, `stat_cla`, `unit_of_meas`, `val_tpl`, `ent_cat`) to
  keep payloads compact; unabbreviated keys are acceptable too (skill documents both).
