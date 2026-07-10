// Package mock implements an in-process stand-in for the Hyundai Bluelink EU API.
// It serves the endpoints the client uses (authorize, certs, signin, token,
// device registration, vehicles, CCS2 + CCS1 status, location) with canned Inster
// data. The Options.CCS2 flag selects which protocol the vehicle reports. It is
// shared by the godog acceptance suite and the standalone `mock` subcommand, so
// the full pipeline can run without the real API.
package mock

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Options configure the canned vehicle state so scenarios can exercise different
// conditions (charging, unplugged, token expiry).
type Options struct {
	VIN             string
	VehicleID       string
	Model           string
	Nickname        string
	BatteryPercent  float64
	RangeKM         float64
	ChargeRemainMin int // 0 => not charging
	PluggedIn       bool
	CCS2            bool // protocol reported to the client; false serves CCS1
	Latitude        float64
	Longitude       float64
	AccessTokenTTL  time.Duration // lifetime advertised for issued access tokens
	Logger          *slog.Logger
}

// Defaults returns Options for a charging, plugged-in Inster.
func Defaults() Options {
	return Options{
		VIN:             "REDACTEDVIN000001",
		VehicleID:       "TEST-VEHICLE-ID-0001",
		Model:           "INSTER",
		Nickname:        "Inster",
		BatteryPercent:  62,
		RangeKM:         268,
		ChargeRemainMin: 145,
		PluggedIn:       true,
		CCS2:            true,
		Latitude:        51.507351,
		Longitude:       -0.127758,
		AccessTokenTTL:  24 * time.Hour,
		Logger:          slog.Default(),
	}
}

// Server is the mock API. Create with New and mount Handler().
type Server struct {
	mu   sync.Mutex
	opts Options
	key  *rsa.PrivateKey

	// Observability for tests.
	Grants       []string // grant_type values seen at the token endpoint
	RegisterHits int
	CachedHits   int
	ForceHits    int

	// failStatus, when set, makes the status endpoints return HTTP 500.
	failStatus bool
}

// SetFailStatus toggles whether status endpoints return HTTP 500 (for testing
// graceful degradation).
func (s *Server) SetFailStatus(fail bool) {
	s.mu.Lock()
	s.failStatus = fail
	s.mu.Unlock()
}

// New builds a mock server with a fresh RSA key for the certs/signin flow.
func New(opts Options) (*Server, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Server{opts: opts, key: key}, nil
}

// SetOptions replaces the canned state (used between scenarios).
func (s *Server) SetOptions(o Options) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if o.Logger == nil {
		o.Logger = s.opts.Logger
	}
	s.opts = o
}

// Counts returns a snapshot of the request counters.
func (s *Server) Counts() (cached, force, register int, grants []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.CachedHits, s.ForceHits, s.RegisterHits, append([]string(nil), s.Grants...)
}

// Handler returns the HTTP handler for the mock API. See routes.go for the
// route table.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	addRoutes(mux, s)
	return mux
}

func (s *Server) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "mock-session"})
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("<html><body>login</body></html>"))
}

func (s *Server) handleCerts(w http.ResponseWriter, _ *http.Request) {
	pub := s.key.PublicKey
	writeJSON(w, http.StatusOK, map[string]any{
		"retValue": map[string]any{
			"kid": "mock-kid",
			"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		},
	})
}

func (s *Server) handleSignin(w http.ResponseWriter, r *http.Request) {
	// Accept any encrypted password; issue an auth code via redirect.
	_ = r.ParseForm()
	redirect := r.FormValue("redirect_uri")
	loc := redirect + "?code=mock-auth-code&state=ccsp"
	w.Header().Set("Location", loc)
	w.WriteHeader(http.StatusFound)
}

func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	grant := r.FormValue("grant_type")
	s.mu.Lock()
	s.Grants = append(s.Grants, grant)
	ttl := s.opts.AccessTokenTTL
	s.mu.Unlock()
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token_type":    "Bearer",
		"access_token":  "mock-access-" + randToken(),
		"refresh_token": "mock-refresh-" + randToken(),
		"expires_in":    int(ttl.Seconds()),
	})
}

