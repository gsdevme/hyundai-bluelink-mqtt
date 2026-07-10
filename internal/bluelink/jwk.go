package bluelink

import (
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// rsaJWK is the RSA public key returned by /auth/api/v1/accounts/certs under
// the "retValue" field. Only the modulus (n) and exponent (e) are needed to
// build a public key; kid is echoed back to signin.
type rsaJWK struct {
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// jwkToRSAPublicKey converts base64url-encoded JWK n/e components into an
// *rsa.PublicKey using math/big, without any external dependency.
func jwkToRSAPublicKey(jwk rsaJWK) (*rsa.PublicKey, error) {
	nBytes, err := base64URLDecode(jwk.N)
	if err != nil {
		return nil, fmt.Errorf("decode JWK modulus: %w", err)
	}
	eBytes, err := base64URLDecode(jwk.E)
	if err != nil {
		return nil, fmt.Errorf("decode JWK exponent: %w", err)
	}
	if len(nBytes) == 0 || len(eBytes) == 0 {
		return nil, errors.New("JWK missing modulus or exponent")
	}
	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)
	if !e.IsInt64() || e.Int64() > 1<<31-1 {
		return nil, errors.New("JWK exponent out of range")
	}
	return &rsa.PublicKey{N: n, E: int(e.Int64())}, nil
}

// base64URLDecode decodes base64url data whether or not it carries padding.
func base64URLDecode(s string) ([]byte, error) {
	s = strings.TrimRight(s, "=")
	return base64.RawURLEncoding.DecodeString(s)
}
