package client

import (
	"crypto/tls"
	"testing"
)

func TestRandomizedTLSConfigInvariants(t *testing.T) {
	for i := 0; i < 25; i++ {
		c := randomizedTLSConfig()
		if c.MinVersion != tls.VersionTLS12 {
			t.Fatalf("MinVersion = %v, want TLS 1.2 floor", c.MinVersion)
		}
		if c.InsecureSkipVerify {
			t.Fatal("InsecureSkipVerify must never be true")
		}
		if len(c.NextProtos) != 2 || c.NextProtos[0] != "h2" || c.NextProtos[1] != "http/1.1" {
			t.Fatalf("ALPN order must stay [h2 http/1.1], got %v", c.NextProtos)
		}
		if len(c.CipherSuites) == 0 {
			t.Fatal("CipherSuites must not be empty")
		}
		// Every shuffled suite must come from Go's secure default set —
		// shuffling must never smuggle in a weak/unknown suite.
		allowed := make(map[uint16]bool, len(tls.CipherSuites()))
		for _, d := range tls.CipherSuites() {
			allowed[d.ID] = true
		}
		for _, id := range c.CipherSuites {
			if !allowed[id] {
				t.Fatalf("cipher suite %#04x not in Go default set", id)
			}
		}
		if len(c.CurvePreferences) == 0 {
			t.Fatal("CurvePreferences must not be empty")
		}
	}
}

func TestShuffledCipherSuitesVary(t *testing.T) {
	// Probabilistic by design, but with 10+ suites the chance of 30
	// identical shuffles is negligible; failure means shuffling is broken.
	first := shuffledCipherSuites()
	same := 0
	for i := 0; i < 30; i++ {
		next := shuffledCipherSuites()
		equal := len(first) == len(next)
		if equal {
			for j := range first {
				if first[j] != next[j] {
					equal = false
					break
				}
			}
		}
		if equal {
			same++
		}
	}
	if same == 30 {
		t.Fatal("shuffledCipherSuites never varied order across 30 draws")
	}
}
