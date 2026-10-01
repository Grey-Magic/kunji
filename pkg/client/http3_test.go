package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// shrinkH3Timeout bounds the QUIC attempt for tests (loopback UDP refusal
// is normally instant, but blackholed UDP would wait out the full window).
func shrinkH3Timeout(t *testing.T) {
	t.Helper()
	old := h3AttemptTimeout
	h3AttemptTimeout = 300 * time.Millisecond
	t.Cleanup(func() { h3AttemptTimeout = old })
}

func TestRacingSkipsH3ForPlainHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	r := newRacingRoundTripper(http.DefaultTransport)
	resp, err := r.RoundTrip(mustReq(t, "GET", srv.URL, nil))
	if err != nil {
		t.Fatalf("fallback RoundTrip: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Fatalf("unexpected body %q", body)
	}
}

func TestRacingFallsBackWhenNoQUICListener(t *testing.T) {
	shrinkH3Timeout(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	// H2 leg trusts the test server's self-signed cert.
	h2 := srv.Client().Transport
	r := newRacingRoundTripper(h2)

	resp, err := r.RoundTrip(mustReq(t, "GET", srv.URL, nil))
	if err != nil {
		t.Fatalf("fallback RoundTrip: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	// Host must now be pinned H2-only so steady state pays no racing cost.
	if v, ok := r.cache.Load(hostOf(t, srv.URL)); !ok || v.(bool) {
		t.Fatal("expected host cached as H2-only after failed H3 attempt")
	}
	// Second request serves straight from H2.
	resp2, err := r.RoundTrip(mustReq(t, "GET", srv.URL, nil))
	if err != nil {
		t.Fatalf("second RoundTrip: %v", err)
	}
	resp2.Body.Close()
}

func TestRacingReplaysPostBodyOnFallback(t *testing.T) {
	shrinkH3Timeout(t)
	const payload = `{"model":"x","messages":[]}`
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) != payload {
			w.WriteHeader(400)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	r := newRacingRoundTripper(srv.Client().Transport)
	req := mustReq(t, "POST", srv.URL, strings.NewReader(payload))
	resp, err := r.RoundTrip(req)
	if err != nil {
		t.Fatalf("fallback RoundTrip: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatal("POST body was not replayed intact on H2 fallback")
	}
}

func TestRacingSkipsH3ForUnrewindableBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	r := newRacingRoundTripper(http.DefaultTransport)
	req := mustReq(t, "POST", srv.URL, io.NopCloser(strings.NewReader("x")))
	req.GetBody = nil // unrewindable: must not attempt H3
	req.ContentLength = 1
	resp, err := r.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	resp.Body.Close()
}

func TestNewH3TransportShape(t *testing.T) {
	tr := newH3Transport()
	if tr == nil {
		t.Fatal("newH3Transport returned nil")
	}
	if tr.TLSClientConfig == nil {
		t.Fatal("H3 transport needs a TLS config")
	}
	found := false
	for _, p := range tr.TLSClientConfig.NextProtos {
		if p == "h3" {
			found = true
		}
	}
	if !found {
		t.Fatalf("H3 ALPN missing, got %v", tr.TLSClientConfig.NextProtos)
	}
	if tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("H3 transport must verify TLS")
	}
}

func mustReq(t *testing.T, method, url string, body io.Reader) *http.Request {
	t.Helper()
	var rc io.ReadCloser
	if body != nil {
		rc = io.NopCloser(body)
	}
	req, err := http.NewRequest(method, url, rc)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	return req
}

func hostOf(t *testing.T, rawURL string) string {
	t.Helper()
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return req.URL.Hostname()
}
