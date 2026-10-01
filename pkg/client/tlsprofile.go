package client

import (
	"crypto/rand"
	"crypto/tls"
	"math/big"
)

// randomizedTLSConfig builds a TLS client config whose ClientHello-visible
// parameters are shuffled on every construction: cipher suite preference
// order, elliptic-curve preference order. This diversifies the JA3
// fingerprint across runs and across rotated clients using only the
// standard library.
//
// What it does NOT do: mimic a specific browser profile (extension layout,
// GREASE placement, ALPN order). True browser-profile mimicry needs a
// ClientHello-spoofing library such as uTLS, which is not vendored here;
// see the evasion notes in USAGE.md. Functional parameters are pinned:
// MinVersion TLS 1.2 (still negotiates 1.3), ALPN ["h2", "http/1.1"] in
// that order (reordering ALPN would silently downgrade connections to
// HTTP/1.1), verification always on.
func randomizedTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: false,
		NextProtos:         []string{"h2", "http/1.1"},
		CipherSuites:       shuffledCipherSuites(),
		CurvePreferences:   shuffledCurves(),
	}
}

// shuffledCipherSuites returns Go's default secure suite list in random
// order. Only TLS 1.2 suites are configurable; TLS 1.3 suites always ride
// first in fixed order, so this shuffles the remainder of the ClientHello
// cipher list (still the bulk of the JA3 cipher segment).
func shuffledCipherSuites() []uint16 {
	defs := tls.CipherSuites()
	ids := make([]uint16, 0, len(defs))
	for _, d := range defs {
		ids = append(ids, d.ID)
	}
	shuffleUint16(ids)
	return ids
}

// defaultCurves mirrors Go's internal default curve preference order
// (crypto/tls has no exported getter for it).
var defaultCurves = []tls.CurveID{
	tls.X25519,
	tls.CurveP256,
	tls.CurveP384,
	tls.CurveP521,
}

// shuffledCurves returns the default curve list in random order, which
// reorders the supported_groups extension in the ClientHello.
func shuffledCurves() []tls.CurveID {
	out := make([]tls.CurveID, len(defaultCurves))
	copy(out, defaultCurves)
	// Fisher-Yates with crypto/rand; falls back to unshuffled on failure
	// (fail-closed: handshake still works, fingerprint just repeats).
	for i := len(out) - 1; i > 0; i-- {
		j, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return defaultCurves
		}
		out[i], out[j.Int64()] = out[j.Int64()], out[i]
	}
	return out
}

func shuffleUint16(s []uint16) {
	for i := len(s) - 1; i > 0; i-- {
		j, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return
		}
		s[i], s[j.Int64()] = s[j.Int64()], s[i]
	}
}
