package client

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

// HTTP/3 (QUIC) support, opt-in via --http3.
//
// Design constraints that shaped this file:
//
//  1. Most provider APIs do not speak HTTP/3. Blindly switching the shared
//     client to H3 would turn every H2-only host into a false-invalid, so
//     H3 is attempted per host with a bounded timeout and a persistent
//     per-host outcome cache: hosts that fail H3 go straight to H2 next
//     time (racingRoundTripper below).
//  2. HTTP/3 cannot run through a CONNECT proxy (that needs CONNECT-UDP /
//     MASQUE, which proxies rarely support), so the H3 path is only built
//     when no --proxy is configured. With a proxy set, --http3 is accepted
//     and ignored in favor of HTTP/2.
//  3. Request bodies are replay-safe: POST bodies built from rewindable
//     readers are rewound before the H2 fallback. Bodies without GetBody
//     skip H3 entirely rather than risk sending a truncated retry.
//
// h3AttemptTimeout bounds a single H3 attempt. It is a var (not const) so
// tests can shrink it; production default keeps the worst case (H3 timeout
// + full H2 attempt) inside the default 15s request timeout.
var h3AttemptTimeout = 3 * time.Second

var http3Enabled = false

// EnableHTTP3 records the --http3 flag. It must be called before the
// shared client is constructed (validate.go does this ahead of the
// runner/factory setup) because transports are built once.
func EnableHTTP3(on bool) { http3Enabled = on }

// HTTP3Enabled reports whether the HTTP/3 flag is on.
func HTTP3Enabled() bool { return http3Enabled }

// newH3Transport builds a QUIC-backed HTTP/3 transport sharing the same
// TLS posture as the H2 client (verification on, TLS 1.2 floor). The
// cipher/curve shuffle from randomizedTLSConfig applies to the QUIC
// handshake too; only ALPN is overridden to h3.
func newH3Transport() *http3.Transport {
	cfg := randomizedTLSConfig()
	cfg.NextProtos = []string{"h3"}
	return &http3.Transport{
		TLSClientConfig: cfg,
		QUICConfig:      &quic.Config{},
	}
}

// racingRoundTripper tries HTTP/3 first per host and falls back to the
// wrapped (HTTP/2) transport on any failure. Hosts are remembered after
// the first attempt so steady-state traffic pays no racing overhead.
type racingRoundTripper struct {
	h3 *http3.Transport
	h2 http.RoundTripper
	// host -> true when H3 succeeded there at least once.
	cache sync.Map
}

func newRacingRoundTripper(h2 http.RoundTripper) *racingRoundTripper {
	return &racingRoundTripper{h3: newH3Transport(), h2: h2}
}

func (r *racingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL == nil || req.URL.Scheme != "https" {
		return r.h2.RoundTrip(req)
	}
	host := req.URL.Hostname()
	if v, ok := r.cache.Load(host); ok && !v.(bool) {
		return r.h2.RoundTrip(req)
	}
	// Rewindable bodies can be replayed on fallback; anything else skips
	// H3 to avoid sending a truncated second attempt.
	h3req := req
	if req.Body != nil {
		if req.GetBody == nil {
			r.cache.Store(host, false)
			return r.h2.RoundTrip(req)
		}
		body, err := req.GetBody()
		if err != nil {
			r.cache.Store(host, false)
			return r.h2.RoundTrip(req)
		}
		h3req = req.Clone(req.Context())
		h3req.Body = body
	}
	ctx, cancel := context.WithTimeout(h3req.Context(), h3AttemptTimeout)
	defer cancel()
	h3req = h3req.WithContext(ctx)

	resp, err := r.h3.RoundTrip(h3req)
	if err == nil {
		r.cache.Store(host, true)
		return resp, nil
	}
	r.cache.Store(host, false)
	return r.h2.RoundTrip(req)
}

// CloseIdleConnections relays connection cleanup to both transports.
func (r *racingRoundTripper) CloseIdleConnections() {
	r.h3.CloseIdleConnections()
	if c, ok := r.h2.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}
