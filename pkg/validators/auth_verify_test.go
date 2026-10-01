package validators

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Grey-Magic/kunji/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// allowLoopback disables the SSRF guard so httptest servers (127.0.0.1)
// are reachable, following the pattern in regression_test.go.
func allowLoopback(t *testing.T) {
	t.Helper()
	utils.SkipSSRFCheck = true
	t.Cleanup(func() { utils.SkipSSRFCheck = false })
}

// captureServer records the Authorization-relevant headers it sees.
type captureServer struct {
	auth      string
	custom    string
	status    int
	srv       *httptest.Server
	lastQuery string
}

func newCaptureServer(status int) *captureServer {
	c := &captureServer{status: status}
	c.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.auth = r.Header.Get("Authorization")
		c.custom = r.Header.Get("X-Custom")
		c.lastQuery = r.URL.RawQuery
		w.WriteHeader(c.status)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	return c
}

func (c *captureServer) close() { c.srv.Close() }

func validateAgainst(t *testing.T, cfg ValidationConfig, key string) (string, string, bool) {
	t.Helper()
	allowLoopback(t)
	c := newCaptureServer(200)
	defer c.close()
	cfg.URL = c.srv.URL
	v := NewGenericValidatorWithClient(ProviderConfig{Name: "t", Validation: cfg}, c.srv.Client(), nil)
	res, err := v.Validate(context.Background(), key)
	require.NoError(t, err)
	require.NotNil(t, res)
	return c.auth, c.custom, res.IsValid
}

func TestCompositeBearerSendsSecretOnly(t *testing.T) {
	v := &GenericValidator{}

	// Composite host:secret key + templated host in the endpoint URL: the
	// header must carry only the secret part (previously the whole
	// "host:secret" string went out as the Bearer token and always 401'd).
	req, _ := http.NewRequest("GET", "https://example.com/x", nil)
	v.applyAuth(req, "bearer", "myhost:SECRET123", "https://{{key.client_id}}/api/v1/x")
	assert.Equal(t, "Bearer SECRET123", req.Header.Get("Authorization"))

	// Fixed host + plain key: whole key still sent.
	req2, _ := http.NewRequest("GET", "https://fixed.example.com/x", nil)
	v.applyAuth(req2, "bearer", "PLAINKEY", "https://fixed.example.com/x")
	assert.Equal(t, "Bearer PLAINKEY", req2.Header.Get("Authorization"))

	// Composite key but fixed host (no template consumed): unchanged
	// behavior, whole key sent.
	req3, _ := http.NewRequest("GET", "https://fixed.example.com/x", nil)
	v.applyAuth(req3, "bearer", "myhost:SECRET123", "https://fixed.example.com/x")
	assert.Equal(t, "Bearer myhost:SECRET123", req3.Header.Get("Authorization"))

	// Same split rule applies to header: and query: auth.
	req4, _ := http.NewRequest("GET", "https://fixed.example.com/x", nil)
	v.applyAuth(req4, "header:X-Key", "myhost:SECRET123", "https://{{key.client_id}}/x")
	assert.Equal(t, "SECRET123", req4.Header.Get("X-Key"))
}

func TestHeaderPrefixForm(t *testing.T) {
	cfg := ValidationConfig{
		Method: "GET",
		URL:    "https://fixed.example.com/x",
		Auth:   "header:Authorization:token ",
	}
	auth, _, valid := validateAgainst(t, cfg, "SECRET123")
	assert.Equal(t, "token SECRET123", auth)
	assert.True(t, valid)
}

func TestHeaderTemplateSubstitution(t *testing.T) {
	cfg := ValidationConfig{
		Method: "GET",
		URL:    "https://fixed.example.com/x",
		Auth:   "none",
		Headers: map[string]string{
			"X-Custom": "{{key.secret}}",
		},
	}
	_, custom, valid := validateAgainst(t, cfg, "acct:SECRET123")
	assert.Equal(t, "SECRET123", custom)
	assert.True(t, valid)
}

func TestBasicCompositeUnchanged(t *testing.T) {
	allowLoopback(t)
	var gotUser, gotPass string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, _ = r.BasicAuth()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cfg := ValidationConfig{Method: "GET", URL: srv.URL, Auth: "basic_composite"}
	v := NewGenericValidatorWithClient(ProviderConfig{Name: "t", Validation: cfg}, srv.Client(), nil)
	res, err := v.Validate(context.Background(), "alice:s3cret")
	require.NoError(t, err)
	assert.True(t, res.IsValid)
	assert.Equal(t, "alice", gotUser)
	assert.Equal(t, "s3cret", gotPass)
}

func TestExpectedStatusHonored(t *testing.T) {
	allowLoopback(t)
	new404Validator := func(extra ValidationConfig) (*GenericValidator, *httptest.Server) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(404)
		}))
		extra.Method = "GET"
		extra.URL = srv.URL
		v := NewGenericValidatorWithClient(ProviderConfig{Name: "t", Validation: extra}, srv.Client(), nil)
		return v, srv
	}

	// 404 listed as expected → valid (HIBP-style: key good, object unknown).
	v, srv := new404Validator(ValidationConfig{ExpectedStatus: []int{200, 404}})
	res, err := v.Validate(context.Background(), "k")
	srv.Close()
	require.NoError(t, err)
	assert.True(t, res.IsValid, "404 in expected_status should validate")

	// No expected_status → historical behavior: 404 invalid.
	v2, srv2 := new404Validator(ValidationConfig{})
	res2, err := v2.Validate(context.Background(), "k")
	srv2.Close()
	require.NoError(t, err)
	assert.False(t, res2.IsValid, "404 without expected_status should stay invalid")
}
