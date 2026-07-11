package bluelink

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/hex"
	"math/big"
	"testing"
)

// jwkFromKey encodes an RSA public key as a JWK the way the IDP does.
func jwkFromKey(t *testing.T, pub *rsa.PublicKey) rsaJWK {
	t.Helper()
	eBytes := big.NewInt(int64(pub.E)).Bytes()
	return rsaJWK{
		Kid: "test-kid",
		N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(eBytes),
	}
}

func TestJWKToRSAPublicKey(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	jwk := jwkFromKey(t, &key.PublicKey)

	got, err := jwkToRSAPublicKey(jwk)
	if err != nil {
		t.Fatalf("jwkToRSAPublicKey: %v", err)
	}
	if got.N.Cmp(key.N) != 0 {
		t.Fatal("modulus mismatch")
	}
	if got.E != key.E {
		t.Fatalf("exponent = %d, want %d", got.E, key.E)
	}
}

// TestPasswordEncryptRoundTrip proves the recovered public key can encrypt a
// password the matching private key decrypts — i.e. the full auth-crypto path.
func TestPasswordEncryptRoundTrip(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pub, err := jwkToRSAPublicKey(jwkFromKey(t, &key.PublicKey))
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	const password = "sup3r-s3cret!"
	encHex, err := encryptPassword(pub, password)
	if err != nil {
		t.Fatalf("encryptPassword: %v", err)
	}
	cipher, err := hex.DecodeString(encHex)
	if err != nil {
		t.Fatalf("result is not hex: %v", err)
	}
	//nolint:staticcheck // SA1019: mirrors the protocol-mandated PKCS#1 v1.5 encrypt path.
	plain, err := rsa.DecryptPKCS1v15(rand.Reader, key, cipher)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(plain) != password {
		t.Fatalf("round-trip = %q, want %q", plain, password)
	}
}

func TestBadJWK(t *testing.T) {
	if _, err := jwkToRSAPublicKey(rsaJWK{N: "!!!", E: "AQAB"}); err == nil {
		t.Fatal("expected error for invalid modulus")
	}
	if _, err := jwkToRSAPublicKey(rsaJWK{N: "", E: ""}); err == nil {
		t.Fatal("expected error for empty JWK")
	}
}
