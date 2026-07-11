package bluelink

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Config configures a Client. Only Username and Password are required; the host
// overrides and Store/Logger/Now/HTTPClient have sensible defaults.
type Config struct {
	Username string
	Password string
	VIN      string // target vehicle; empty selects the first

	LoginHost string // override IDP host (tests/mock)
	SPABase   string // override SPA host (tests/mock)

	Store      TokenStore
	Logger     *slog.Logger
	Now        func() time.Time
	HTTPClient *http.Client
}

// Client is a read-only Bluelink EU API client. It is safe for concurrent use;
// token refresh and requests are serialised internally.
type Client struct {
	httpc     *http.Client
	loginHost string
	spaBase   string
	spaAPI    string
	username  string
	password  string
	vin       string
	store     TokenStore
	logger    *slog.Logger
	now       func() time.Time

	mu     sync.Mutex
	tokens Tokens
}

// New builds a Client from cfg, applying defaults and validating required fields.
func New(cfg Config) (*Client, error) {
	if cfg.Username == "" || cfg.Password == "" {
		return nil, errors.New("bluelink: username and password are required")
	}
	loginHost := cfg.LoginHost
	if loginHost == "" {
		loginHost = defaultLoginHost
	}
	spaBase := cfg.SPABase
	if spaBase == "" {
		spaBase = defaultSPABase
	}
	store := cfg.Store
	if store == nil {
		store = NewMemoryStore()
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	nowFn := cfg.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		httpc:     hc,
		loginHost: strings.TrimRight(loginHost, "/"),
		spaBase:   strings.TrimRight(spaBase, "/"),
		spaAPI:    strings.TrimRight(spaBase, "/") + spaAPIPath,
		username:  cfg.Username,
		password:  cfg.Password,
		vin:       cfg.VIN,
		store:     store,
		logger:    logger,
		now:       nowFn,
	}, nil
}

// Connect restores tokens from the store (or performs a full login + device
// registration) and ensures a usable access token.
func (c *Client) Connect(ctx context.Context) error {
	stored, ok, err := c.store.Load(ctx)
	if err != nil {
		return fmt.Errorf("load tokens: %w", err)
	}
	c.mu.Lock()
	if ok {
		c.tokens = stored
	}
	err = c.ensureTokenLocked(ctx)
	c.mu.Unlock()
	return err
}

// ensureTokenLocked guarantees a valid access token, refreshing or performing a
// full login as needed. The caller must hold c.mu.
func (c *Client) ensureTokenLocked(ctx context.Context) error {
	if c.tokens.Valid(c.now()) && c.tokens.DeviceID != "" {
		return nil
	}
	if c.tokens.RefreshToken != "" && c.tokens.DeviceID != "" {
		tok, err := c.refreshTokens(ctx, c.tokens.RefreshToken)
		if err == nil {
			tok.DeviceID = c.tokens.DeviceID
			c.tokens = tok
			return c.store.Save(ctx, tok)
		}
		c.logger.WarnContext(ctx, "token refresh failed, falling back to full login", "err", err)
	}
	return c.fullLoginLocked(ctx)
}

func (c *Client) fullLoginLocked(ctx context.Context) error {
	deviceID, err := c.registerDevice(ctx)
	if err != nil {
		return fmt.Errorf("register device: %w", err)
	}
	tok, err := c.loginWithPassword(ctx)
	if err != nil {
		return err
	}
	tok.DeviceID = deviceID
	c.tokens = tok
	if err := c.store.Save(ctx, tok); err != nil {
		return fmt.Errorf("save tokens: %w", err)
	}
	c.logger.InfoContext(ctx, "authenticated with bluelink", "device_id", deviceID)
	return nil
}

