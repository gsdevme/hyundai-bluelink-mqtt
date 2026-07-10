# 01 — Bluelink EU API

Verified against the Python reference lib. Two **distinct hosts** — do not conflate.

## Hosts & brand constants (Hyundai EU)

Copied verbatim into `internal/bluelink/constants.go`:

| Constant | Value |
| --- | --- |
| LOGIN host (IDP/OAuth2) | `https://idpconnect-eu.hyundai.com` |
| SPA host base | `https://prd.eu-ccapi.hyundai.com:8080` |
| SPA API prefix | `{SPA host}/api/v1/spa/` |
| USER API prefix | `{SPA host}/api/v1/user/` |
| `ccsp-service-id` (client_id) | `6d477c38-3ca4-4cf3-9557-2a1929a94654` |
| `client_secret` | `KUy49XxPzLpLuoK0xhBC77W6VXhmtQR9iQhmIFjjoY4IpxsV` |
| `ccsp-application-id` (APP_ID) | `014d2225-8495-4735-812d-2616334fd15d` |
| CFB (base64, decode to bytes) | `RFtoRq/vDXJmRndoZaZQyfOot7OrIqGVFj96iY2WL3yyH5Z/pUvlUhqmCxD2t+D65SQ=` |
| `redirect_uri` (exact, USER API + `oauth2/token`) | `https://prd.eu-ccapi.hyundai.com:8080/api/v1/user/oauth2/token` |
| Push type | `GCM` |

**User-Agent:** `okhttp/3.12.0` for all SPA/API calls; for the login **form** flow use
`Mozilla/5.0 (Linux; Android 4.1.1; Galaxy Nexus Build/JRO03C) AppleWebKit/535.19
(KHTML, like Gecko) Chrome/18.0.1025.166 Mobile Safari/535.19_CCS_APP_AOS` — the
`_CCS_APP_AOS` suffix is **required** or `authorize` returns 400.

## Stamp

Every SPA call carries a `Stamp` header:

```
stamp = base64( CFB_bytes XOR ("{APP_ID}:{unix_seconds}") )
```

`CFB_bytes` is the base64-decoded CFB constant. XOR is **byte-wise** and **truncates to
the shorter operand** (Go: iterate `min(len(cfb), len(raw))`). Replicate exactly.

## Headless OAuth2 login (all on LOGIN host)

The service authenticates with **username + plaintext password** (from config), no
browser. Cookies from the `authorize` GET must be carried through signin (use an
`http.CookieJar`).

1. `GET /auth/api/v2/user/oauth2/authorize?response_type=code&client_id={id}&redirect_uri={redirect}&lang=en&state=ccsp&country=de`
   — sets session cookies (follow redirects). Uses the mobile form UA.
2. `GET /auth/api/v1/accounts/certs` → JSON `retValue` = JWK `{n, e, kid}`
   (base64url, no padding). Build an `rsa.PublicKey` (`jwk.go`), then encrypt the
   UTF-8 password with **PKCS#1 v1.5** (`rsa.EncryptPKCS1v15`) and hex-encode it.
3. `POST /auth/account/signin` (form) with `encryptedPassword=true`, `password=<hex>`,
   `client_id`, `redirect_uri`, `state=ccsp`, `username`, `kid`, and empty
   `scope/nonce/connector_session_key/_csrf`. **Do not follow redirects.** Expect
   **302**; parse `code` from the `Location` query.
   - `Location` contains `/web/v1/user/authorization` → `ConsentRequiredError`
     (user must accept terms in a browser once).
   - `Location` contains `error`/`error_description` or `authorize` → auth failure.
4. `POST /auth/api/v2/user/oauth2/token` (form) `grant_type=authorization_code`,
   `code`, the hard-coded `redirect_uri`, `client_id`, `client_secret` → JSON
   `{token_type, access_token, refresh_token, expires_in}`. The stored access token is
   `"{token_type} {access_token}"` (e.g. `Bearer eyJ...`).

