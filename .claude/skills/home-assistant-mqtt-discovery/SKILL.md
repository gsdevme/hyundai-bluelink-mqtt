---
name: home-assistant-mqtt-discovery
description: Use when building or reviewing MQTT payloads and topics for Home Assistant autodiscovery (discovery configs, device blocks, availability/LWT, device_class/state_class, shared-state-topic pattern). Source of truth for internal/homeassistant.
---

# Home Assistant MQTT autodiscovery

Home Assistant (HA) can create entities automatically from **retained** MQTT
"discovery" config messages. This skill captures the rules this project follows.

## Topic structure

Per-entity discovery topic:

```
<discovery_prefix>/<component>/<node_id>/<object_id>/config
```

- `discovery_prefix` — default `homeassistant`.
- `component` — `sensor`, `binary_sensor`, `device_tracker`, etc.
- We use `<discovery_prefix>/<component>/<vin>_<key>/config` (object_id only form).
- Publish **retained, QoS 1** so HA rebuilds entities after a broker restart with no
  producer restart. An empty retained payload on the same topic **deletes** the entity.

## Shared state topic pattern (preferred)

Instead of one topic per value, publish **one retained JSON document** per device and
point every entity at it with a `value_template`:

```json
{ "state_topic": "hyundai_bluelink/<vin>/state",
  "value_template": "{{ value_json.ev_battery_percentage }}" }
```

Benefits: fewer topics, atomic multi-value updates, one retained message to restore
full state. Booleans render via `{{ 'ON' if value_json.charging else 'OFF' }}` with
`payload_on: ON` / `payload_off: OFF`.

## The `~` base-topic abbreviation

Set `~` once and reference it with a leading `~`:

```json
{ "~": "hyundai_bluelink/<vin>",
  "state_topic": "~/state",
  "availability_topic": "~/availability" }
```

## Shared device block

Give every entity the **same** `device` block so HA groups them under one device:

```json
{ "device": {
    "identifiers": ["<vin>"],
    "manufacturer": "Hyundai",
    "model": "Inster",
    "name": "Hyundai Inster" } }
```

`unique_id` and `object_id` = `<vin>_<key>` (stable, unique). `unique_id` is required
for the entity to be editable in the HA UI.

## Availability (LWT)

- Register a **Last Will** on `~/availability` = `offline`, retained.
- Publish `online` (retained) after connect.
- On graceful stop, publish `offline` (retained) **explicitly**, then disconnect.
- Every entity sets `availability_topic: ~/availability`, `payload_available: online`,
  `payload_not_available: offline`. All entities go unavailable together on disconnect.

## device_class / state_class cheatsheet

- `sensor` numeric: pick a `device_class` (`battery`, `distance`, `power`,
  `temperature`, `duration`, `timestamp`) and usually `state_class: measurement`.
- Cumulative counters: `state_class: total_increasing`. For an **odometer that may
  reset** (unit change, replacement), use `state_class: total` to avoid spurious spikes.
- `timestamp` sensors take an ISO-8601/RFC3339 string; no unit.
- `binary_sensor`: `device_class` such as `battery_charging`, `plug`, `problem`,
  `door`, `lock`. Note `lock`: `ON` = unlocked, `OFF` = locked (invert in the template).
- `device_tracker`: `state` must be `home` / `not_home` / a zone name / `None` — **not**
  coordinates. Publish coordinates via `json_attributes_topic`
  (`{latitude, longitude, gps_accuracy, source_type: gps}`).
- `entity_category: diagnostic` demotes non-primary entities (SoH, 12V, charge limits,
  charge-port, last-updated) out of the main controls area.

## Common abbreviations (optional, compact payloads)

`~`, `stat_t` (state_topic), `avty_t` (availability_topic), `json_attr_t`
(json_attributes_topic), `dev` (device), `uniq_id`, `obj_id`, `name`, `dev_cla`
(device_class), `stat_cla` (state_class), `unit_of_meas`, `val_tpl` (value_template),
`ent_cat` (entity_category), `pl_on`/`pl_off`, `pl_avail`/`pl_not_avail`. Full keys are
equally valid; don't mix half-and-half within a payload confusingly.

## Pitfalls

- Forgetting `retain=true` → entities vanish after a broker restart.
- Publishing coordinates as `device_tracker` **state** → entity shows "unknown".
- Reusing a `unique_id` across entities → HA rejects/merges them.
- `total_increasing` on an odometer → false "reset" energy spikes.
- Not setting `availability_topic` on entities → they never show "unavailable".
- Changing a discovery topic's `<object_id>` orphans the old retained config (publish
  an empty payload to the old topic to clean it up).
