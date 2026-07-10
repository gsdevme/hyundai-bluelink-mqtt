package bluelink

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// CachedStatus fetches the latest cached CCS2 status plus park location. It never
// wakes the car.
func (c *Client) CachedStatus(ctx context.Context, v Vehicle) (VehicleState, error) {
	return c.fetchStatus(ctx, v, false)
}

// ForceStatus fetches a forced CCS2 status plus park location. This wakes the car
// and should be used sparingly (the scheduled daily refresh).
func (c *Client) ForceStatus(ctx context.Context, v Vehicle) (VehicleState, error) {
	return c.fetchStatus(ctx, v, true)
}

func (c *Client) fetchStatus(ctx context.Context, v Vehicle, force bool) (VehicleState, error) {
	if !v.IsCCS2() {
		// Decision point for future CCS1 support (see parse_ccs1.go).
		c.logger.WarnContext(ctx, "vehicle uses CCS1 protocol; status parsing not implemented, degrading",
			"vehicle_id", v.ID, "ccs2_support", v.CCS2ProtocolSupport)
		return VehicleState{}, ErrCCS1NotImplemented
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