**Refresh:** `POST /auth/api/v2/user/oauth2/token` (form) `grant_type=refresh_token`,
`refresh_token`, `client_id`, `client_secret`. Response may include a new
`refresh_token` (persist it). On refresh failure, fall back to a full login.

Access-token lifetime: `valid_until = now + expires_in` (default 86400s). Refresh a
few minutes before expiry.

## Device registration (SPA host)

`POST notifications/register` with headers `ccsp-service-id`, `ccsp-application-id`,
`Stamp`, `Content-Type: application/json;charset=UTF-8`, `okhttp` UA. Body:
`{pushRegId: <64 hex>, pushType: "GCM", uuid: <uuid v4>}`.
Response → `resMsg.deviceId`. Store as `deviceId`; sent as `ccsp-device-id` on SPA calls.

## Authenticated SPA headers

Every SPA call carries: `Authorization: <access token>`, `ccsp-service-id`,
`ccsp-application-id`, `Stamp` (fresh per call), `ccsp-device-id`,
`Ccuccs2protocolsupport: <flag>`, `Host`, `okhttp` UA, `Accept-Encoding: gzip`.

## Endpoints used (read-only)

| Purpose | Method & path (relative to SPA prefix) | Notes |
| --- | --- | --- |
| Vehicle list | `GET vehicles` | `resMsg.vehicles[]`; capture `ccuCCS2ProtocolSupport`, `vin`, `vehicleId`, `nickname`, `vehicleName`, `type`, `regDate` |
| Cached status (CCS2, **no wake**) | `GET vehicles/{id}/ccs2/carstatus/latest` | parse `resMsg.state.Vehicle` |
| Force status (CCS2, **wakes car**) | `GET vehicles/{id}/ccs2/carstatus` | **GET**, not POST; parse `resMsg.state.Vehicle` |
| Cached park location | `GET vehicles/{id}/location/park` | `resMsg.coord.{lat,lon}`, `resMsg.time` |

Response envelope: `{ retCode, resCode, resMsg, msgId }`. Treat non-`S` `retCode` (or a
`resCode`/error body) as an API error. A `1003`/device-id error should trigger one
re-register + retry (reference lib's `_retry_on_device_id_error`); not required for the
first release but the client should surface the error clearly.

## Protocol selection (runtime)

Branch on `ccuCCS2ProtocolSupport` from the vehicle list:
- `!= 0` → **CCS2** (`parse_ccs2.go`, `fetchStatus`). Newer E-GMP vehicles report CCS2.
- `== 0` → **CCS1** (`parse_ccs1.go`, `fetchStatusCCS1`). Older vehicles (including some
  Insters) report CCS1. Both protocols map into the same `VehicleState` and run the full
  publish pipeline; the two parsers stay isolated as siblings.

### CCS1 endpoints

| Purpose | Endpoint | Envelope |
|---|---|---|
| Cached status (no wake) | `GET vehicles/{id}/status/latest` | `resMsg.vehicleStatusInfo.{vehicleStatus, vehicleLocation, odometer}` |
| Force status (wakes car) | `GET vehicles/{id}/status` | `resMsg` **is** the `vehicleStatus` (no wrapper, no location, no odometer) |
| Park location (no wake) | `GET vehicles/{id}/location/park` | `resMsg.gpsDetail.{coord,time}` |

The two status endpoints return **different shapes**, so `fetchStatusCCS1` handles each
separately and normalises both into the `vehicleStatusInfo` shape the parser expects:

- **Cached** embeds a fresh `vehicleLocation` and `odometer`, so no extra call is needed.
- **Force** carries neither, so location is resolved from the non-waking `/location/park`
  endpoint (its `gpsDetail` shares `vehicleLocation`'s `{coord,time}` shape) and odometer
  stays unknown until the next cached poll.

Note the CCS1 `/location/park` nests the fix under `gpsDetail`, unlike CCS2's `/location/park`
which puts `coord` directly in `resMsg`.
