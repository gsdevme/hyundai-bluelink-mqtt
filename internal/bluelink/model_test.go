package bluelink

import (
	"math"
	"testing"
)

func TestVehicleStateInDistanceUnit(t *testing.T) {
	f := func(v float64) *float64 { return &v }

	tests := []struct {
		name     string
		odo      *float64
		fromUnit string
		toUnit   string
		wantOdo  *float64 // nil means "expect Odometer stays nil"
		wantUnit string
	}{
		{"km to mi converts and rounds", f(14213), "km", "mi", f(8831.5), "mi"},
		{"km to mi fractional source", f(14213.5), "km", "mi", f(8831.9), "mi"},
		{"mi to km converts", f(8832), "mi", "km", f(14213.7), "km"},
		{"same unit unchanged", f(14213.5), "km", "km", f(14213.5), "km"},
		{"empty target unchanged", f(14213), "km", "", f(14213), "km"},
		{"unknown source keeps value and unit", f(14213), "", "mi", f(14213), ""},
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
	f := func(v float64) *float64 { return &v }

	st := VehicleState{
		Odometer: f(14213), OdometerUnit: "km",
		EVRange: f(259.99), EVRangeUnit: "km",
	}
	got := st.InDistanceUnit("mi")

	assertDistance(t, "Odometer", got.Odometer, got.OdometerUnit, f(8831.5), "mi")
	assertDistance(t, "EVRange", got.EVRange, got.EVRangeUnit, f(161.6), "mi")
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