// registerDevice registers a push device and returns its deviceId. It needs only
// the stamp + service/application id headers (no access token).
func (c *Client) registerDevice(ctx context.Context) (string, error) {
	st, err := stamp(c.now())
	if err != nil {
		return "", err
	}
	payload := map[string]string{
		"pushRegId": randomHex(32),
		"pushType":  pushType,
		"uuid":      randomUUID(),
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.spaAPI+"notifications/register", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("ccsp-service-id", ccspServiceID)
	req.Header.Set("ccsp-application-id", appID)
	req.Header.Set("Stamp", st)
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	req.Header.Set("User-Agent", userAgentOkHTTP)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return "", err
	}
	defer drain(resp)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("notifications/register HTTP %d", resp.StatusCode)
	}
	var out struct {
		ResMsg struct {
			DeviceID string `json:"deviceId"`
		} `json:"resMsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode register response: %w", err)
	}
	if out.ResMsg.DeviceID == "" {
		return "", errors.New("register response missing deviceId")
	}
	return out.ResMsg.DeviceID, nil
}

// vehicleListEntry mirrors one element of resMsg.vehicles.
type vehicleListEntry struct {
	VehicleID          string `json:"vehicleId"`
	Nickname           string `json:"nickname"`
	VehicleName        string `json:"vehicleName"`
	RegDate            string `json:"regDate"`
	VIN                string `json:"vin"`
	Type               string `json:"type"`
	CCUCCS2ProtocolSup int    `json:"ccuCCS2ProtocolSupport"`
}

// Vehicles lists the vehicles on the account.
func (c *Client) Vehicles(ctx context.Context) ([]Vehicle, error) {
	var out struct {
		ResMsg struct {
			Vehicles []vehicleListEntry `json:"vehicles"`
		} `json:"resMsg"`
	}
	if err := c.authedGet(ctx, "vehicles", 0, &out); err != nil {
		return nil, fmt.Errorf("list vehicles: %w", err)
	}
	vehicles := make([]Vehicle, 0, len(out.ResMsg.Vehicles))
	for _, e := range out.ResMsg.Vehicles {
		vehicles = append(vehicles, Vehicle{
			ID:                  e.VehicleID,
			VIN:                 e.VIN,
			Name:                e.Nickname,
			Model:               e.VehicleName,
			RegDate:             e.RegDate,
			Type:                e.Type,
			CCS2ProtocolSupport: e.CCUCCS2ProtocolSup,
		})
	}
	return vehicles, nil
}

// SelectVehicle returns the target vehicle: the one matching the configured VIN,
// or the first vehicle if no VIN is configured.
func (c *Client) SelectVehicle(ctx context.Context) (Vehicle, error) {
	vehicles, err := c.Vehicles(ctx)
	if err != nil {
		return Vehicle{}, err
	}
	if len(vehicles) == 0 {
		return Vehicle{}, errors.New("bluelink: no vehicles on account")
	}
	if c.vin == "" {
		return vehicles[0], nil
	}
	for _, v := range vehicles {
		if strings.EqualFold(v.VIN, c.vin) {
			return v, nil
		}
	}
	return Vehicle{}, fmt.Errorf("bluelink: no vehicle with VIN %s", c.vin)
}

// DebugGet performs an authenticated SPA GET against relPath (relative to the API
// base) and returns the raw response body, reusing the normal auth/refresh, stamp
// and header machinery. It backs the hidden `dump` command used to capture raw
// API responses when adding protocol support; it is not used by the daemon.
func (c *Client) DebugGet(ctx context.Context, v Vehicle, relPath string) ([]byte, error) {
	return c.doAuthedGet(ctx, relPath, v.CCS2ProtocolSupport)
}

// authedGet performs an authenticated SPA GET, decoding resMsg-bearing JSON into
// out. It refreshes the token once on a 401 and retries.
func (c *Client) authedGet(ctx context.Context, path string, ccs2Support int, out any) error {
	body, err := c.doAuthedGet(ctx, path, ccs2Support)
	if err != nil {
		return err
	}
	if err := checkEnvelope(body); err != nil {
		return err
	}
	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("decode %s: %w", path, err)
		}
	}
	return nil
}

func (c *Client) doAuthedGet(ctx context.Context, path string, ccs2Support int) ([]byte, error) {
	c.mu.Lock()
	if err := c.ensureTokenLocked(ctx); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	tok := c.tokens
	c.mu.Unlock()

	body, status, err := c.rawGet(ctx, path, ccs2Support, tok)
	if err != nil {
		return nil, err
	}
	if status == http.StatusUnauthorized {
		// Force a refresh and retry once.
		c.mu.Lock()
		c.tokens.ValidUntil = time.Time{} // invalidate
		if err := c.ensureTokenLocked(ctx); err != nil {
			c.mu.Unlock()
			return nil, err
		}
		tok = c.tokens
		c.mu.Unlock()
		body, status, err = c.rawGet(ctx, path, ccs2Support, tok)
		if err != nil {
			return nil, err
		}
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", path, status)
	}
	return body, nil
}

func (c *Client) rawGet(ctx context.Context, path string, ccs2Support int, tok Tokens) ([]byte, int, error) {
	st, err := stamp(c.now())
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.spaAPI+path, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", tok.AccessToken)
	req.Header.Set("ccsp-service-id", ccspServiceID)
	req.Header.Set("ccsp-application-id", appID)
	req.Header.Set("Stamp", st)
	req.Header.Set("ccsp-device-id", tok.DeviceID)
	req.Header.Set("Ccuccs2protocolsupport", strconv.Itoa(ccs2Support))
	req.Header.Set("User-Agent", userAgentOkHTTP)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// checkEnvelope inspects the standard {retCode, resCode, resMsg} envelope and
// returns an error for non-success responses.
func checkEnvelope(body []byte) error {
	var env struct {
		RetCode string `json:"retCode"`
		ResCode string `json:"resCode"`
	}
	// resCode may be numeric or string; ignore decode errors on that field.
	_ = json.Unmarshal(body, &env)
	if env.RetCode != "" && env.RetCode != "S" {
		return fmt.Errorf("bluelink API error: retCode=%s resCode=%s", env.RetCode, env.ResCode)
	}
	return nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func randomUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