func (s *Server) handleRegister(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	s.RegisterHits++
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"retCode": "S",
		"resMsg":  map[string]any{"deviceId": "mock-device-" + randToken()},
	})
}

func (s *Server) handleVehicles(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	o := s.opts
	s.mu.Unlock()
	ccs2 := 0
	if o.CCS2 {
		ccs2 = 1
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"retCode": "S",
		"resMsg": map[string]any{
			"vehicles": []map[string]any{{
				"vehicleId":              o.VehicleID,
				"nickname":               o.Nickname,
				"vehicleName":            o.Model,
				"regDate":                "2026-01-15T00:00:00",
				"vin":                    o.VIN,
				"type":                   "EV",
				"ccuCCS2ProtocolSupport": ccs2,
			}},
		},
	})
}

func (s *Server) handleCached(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	s.CachedHits++
	o := s.opts
	fail := s.failStatus
	s.mu.Unlock()
	if fail {
		http.Error(w, "upstream error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, s.carStatus(o, false))
}

func (s *Server) handleForce(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	s.ForceHits++
	o := s.opts
	fail := s.failStatus
	s.mu.Unlock()
	if fail {
		http.Error(w, "upstream error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, s.carStatus(o, true))
}

func (s *Server) handleLocation(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	o := s.opts
	s.mu.Unlock()
	coord := map[string]any{"lat": o.Latitude, "lon": o.Longitude, "alt": 0}
	if !o.CCS2 {
		// CCS1 nests the fix under gpsDetail; CCS2 puts it directly in resMsg.
		writeJSON(w, http.StatusOK, map[string]any{
			"retCode": "S",
			"resMsg":  map[string]any{"gpsDetail": map[string]any{"coord": coord, "time": "20260710063000"}},
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"retCode": "S",
		"resMsg":  map[string]any{"coord": coord, "time": "20260710063000"},
	})
}

// handleCachedCCS1 serves the CCS1 cached status endpoint. It shares the hit
// counter and failure toggle with the CCS2 path so tests are protocol-agnostic.
func (s *Server) handleCachedCCS1(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	s.CachedHits++
	o := s.opts
	fail := s.failStatus
	s.mu.Unlock()
	if fail {
		http.Error(w, "upstream error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, s.carStatusCCS1(o, false))
}

func (s *Server) handleForceCCS1(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	s.ForceHits++
	o := s.opts
	fail := s.failStatus
	s.mu.Unlock()
	if fail {
		http.Error(w, "upstream error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, s.carStatusCCS1(o, true))
}

// carStatusCCS1 builds a CCS1 status envelope, matching the two distinct shapes a
// real Inster returns: the cached endpoint nests vehicleStatus under
// vehicleStatusInfo with an embedded vehicleLocation and odometer, while the force
// endpoint returns vehicleStatus directly under resMsg (no wrapper, no location,
// no odometer). A forced read reports a fresher battery % like the CCS2 path.
func (s *Server) carStatusCCS1(o Options, forced bool) map[string]any {
	battery := o.BatteryPercent
	statusTime := "20260710063000"
	if forced {
		battery += 1
		statusTime = "20260710050000"
	}
	plugin := 0
	if o.PluggedIn {
		plugin = 1
	}
	vehicleStatus := map[string]any{
		"doorLock": true,
		"evStatus": map[string]any{
			"batteryCharge": o.ChargeRemainMin > 0,
			"batteryStatus": battery,
			"batteryPlugin": plugin,
			"remainTime2": map[string]any{
				"atc":  map[string]any{"value": o.ChargeRemainMin, "unit": 1},
				"etc3": map[string]any{"value": 42, "unit": 1},
			},
			"drvDistance": []map[string]any{{
				"rangeByFuel": map[string]any{
					"totalAvailableRange": map[string]any{"value": o.RangeKM, "unit": 1},
				},
				"type": 2,
			}},
			"reservChargeInfos": map[string]any{
				"targetSOClist": []map[string]any{
					{"targetSOClevel": 100, "plugType": 0},
					{"targetSOClevel": 80, "plugType": 1},
				},
			},
		},
		"tirePressureLamp": map[string]any{"tirePressureLampAll": 0},
		"battery":          map[string]any{"batSoc": 87},
		"time":             statusTime,
	}
	if forced {
		// Force endpoint: resMsg is the vehicleStatus itself.
		return map[string]any{"retCode": "S", "resMsg": vehicleStatus}
	}
	return map[string]any{
		"retCode": "S",
		"resMsg": map[string]any{
			"vehicleStatusInfo": map[string]any{
				"vehicleLocation": map[string]any{
					"coord": map[string]any{"lat": o.Latitude, "lon": o.Longitude, "alt": 0},
					"time":  statusTime,
				},
				"vehicleStatus": vehicleStatus,
				"odometer":      map[string]any{"value": 4213.5, "unit": 1},
			},
		},
	}
}

// carStatus builds a CCS2 status envelope from the current options. A forced
// read reports a fresher battery % so tests can prove the force path was used.
func (s *Server) carStatus(o Options, forced bool) map[string]any {
	battery := o.BatteryPercent
	date := "2026-07-10T06:30:00Z"
	if forced {
		battery += 1
		date = "2026-07-10T05:00:00Z"
	}
	connector := 0
	if o.PluggedIn {
		connector = 1
	}
	vehicle := map[string]any{
		"Date": date,
		"Drivetrain": map[string]any{
			"Odometer":   4213.5,
			"FuelSystem": map[string]any{"DTE": map[string]any{"Total": o.RangeKM, "Unit": 1}},
		},
		"Electronics": map[string]any{
			"Battery": map[string]any{"Level": 87, "SensorReliability": 0},
		},
		"Green": map[string]any{
			"BatteryManagement": map[string]any{
				"BatteryRemain": map[string]any{"Ratio": battery},
				"SoH":           map[string]any{"Ratio": 99.0},
			},
			"ChargingInformation": map[string]any{
				"Charging":           map[string]any{"RemainTime": o.ChargeRemainMin},
				"EstimatedTime":      map[string]any{"Quick": 42, "Standard": 320, "ICCB": 600},
				"ConnectorFastening": map[string]any{"State": connector},
				"TargetSoC":          map[string]any{"Standard": 80, "Quick": 100},
			},
			"ChargingDoor": map[string]any{"State": connector},
			"Electric":     map[string]any{"SmartGrid": map[string]any{"RealTimePower": 7.2}},
		},
		"Cabin": map[string]any{
			"HVAC": map[string]any{
				"OutsideTemperature": map[string]any{"Value": 18.5, "Unit": 0},
				"Row1":               map[string]any{"Driver": map[string]any{"Temperature": map[string]any{"Value": 21.0, "Unit": 0}}},
			},
			"Door": map[string]any{
				"Row1": map[string]any{"Driver": map[string]any{"Lock": 0}, "Passenger": map[string]any{"Lock": 0}},
				"Row2": map[string]any{"Left": map[string]any{"Lock": 0}, "Right": map[string]any{"Lock": 0}},
			},
		},
		"Chassis": map[string]any{"Axle": map[string]any{"Tire": map[string]any{"PressureLow": 0}}},
	}
	return map[string]any{
		"retCode": "S",
		"resMsg":  map[string]any{"state": map[string]any{"Vehicle": vehicle}},
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func randToken() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return strings.TrimRight(base64.URLEncoding.EncodeToString(b), "=")
}
