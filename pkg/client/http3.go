package client

import (
	"net/http"
	"net/url"
)

// HTTP/3 / QUIC support is opt-in via --http3.
//
// The implementation depends on github.com/quic-go/quic-go, which is NOT yet
// added to go.mod. When the dep is available, the build path is:
//
//  1. go get github.com/quic-go/quic-go
//  2. Wire a quic-go RoundTripper into a parallel http.Client.
//  3. Override transport on the shared client when useHTTP3 is true.
//
// To make progress in the meantime, this file documents the integration
// point and exposes a public knob so callers (validate.go) can pass through
// the --http3 flag without compile errors.
func EnableHTTP3(on bool) { http3Enabled = on }

var http3Enabled = false

// HTTP3Enabled reports whether the HTTP/3 flag is on. Used by tests and by
// future code paths that need to know whether to route to QUIC.
func HTTP3Enabled() bool { return http3Enabled }

// BuildHTTP3Client is the planned integration hook for github.com/quic-go/quic-go.
//
// When the dependency is added, replace the stub body with code that builds
// a *http.Client whose Transport is a *http3.RoundTripper. The return value
// should be safe to use in place of the result of NewHTTPClient for any
// request that supports HTTP/3 (most do via Alt-Svc; some need an explicit
// URL scheme change).
//
// Until quic-go is added this always returns nil so callers can detect the
// unavailable path:
//
//	if c := BuildHTTP3Client(...); c != nil {
//	    r.sharedHTTPClient = c
//	} else {
//	    // fall back to HTTP/2
//	}
func BuildHTTP3Client(host string, proxyFunc func(*http.Request) (*url.URL, error)) *http.Client {
	// TODO(quic): replace with real implementation once the dep is added.
	_ = host
	_ = proxyFunc
	return nil
}
