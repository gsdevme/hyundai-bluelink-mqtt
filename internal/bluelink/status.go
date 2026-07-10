package bluelink

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// CachedStatus fetches the latest cached status (CCS2 or CCS1, per the vehicle's
// protocol) plus location. It never wakes the car.
func (c *Client) CachedStatus(ctx context.Context, v Vehicle) (VehicleState, error) {
	return c.fetchStatus(ctx, v, false)
}

// ForceStatus fetches a forced status (CCS2 or CCS1, per the vehicle's protocol)
// plus location. This wakes the car and should be used sparingly (the scheduled
// daily refresh).
func (c *Client) ForceStatus(ctx context.Context, v Vehicle) (VehicleState, error) {
	return c.fetchStatus(ctx, v, true)
}

func (c *Client) fetchStatus(ctx context.Context, v Vehicle, force bool) (VehicleState, error) {
	if !v.IsCCS2() {
		return c.fetchStatusCCS1(ctx, v, force)
	}

	path := "vehicles/" + v.ID + "/ccs2/carstatus/latest"
	if force {
		path = "vehicles/" + v.ID + "/ccs2/carstatus"
	}

	var out struct {
		ResMsg struct {
			State struct {
				Vehicle map[string]any `json:"Vehicle"`
			} `json:"state"`
		} `json:"resMsg"`
	}
	if err := c.authedGet(ctx, path, v.CCS2ProtocolSupport, &out); err != nil {
		return VehicleState{}, fmt.Errorf("fetch status: %w", err)
	}
	if out.ResMsg.State.Vehicle == nil {
		return VehicleState{}, errors.New("status response missing resMsg.state.Vehicle")
	}

	state := parseCCS2(out.ResMsg.State.Vehicle)

	// The status response embeds a stale location; override it with the
	// non-waking park-location endpoint (matches the reference library).
	if lat, lon, ts, err := c.parkLocation(ctx, v); err != nil {
		c.logger.DebugContext(ctx, "park location unavailable", "err", err)
	} else {
		state.Latitude, state.Longitude, state.LocationUpdatedAt = lat, lon, ts
	}
	return state, nil
}

// fetchStatusCCS1 is the CCS1 sibling of the CCS2 path in fetchStatus. The two
// CCS1 endpoints return different envelopes, so each is handled separately and
// normalised into the vehicleStatusInfo shape parseCCS1 expects.
func (c *Client) fetchStatusCCS1(ctx context.Context, v Vehicle, force bool) (VehicleState, error) {
	if force {
		return c.forceStatusCCS1(ctx, v)
	}
	return c.cachedStatusCCS1(ctx, v)
}

// cachedStatusCCS1 reads status/latest, whose resMsg.vehicleStatusInfo already
// nests vehicleStatus plus an embedded (fresh) vehicleLocation and odometer.
func (c *Client) cachedStatusCCS1(ctx context.Context, v Vehicle) (VehicleState, error) {
	var out struct {
		ResMsg struct {
			VehicleStatusInfo map[string]any `json:"vehicleStatusInfo"`
		} `json:"resMsg"`
	}
	if err := c.authedGet(ctx, "vehicles/"+v.ID+"/status/latest", v.CCS2ProtocolSupport, &out); err != nil {
		return VehicleState{}, fmt.Errorf("fetch CCS1 status: %w", err)
	}
	if out.ResMsg.VehicleStatusInfo == nil {
		return VehicleState{}, errors.New("CCS1 status response missing resMsg.vehicleStatusInfo")
	}
	return parseCCS1(out.ResMsg.VehicleStatusInfo)
}

// forceStatusCCS1 reads the force status endpoint, whose resMsg *is* the
// vehicleStatus (no vehicleStatusInfo wrapper, no embedded location, no
// odometer). It wraps that into the shape parseCCS1 expects and resolves location
// from the non-waking park endpoint. Odometer is not returned by this endpoint,
// so it stays nil until the next cached poll.
func (c *Client) forceStatusCCS1(ctx context.Context, v Vehicle) (VehicleState, error) {
	var out struct {
		ResMsg map[string]any `json:"resMsg"`
	}
	if err := c.authedGet(ctx, "vehicles/"+v.ID+"/status", v.CCS2ProtocolSupport, &out); err != nil {
		return VehicleState{}, fmt.Errorf("fetch CCS1 status: %w", err)
	}
	if out.ResMsg == nil {
		return VehicleState{}, errors.New("CCS1 force status response missing resMsg")
	}
	info := map[string]any{"vehicleStatus": out.ResMsg}
	// gpsDetail and vehicleLocation share the same {coord, time} shape, so the
	// park-location payload can be injected directly.
	if loc, err := c.parkLocationCCS1(ctx, v); err != nil {
		c.logger.DebugContext(ctx, "CCS1 park location unavailable", "err", err)
	} else if loc != nil {
		info["vehicleLocation"] = loc
	}
	return parseCCS1(info)
}

// parkLocationCCS1 fetches the cached CCS1 park location (does not wake the car).
// Unlike CCS2's /location/park, the CCS1 response nests the fix under gpsDetail.
func (c *Client) parkLocationCCS1(ctx context.Context, v Vehicle) (map[string]any, error) {
	var out struct {
		ResMsg struct {
			GpsDetail map[string]any `json:"gpsDetail"`
		} `json:"resMsg"`
	}
	if err := c.authedGet(ctx, "vehicles/"+v.ID+"/location/park", v.CCS2ProtocolSupport, &out); err != nil {
		return nil, err
	}
	return out.ResMsg.GpsDetail, nil
}

// parkLocation fetches the cached park location (does not wake the car).
func (c *Client) parkLocation(ctx context.Context, v Vehicle) (lat, lon *float64, ts *time.Time, err error) {
	var out struct {
		ResMsg struct {
			Coord struct {
				Lat *float64 `json:"lat"`
				Lon *float64 `json:"lon"`
			} `json:"coord"`
			Time string `json:"time"`
		} `json:"resMsg"`
	}
	if err := c.authedGet(ctx, "vehicles/"+v.ID+"/location/park", v.CCS2ProtocolSupport, &out); err != nil {
		return nil, nil, nil, err
	}
	lat, lon = out.ResMsg.Coord.Lat, out.ResMsg.Coord.Lon
	if t, ok := parseBluelinkTime(out.ResMsg.Time); ok {
		ts = &t
	}
	return lat, lon, ts, nil
}

// parseBluelinkTime parses the compact "YYYYMMDDHHMMSS" timestamp used by the
// location endpoint (treated as UTC). Returns ok=false for empty/unparseable.
func parseBluelinkTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{"20060102150405", time.RFC3339} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
