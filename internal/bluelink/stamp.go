package bluelink

import (
	"encoding/base64"
	"fmt"
	"time"
)

// stamp builds the per-request Stamp header:
//
//	base64( CFB_bytes XOR "{APP_ID}:{unix_seconds}" )
//
// The XOR is byte-wise and truncates to the shorter operand (see xorTruncate),
// replicating the reference library exactly.
func stamp(now time.Time) (string, error) {
	cfb, err := base64.StdEncoding.DecodeString(cfbBase64)
	if err != nil {
		return "", fmt.Errorf("decode CFB constant: %w", err)
	}
	raw := []byte(fmt.Sprintf("%s:%d", appID, now.Unix()))
	return base64.StdEncoding.EncodeToString(xorTruncate(cfb, raw)), nil
}

// xorTruncate XORs a and b byte-wise, producing a result the length of the
// shorter input (bytes past that point are dropped, matching Python's zip()).
func xorTruncate(a, b []byte) []byte {
	n := min(len(a), len(b))
	out := make([]byte, n)
	for i := range n {
		out[i] = a[i] ^ b[i]
	}
	return out
}
