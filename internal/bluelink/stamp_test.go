package bluelink

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestXorTruncate(t *testing.T) {
	tests := []struct {
		name string
		a, b []byte
		want []byte
	}{
		{"equal length", []byte{0x01, 0x02, 0x03}, []byte{0x10, 0x20, 0x30}, []byte{0x11, 0x22, 0x33}},
		{"a shorter truncates", []byte{0xFF, 0x00}, []byte{0x0F, 0xF0, 0xAA}, []byte{0xF0, 0xF0}},
		{"b shorter truncates", []byte{0x0F, 0xF0, 0xAA}, []byte{0xFF, 0x00}, []byte{0xF0, 0xF0}},
		{"empty", []byte{}, []byte{0x01}, []byte{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := xorTruncate(tc.a, tc.b)
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d", len(got), len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("byte %d = %#x, want %#x", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestStampRoundTrip verifies the stamp is valid base64 whose decoding, XORed
// again against the CFB bytes, recovers the "{APP_ID}:{unix}" plaintext — the
// XOR is involutive over the overlapping (truncated) region.
func TestStampRoundTrip(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	s, err := stamp(now)
	if err != nil {
		t.Fatalf("stamp: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("stamp is not valid base64: %v", err)
	}
	cfb, err := base64.StdEncoding.DecodeString(cfbBase64)
	if err != nil {
		t.Fatalf("decode CFB: %v", err)
	}
	raw := []byte(fmt.Sprintf("%s:%d", appID, now.Unix()))
	if len(decoded) != min(len(cfb), len(raw)) {
		t.Fatalf("decoded len = %d, want %d", len(decoded), min(len(cfb), len(raw)))
	}
	recovered := xorTruncate(cfb, decoded)
	want := raw[:len(recovered)]
	if string(recovered) != string(want) {
		t.Fatalf("recovered = %q, want %q", recovered, want)
	}
	if !strings.HasPrefix(string(recovered), appID+":") {
		t.Fatalf("recovered plaintext %q missing APP_ID prefix", recovered)
	}
}

func TestStampChangesWithTime(t *testing.T) {
	s1, _ := stamp(time.Unix(1000, 0))
	s2, _ := stamp(time.Unix(2000, 0))
	if s1 == s2 {
		t.Fatal("stamp should differ for different timestamps")
	}
}
