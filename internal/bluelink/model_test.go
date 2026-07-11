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
		{"unknown source falls back to km label passthrough", f(14213), "", "mi", f(14213), "mi"},
		{"nil odometer unchanged", nil, "km", "mi", nil, "km"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := VehicleState{Odometer: tt.odo, OdometerUnit: tt.fromUnit}
			got := st.InDistanceUnit(tt.toUnit)

			switch {
			case tt.wantOdo == nil && got.Odometer != nil:
				t.Fatalf("Odometer = %v, want nil", *got.Odometer)
			case tt.wantOdo != nil && got.Odometer == nil:
				t.Fatalf("Odometer = nil, want %v", *tt.wantOdo)
			case tt.wantOdo != nil && math.Abs(*got.Odometer-*tt.wantOdo) > 1e-9:
				t.Errorf("Odometer = %v, want %v", *got.Odometer, *tt.wantOdo)
			}
			if got.OdometerUnit != tt.wantUnit {
				t.Errorf("OdometerUnit = %q, want %q", got.OdometerUnit, tt.wantUnit)
			}
		})
	}
}

// TestVehicleStateInDistanceUnitNoMutation guards against the value receiver
// mutating the caller's shared *float64 during conversion.
func TestVehicleStateInDistanceUnitNoMutation(t *testing.T) {
	orig := 14213.0
	st := VehicleState{Odometer: &orig, OdometerUnit: "km"}
	_ = st.InDistanceUnit("mi")
	if orig != 14213.0 {
		t.Errorf("source odometer mutated to %v, want 14213", orig)
	}
	if st.OdometerUnit != "km" {
		t.Errorf("source unit mutated to %q, want km", st.OdometerUnit)
	}
}
