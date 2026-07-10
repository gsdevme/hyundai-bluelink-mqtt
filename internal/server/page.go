package server

import (
	"fmt"
	"html/template"
	"net/http"
	"runtime"
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

	PollInterval string
	ForceEnabled bool
	ForceTime    string
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
// exposes credentials; the VIN is masked to its last 4 characters.
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
		PollInterval: s.cfg.PollInterval.String(),
		ForceEnabled: s.cfg.ForceEnabled,
		ForceTime:    forceTime(s.cfg),
	}
	s.mu.Unlock()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = statusTmpl.Execute(w, view)
}

// maskVIN keeps only the last 4 characters of a VIN, replacing the rest with
// asterisks so the full identifier is never shown.
func maskVIN(vin string) string {
	const keep = 4
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
