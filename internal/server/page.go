package server

import (
	"fmt"
	"html/template"
	"net/http"
	"runtime"
	"strconv"
	"time"
)

// serviceName is shown at the top of the status page.
const serviceName = "hyundai-bluelink-mqtt"

// statusView is the data rendered into the / page. It is built under lock from a
// snapshot of the Server so the template never touches shared state directly.
type statusView struct {
	Service   string
	Ready     bool
	Uptime    string
	GoVersion string

	VehicleReady bool
	Model        string
	Name         string
	MaskedVIN    string
	CCS2         bool

	MetricsReady bool
	Metrics      []metricRow

	PollInterval string
	ForceEnabled bool
	ForceTime    string
}

// metricRow is a single label/value pair rendered under the Metrics heading. The
// value is pre-formatted here so the template stays dumb.
type metricRow struct {
	Label string
	Value string
}

// statusTmpl is parsed once at package load. The markup is deliberately minimal
// and isolated here so it is the single place to iterate on styling later
// (htmx, CSS, etc.).
var statusTmpl = template.Must(template.New("status").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Service}} status</title>
</head>
<body>
<h1>{{.Service}}</h1>
<p>Status: {{if .Ready}}ready{{else}}not ready{{end}}</p>
<p>Uptime: {{.Uptime}}</p>
<h2>Vehicle</h2>
{{if .VehicleReady}}
<ul>
<li>Model: {{.Model}}</li>
<li>Name: {{.Name}}</li>
<li>VIN: {{.MaskedVIN}}</li>
<li>CCS2: {{if .CCS2}}yes{{else}}no{{end}}</li>
</ul>
{{else}}
<p>initialising</p>
{{end}}
{{if .MetricsReady}}
<h2>Metrics</h2>
<ul>
{{range .Metrics}}<li>{{.Label}}: {{.Value}}</li>
{{end}}</ul>
{{end}}
<h2>Schedule</h2>
<ul>
<li>Poll interval: {{.PollInterval}}</li>
<li>Force refresh: {{if .ForceEnabled}}{{.ForceTime}}{{else}}disabled{{end}}</li>
</ul>
<p>Go: {{.GoVersion}}</p>
</body>
</html>
`))

// handleRoot renders the HTML status page. It always returns 200 and never
// exposes credentials; the VIN is masked to its last character.
func (s *Server) handleRoot(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	view := statusView{
		Service:      serviceName,
		Ready:        s.ready,
		Uptime:       time.Since(s.startedAt).Round(time.Second).String(),
		GoVersion:    runtime.Version(),
		VehicleReady: s.vehicleSet,
		Model:        s.vehicleModel,
		Name:         s.vehicleName,
		MaskedVIN:    maskVIN(s.vehicleVIN),
		CCS2:         s.vehicleCCS2,
		MetricsReady: s.metrics.Set,
		Metrics:      metricRows(s.metrics),
		PollInterval: s.cfg.PollInterval.String(),
		ForceEnabled: s.cfg.ForceEnabled,
		ForceTime:    forceTime(s.cfg),
	}
	s.mu.Unlock()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = statusTmpl.Execute(w, view)
}

// metricRows formats the non-personal metrics into display rows in a fixed
// order. Each absent (nil) value renders as "unknown". Values are plain strings
// escaped by html/template on render.
func metricRows(m Metrics) []metricRow {
	return []metricRow{
		{"Battery", pctVal(m.EVBatteryPercentage)},
		{"State of health", pctVal(m.EVBatterySoH)},
		{"Range", rangeVal(m.EVRange, m.EVRangeUnit)},
		{"Charging", boolVal(m.Charging)},
		{"Plugged in", boolVal(m.PluggedIn)},
		{"Charge port door", boolVal(m.ChargePortDoorOpen)},
		{"Charge limit (AC)", pctVal(m.ChargeLimitAC)},
		{"Charge limit (DC)", pctVal(m.ChargeLimitDC)},
		{"Charging power", floatUnitVal(m.ChargingPowerKW, "kW")},
		{"Est. charge time", minVal(m.EstChargeDurationMin)},
		{"Est. fast-charge time", minVal(m.EstFastChargeDurationMin)},
		{"12V battery", intUnitVal(m.Battery12VPercentage, "%")},
		{"Outside temperature", floatUnitVal(m.OutsideTemperatureC, "°C")},
		{"Inside temperature", floatUnitVal(m.InsideTemperatureC, "°C")},
		{"Tyre pressure warning", boolVal(m.TirePressureWarning)},
		{"Last updated", timeVal(m.LastUpdatedAt)},
	}
}

// unknown is the display string for any absent (nil) metric.
const unknown = "unknown"

func pctVal(v *float64) string { return floatUnitVal(v, "%") }

// rangeVal formats a range value, appending the unit only when the value is
// present so an absent range renders "unknown", never "unknown km".
func rangeVal(v *float64, unit string) string {
	if v == nil {
		return unknown
	}
	if unit == "" {
		return trimFloat(*v)
	}
	return trimFloat(*v) + " " + unit
}

func floatUnitVal(v *float64, unit string) string {
	if v == nil {
		return unknown
	}
	return trimFloat(*v) + " " + unit
}

func intUnitVal(v *int, unit string) string {
	if v == nil {
		return unknown
	}
	return fmt.Sprintf("%d %s", *v, unit)
}

// minVal formats a duration-in-minutes metric.
func minVal(v *int) string {
	if v == nil {
		return unknown
	}
	return fmt.Sprintf("%d min", *v)
}

func boolVal(v *bool) string {
	if v == nil {
		return unknown
	}
	if *v {
		return "yes"
	}
	return "no"
}

func timeVal(v *time.Time) string {
	if v == nil {
		return unknown
	}
	return v.Format(time.RFC3339)
}

// trimFloat formats a float without trailing zeros (e.g. 42.5, 100).
func trimFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// maskVIN keeps only the last character of a VIN, replacing the rest with
// asterisks so the full identifier is never shown.
func maskVIN(vin string) string {
	const keep = 1
	if len(vin) <= keep {
		return vin
	}
	masked := make([]byte, len(vin)-keep)
	for i := range masked {
		masked[i] = '*'
	}
	return string(masked) + vin[len(vin)-keep:]
}

// forceTime formats the daily force-refresh time and location for display.
func forceTime(cfg Config) string {
	loc := "UTC"
	if cfg.ForceLocation != nil {
		loc = cfg.ForceLocation.String()
	}
	return fmt.Sprintf("%02d:%02d %s", cfg.ForceHour, cfg.ForceMinute, loc)
}
