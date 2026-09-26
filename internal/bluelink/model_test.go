package bluelink

import (
	"math"
	"testing"
)

func TestVehicleStateInDistanceUnit(t *testing.T) {
	tests := []struct {
		name     string
		odo      *float64
		fromUnit string
		toUnit   string
		wantOdo  *float64 // nil means "expect Odometer stays nil"
		wantUnit string
	}{
		{"km to mi converts and rounds", new(14213.0), "km", "mi", new(8831.5), "mi"},
		{"km to mi fractional source", new(14213.5), "km", "mi", new(8831.9), "mi"},
		{"mi to km converts", new(8832.0), "mi", "km", new(14213.7), "km"},
		{"same unit unchanged", new(14213.5), "km", "km", new(14213.5), "km"},
		{"empty target unchanged", new(14213.0), "km", "", new(14213.0), "km"},
		{"unknown source keeps value and unit", new(14213.0), "", "mi", new(14213.0), ""},
		{"nil value unchanged", nil, "km", "mi", nil, "km"},
	}

	// The odometer and the range go through the same conversion, so drive both
	// fields off one table.
	for _, tt := range tests {
		t.Run("odometer/"+tt.name, func(t *testing.T) {
			st := VehicleState{Odometer: tt.odo, OdometerUnit: tt.fromUnit}
			got := st.InDistanceUnit(tt.toUnit)
			assertDistance(t, "Odometer", got.Odometer, got.OdometerUnit, tt.wantOdo, tt.wantUnit)
		})

		t.Run("range/"+tt.name, func(t *testing.T) {
			st := VehicleState{EVRange: tt.odo, EVRangeUnit: tt.fromUnit}
			got := st.InDistanceUnit(tt.toUnit)
			assertDistance(t, "EVRange", got.EVRange, got.EVRangeUnit, tt.wantOdo, tt.wantUnit)
		})
	}
}

// assertDistance checks an optional distance and its unit against what the table
// expects; a nil want means "expect the value to stay nil".
func assertDistance(t *testing.T, name string, got *float64, gotUnit string, want *float64, wantUnit string) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Fatalf("%s = %v, want nil", name, *got)
	case want != nil && got == nil:
		t.Fatalf("%s = nil, want %v", name, *want)
	case want != nil && math.Abs(*got-*want) > 1e-9:
		t.Errorf("%s = %v, want %v", name, *got, *want)
	}
	if gotUnit != wantUnit {
		t.Errorf("%s unit = %q, want %q", name, gotUnit, wantUnit)
	}
}

// TestVehicleStateInDistanceUnitBothFields checks a realistic state where the two
// distances arrive in different units — the CCS2 case, where the odometer is always
// km but the range follows the driver's display setting.
func TestVehicleStateInDistanceUnitBothFields(t *testing.T) {
	st := VehicleState{
		Odometer: new(14213.0), OdometerUnit: "km",
		EVRange: new(259.99), EVRangeUnit: "km",
	}
	got := st.InDistanceUnit("mi")

	assertDistance(t, "Odometer", got.Odometer, got.OdometerUnit, new(8831.5), "mi")
	assertDistance(t, "EVRange", got.EVRange, got.EVRangeUnit, new(161.6), "mi")
}

// TestVehicleStateInDistanceUnitNoMutation guards against the value receiver
// mutating the caller's shared *float64 during conversion.
func TestVehicleStateInDistanceUnitNoMutation(t *testing.T) {
	odo, rng := 14213.0, 259.99
	st := VehicleState{Odometer: &odo, OdometerUnit: "km", EVRange: &rng, EVRangeUnit: "km"}
	_ = st.InDistanceUnit("mi")

	if odo != 14213.0 {
		t.Errorf("source odometer mutated to %v, want 14213", odo)
	}
	if rng != 259.99 {
		t.Errorf("source range mutated to %v, want 259.99", rng)
	}
	if st.OdometerUnit != "km" {
		t.Errorf("source odometer unit mutated to %q, want km", st.OdometerUnit)
	}
	if st.EVRangeUnit != "km" {
		t.Errorf("source range unit mutated to %q, want km", st.EVRangeUnit)
	}
}
