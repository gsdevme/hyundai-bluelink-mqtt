package bluelink

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// Errors surfaced by the auth flow.
var (
	// ErrConsentRequired means the account must accept updated terms in a
	// browser once before headless login can succeed.
	ErrConsentRequired = errors.New("bluelink: account consent required (log in via browser once)")
	// ErrAuthFailed means credentials were rejected or the flow broke.
	ErrAuthFailed = errors.New("bluelink: authentication failed")
)

// encryptPassword RSA-encrypts the password with PKCS#1 v1.5 and hex-encodes it,
// matching the IDP's expected `encryptedPassword=true` format.
func encryptPassword(pub *rsa.PublicKey, password string) (string, error) {
	ct, err := rsa.EncryptPKCS1v15(rand.Reader, pub, []byte(password))
	if err != nil {
		return "", fmt.Errorf("rsa encrypt password: %w", err)
	}
	return hex.EncodeToString(ct), nil
}

// tokenResponse is the OAuth2 token endpoint payload.
type tokenResponse struct {
	TokenType    string `json:"token_type"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// loginWithPassword runs the full headless OAuth2 flow and returns fresh tokens
// (access/refresh + expiry). The device ID is registered separately.
func (c *Client) loginWithPassword(ctx context.Context) (Tokens, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return Tokens{}, fmt.Errorf("cookie jar: %w", err)
	}
	// A dedicated client carrying the login cookies; do not follow the signin
	// 302 so we can read the authorization code from the Location header.
	lc := &http.Client{Jar: jar, Timeout: c.httpc.Timeout}

	redirect := redirectURI(c.spaBase)

	// Step 1: authorize (sets session cookies).
	authURL := c.loginHost + "/auth/api/v2/user/oauth2/authorize" +
		"?response_type=code&client_id=" + ccspServiceID +
		"&redirect_uri=" + url.QueryEscape(redirect) +
		"&lang=en&state=ccsp&country=de"
	if err := c.loginGet(ctx, lc, authURL); err != nil {
		return Tokens{}, fmt.Errorf("authorize: %w", err)
	}

	// Step 2: fetch the RSA public key.
	jwk, err := c.fetchCerts(ctx, lc)
	if err != nil {
		return Tokens{}, err
	}
	pub, err := jwkToRSAPublicKey(jwk)
	if err != nil {
		return Tokens{}, err
	}

	// Step 3: encrypt password and sign in.
	encPW, err := encryptPassword(pub, c.password)
	if err != nil {
		return Tokens{}, err
	}
	code, err := c.signin(ctx, lc, redirect, jwk.Kid, encPW)
	if err != nil {
		return Tokens{}, err
	}

	// Step 4: exchange the code for tokens.
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirect},
		"client_id":     {ccspServiceID},
		"client_secret": {clientSecret},
	}
	tr, err := c.postToken(ctx, lc, form)
	if err != nil {
		return Tokens{}, fmt.Errorf("token exchange: %w", err)
	}
	return c.tokensFrom(tr), nil
}

// refreshTokens exchanges the refresh token for a new access token, rotating the
// refresh token if the server issues a new one.
func (c *Client) refreshTokens(ctx context.Context, refresh string) (Tokens, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refresh},
		"client_id":     {ccspServiceID},
		"client_secret": {clientSecret},
	}
	tr, err := c.postToken(ctx, c.httpc, form)
	if err != nil {
		return Tokens{}, fmt.Errorf("refresh: %w", err)
	}
	tok := c.tokensFrom(tr)
	if tok.RefreshToken == "" {
		tok.RefreshToken = refresh // server did not rotate it
	}
	return tok, nil
}

func (c *Client) tokensFrom(tr tokenResponse) Tokens {
	expires := tr.ExpiresIn
	if expires == 0 {
		expires = 86400
	}
	return Tokens{
		AccessToken:  strings.TrimSpace(tr.TokenType + " " + tr.AccessToken),
		RefreshToken: tr.RefreshToken,
		ValidUntil:   c.now().Add(time.Duration(expires) * time.Second),
	}
}

func (c *Client) loginGet(ctx context.Context, lc *http.Client, u string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgentLoginForm)
	resp, err := lc.Do(req)
	if err != nil {
		return err
	}
	defer drain(resp)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("%w: authorize returned HTTP %d", ErrAuthFailed, resp.StatusCode)
	}
	return nil
}

func (c *Client) fetchCerts(ctx context.Context, lc *http.Client) (rsaJWK, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.loginHost+"/auth/api/v1/accounts/certs", nil)
	if err != nil {
		return rsaJWK{}, err
	}
	req.Header.Set("User-Agent", userAgentLoginForm)
	resp, err := lc.Do(req)
	if err != nil {
		return rsaJWK{}, err
	}
	defer drain(resp)
	if resp.StatusCode != http.StatusOK {
		return rsaJWK{}, fmt.Errorf("%w: certs returned HTTP %d", ErrAuthFailed, resp.StatusCode)
	}
	var body struct {
		RetValue rsaJWK `json:"retValue"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return rsaJWK{}, fmt.Errorf("decode certs: %w", err)
	}
	return body.RetValue, nil
}

