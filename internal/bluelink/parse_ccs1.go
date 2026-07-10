package bluelink

import "errors"

// ErrCCS1NotImplemented is returned when a vehicle reports the older CCS v1
// protocol. The Inster is CCS2, so CCS1 parsing is intentionally unimplemented;
// this is the documented extension point (see docs/specs/01-bluelink-api.md).
var ErrCCS1NotImplemented = errors.New("bluelink: CCS1 protocol not implemented")

// parseCCS1 is a stub. The runtime protocol branch in status.go calls into this
// path for CCS1 vehicles; implement the mapping here to add CCS1 support.
func parseCCS1(_ map[string]any) (VehicleState, error) {
	return VehicleState{}, ErrCCS1NotImplemented
}
