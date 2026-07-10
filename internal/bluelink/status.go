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

// fetchStatusCCS1 is the CCS1 sibling of the CCS2 path in fetchStatus. It reads
// the cached or forced status document and maps it via parseCCS1. Unlike CCS2,
// the CCS1 response embeds a fresh vehicleLocation, so no separate location call
// is needed.
func (c *Client) fetchStatusCCS1(ctx context.Context, v Vehicle, force bool) (VehicleState, error) {
	path := "vehicles/" + v.ID + "/status/latest"
	if force {
		path = "vehicles/" + v.ID + "/status"
	}

	var out struct {
		ResMsg struct {
			VehicleStatusInfo map[string]any `json:"vehicleStatusInfo"`
		} `json:"resMsg"`
	}
	if err := c.authedGet(ctx, path, v.CCS2ProtocolSupport, &out); err != nil {
		return VehicleState{}, fmt.Errorf("fetch CCS1 status: %w", err)
	}
	if out.ResMsg.VehicleStatusInfo == nil {
		return VehicleState{}, errors.New("CCS1 status response missing resMsg.vehicleStatusInfo")
	}
	return parseCCS1(out.ResMsg.VehicleStatusInfo)
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