func (c *Client) signin(ctx context.Context, lc *http.Client, redirect, kid, encPW string) (string, error) {
	form := url.Values{
		"client_id":             {ccspServiceID},
		"encryptedPassword":     {"true"},
		"password":              {encPW},
		"redirect_uri":          {redirect},
		"scope":                 {""},
		"nonce":                 {""},
		"state":                 {"ccsp"},
		"username":              {c.username},
		"connector_session_key": {""},
		"kid":                   {kid},
		"_csrf":                 {""},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.loginHost+"/auth/account/signin", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgentLoginForm)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Do not follow the redirect: we need the Location header.
	lc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	defer func() { lc.CheckRedirect = nil }()

	resp, err := lc.Do(req)
	if err != nil {
		return "", err
	}
	defer drain(resp)
	if resp.StatusCode != http.StatusFound {
		return "", fmt.Errorf("%w: signin returned HTTP %d (check username/password)", ErrAuthFailed, resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	return codeFromLocation(loc)
}

// codeFromLocation extracts the ?code=... value from a signin redirect Location,
// mapping known non-code redirects to specific errors.
func codeFromLocation(loc string) (string, error) {
	u, err := url.Parse(loc)
	if err != nil {
		return "", fmt.Errorf("%w: unparseable redirect %q", ErrAuthFailed, loc)
	}
	if code := u.Query().Get("code"); code != "" {
		return code, nil
	}
	lower := strings.ToLower(loc)
	switch {
	case strings.Contains(loc, "/web/v1/user/authorization"):
		return "", ErrConsentRequired
	case u.Query().Get("error") != "" || strings.Contains(lower, "error"):
		desc := u.Query().Get("error_description")
		return "", fmt.Errorf("%w: %s", ErrAuthFailed, desc)
	case strings.Contains(lower, "authorize"):
		return "", fmt.Errorf("%w: returned to login page", ErrAuthFailed)
	default:
		return "", fmt.Errorf("%w: unexpected redirect", ErrAuthFailed)
	}
}

func (c *Client) postToken(ctx context.Context, hc *http.Client, form url.Values) (tokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.loginHost+"/auth/api/v2/user/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("User-Agent", userAgentOkHTTP)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := hc.Do(req)
	if err != nil {
		return tokenResponse{}, err
	}
	defer drain(resp)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return tokenResponse{}, fmt.Errorf("token endpoint HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return tokenResponse{}, fmt.Errorf("decode token response: %w", err)
	}
	if tr.AccessToken == "" {
		return tokenResponse{}, fmt.Errorf("token response missing access_token")
	}
	return tr, nil
}

// drain reads and closes a response body so the connection can be reused.
func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
}
