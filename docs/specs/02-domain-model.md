# 02 — Domain model

`internal/bluelink` exposes two clean internal types the rest of the app consumes:
`Vehicle` (identity + protocol flag) and `VehicleState` (the metrics snapshot).
Units are **normalised on ingest** so the publisher never converts.

## `Vehicle`

| Field | Source (`resMsg.vehicles[]`) |
| --- | --- |
| `ID` | `vehicleId` |
| `VIN` | `vin` |
| `Name` | `nickname` |
| `Model` | `vehicleName` (e.g. "INSTER") |
| `RegDate` | `regDate` |
| `Type` | `type` (`EV` for the Inster) |
| `CCS2ProtocolSupport` | `ccuCCS2ProtocolSupport` (int; `!=0` ⇒ CCS2) |

`Vehicle.IsCCS2()` returns `CCS2ProtocolSupport != 0`.

## `VehicleState`

The snapshot published to MQTT. Pointer/`*T` fields (or an `Optional[T]`) distinguish
"unknown/absent" (→ HA `None`) from a real zero. Times are `time.Time` (UTC).

| Field | Type | CCS2 path (under `resMsg.state.Vehicle`) | Unit / notes |
| --- | --- | --- | --- |
| `EVBatteryPercentage` | `*float64` | `Green.BatteryManagement.BatteryRemain.Ratio` | % |
| `EVBatterySoH` | `*float64` | `Green.BatteryManagement.SoH.Ratio` | % |
| `EVRange` | `*float64` | `Drivetrain.FuelSystem.DTE.Total` | value |
| `EVRangeUnit` | `string` | `Drivetrain.FuelSystem.DTE.Unit` (index→unit) | `km`/`mi` |
| `Charging` | `*bool` | derived: `Green.ChargingInformation.Charging.RemainTime > 0` | see note |
| `PluggedIn` | `*bool` | `Green.ChargingInformation.ConnectorFastening.State` (`!=0`) | |
| `ChargePortDoorOpen` | `*bool` | `Green.ChargingDoor.State` (`1`→open, `0/2`→closed) | |
| `ChargeLimitAC` | `*float64` | `Green.ChargingInformation.TargetSoC.Standard` | % |
| `ChargeLimitDC` | `*float64` | `Green.ChargingInformation.TargetSoC.Quick` | % |
| `ChargingPowerKW` | `*float64` | `Green.Electric.SmartGrid.RealTimePower` | kW |
| `EstChargeDurationMin` | `*int` | `Green.ChargingInformation.Charging.RemainTime` | minutes |
| `EstFastChargeDurationMin` | `*int` | `Green.ChargingInformation.EstimatedTime.Quick` | minutes |
| `Battery12VPercentage` | `*int` | `Electronics.Battery.Level` (via `normalizeBatterySoC`) | % |
| `Odometer` | `*float64` | `Drivetrain.Odometer` | value |
| `OdometerUnit` | `string` | fixed `km` (`DISTANCE_UNITS[1]`) | |
| `OutsideTemperatureC` | `*float64` | `Cabin.HVAC.OutsideTemperature.Value` (+`.Unit`→°C) | °C |
| `InsideTemperatureC` | `*float64` | `Cabin.HVAC.Row1.Driver.Temperature.Value` (+`.Unit`) | °C; skip if `"OFF"` |
| `Locked` | `*bool` | all `Cabin.Door.*.Lock` truthy ⇒ locked | AND of the four doors |
| `TirePressureWarning` | `*bool` | `Chassis.Axle.Tire.PressureLow` OR any per-axle `PressureLow` | |
| `Latitude` | `*float64` | `location/park` `resMsg.coord.lat` (fallback `Location.GeoCoord.Latitude`) | |
| `Longitude` | `*float64` | `location/park` `resMsg.coord.lon` (fallback `Location.GeoCoord.Longitude`) | |
| `LocationUpdatedAt` | `*time.Time` | `location/park` `resMsg.time` | |
| `LastUpdatedAt` | `*time.Time` | `Date` (UTC) | ISO-8601 in JSON |

### Notes / normalisation rules

- **Charging bool.** The reference lib derives `ev_battery_is_charging` from the
  estimated current-charge duration (`Charging.RemainTime`): `0`→false, `>0`→true.
  We replicate that. (There is no single reliable boolean field in CCS2.)
- **Plugged-in.** Reference sets it from `ConnectorFastening.State`
  (the connector latched flag). `!=0` ⇒ plugged in.
- **Charge-port door.** `Green.ChargingDoor.State`: `1`→open; `0` or `2`→closed;
  anything else→unknown.
- **Distance unit index** (`DISTANCE_UNITS`): `1`→`km`, `2`/`3`→`mi`, `0`/absent→
  unknown. Odometer is always km (`DISTANCE_UNITS[1]`).
- **Temperature unit index** (`TEMPERATURE_UNITS`): `0`→°C, `1`→°F. Values that are
  already °C pass through; °F is converted to °C on ingest so the model is always °C.
  Inside temp value `"OFF"` (HVAC off) ⇒ leave `nil`.
- **12V SoC** (`normalizeBatterySoC`): return `nil` if `SensorReliability == 1`, if the
  value is missing, or if outside `0..100` (e.g. `255`/`0xFF` sentinel).
- **`get_child_value` semantics.** Dotted paths walk nested maps; a missing segment
  yields `nil` (→ pointer stays `nil`). Implement a stdlib equivalent in
  `parse_ccs2.go` that never panics on absent keys.
- The CCS2 cached-status response embeds a **stale** location; always override it with
  the `/location/park` result (a non-wake endpoint), matching the reference lib.

## CCS1

`parse_ccs1.go` is a stub (`ErrCCS1NotImplemented`). The Inster is CCS2, so CCS1 is not
implemented; the branch and error exist as the documented extension point.
