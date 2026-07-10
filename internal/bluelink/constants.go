package bluelink

// Hyundai Bluelink EU brand constants, copied verbatim from the reference library
// Hyundai-Kia-Connect/hyundai_kia_connect_api (KiaUvoApiEU.py, Hyundai branch).
// See docs/specs/01-bluelink-api.md.
const (
	// loginHost is the IDPConnect OAuth2 host (login/authorize/certs/token).
	defaultLoginHost = "https://idpconnect-eu.hyundai.com"
	// spaBase is the vehicle/device (SPA) host, including the :8080 port.
	defaultSPABase = "https://prd.eu-ccapi.hyundai.com:8080"

	// spaAPIPath and userAPIPath are appended to the SPA base.
	spaAPIPath  = "/api/v1/spa/"
	userAPIPath = "/api/v1/user/"

	// ccspServiceID is the OAuth2 client_id and the ccsp-service-id header.
	ccspServiceID = "6d477c38-3ca4-4cf3-9557-2a1929a94654"
	// clientSecret is the OAuth2 client_secret.
	clientSecret = "KUy49XxPzLpLuoK0xhBC77W6VXhmtQR9iQhmIFjjoY4IpxsV"
	// appID is the ccsp-application-id header and the stamp APP_ID.
	appID = "014d2225-8495-4735-812d-2616334fd15d"
	// cfbBase64 decodes to the CFB key bytes used by the stamp XOR.
	cfbBase64 = "RFtoRq/vDXJmRndoZaZQyfOot7OrIqGVFj96iY2WL3yyH5Z/pUvlUhqmCxD2t+D65SQ="

	// pushType for device registration.
	pushType = "GCM"

	// userAgentOkHTTP is sent on all SPA/API calls.
	userAgentOkHTTP = "okhttp/3.12.0"
	// userAgentLoginForm is required (with the _CCS_APP_AOS suffix) on the login
	// form flow, or the authorize endpoint returns HTTP 400.
	userAgentLoginForm = "Mozilla/5.0 (Linux; Android 4.1.1; Galaxy Nexus Build/JRO03C) " +
		"AppleWebKit/535.19 (KHTML, like Gecko) Chrome/18.0.1025.166 Mobile Safari/535.19_CCS_APP_AOS"
)

// redirectURI is the hard-coded OAuth2 redirect_uri and must match exactly. It is
// derived from the SPA base + user API path so a base override (tests/mock) stays
// consistent, but in production it resolves to the constant below.
//
//	https://prd.eu-ccapi.hyundai.com:8080/api/v1/user/oauth2/token
func redirectURI(spaBase string) string {
	return spaBase + userAPIPath + "oauth2/token"
}
