package validators

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestValidationConfig_ExtendedSchema(t *testing.T) {
	yamlDoc := `
name: test-extended
category: test
key_prefixes: ["ext-"]
validation:
  method: GET
  url: https://example.test/whoami
  auth: bearer
  expected_status: [200, 204]
  retry_on_status: [429, 503]
  response_match:
    body_regex: '"active":\s*true'
    body_jsonpath: $.status
    jsonpath_value: "active"
    header_contains: "x-tier: pro"
  failure_when:
    body_regex: 'invalid_api_key'
    body_jsonpath: $.error.code
    jsonpath_value: "unauthorized"
`
	var cfg ProviderConfig
	require.NoError(t, yaml.Unmarshal([]byte(yamlDoc), &cfg))

	assert.Equal(t, "test-extended", cfg.Name)
	assert.Equal(t, []int{200, 204}, cfg.Validation.ExpectedStatus)
	assert.Equal(t, []int{429, 503}, cfg.Validation.RetryOnStatus)
	require.NotNil(t, cfg.Validation.ResponseMatch)
	assert.Equal(t, `"active":\s*true`, cfg.Validation.ResponseMatch.BodyRegex)
	assert.Equal(t, "$.status", cfg.Validation.ResponseMatch.BodyJSONPath)
	assert.Equal(t, "active", cfg.Validation.ResponseMatch.JSONPathValue)
	assert.Equal(t, "x-tier: pro", cfg.Validation.ResponseMatch.HeaderContains)
	require.NotNil(t, cfg.Validation.FailureWhen)
	assert.Equal(t, "invalid_api_key", cfg.Validation.FailureWhen.BodyRegex)
	assert.Equal(t, "$.error.code", cfg.Validation.FailureWhen.BodyJSONPath)
	assert.Equal(t, "unauthorized", cfg.Validation.FailureWhen.JSONPathValue)
}

func TestValidationConfig_LegacySchemaStillParses(t *testing.T) {
	// Older provider YAMLs without the new fields must continue to parse.
	yamlDoc := `
name: test-legacy
category: test
key_prefixes: ["leg-"]
validation:
  method: GET
  url: https://example.test/whoami
  auth: bearer
`
	var cfg ProviderConfig
	require.NoError(t, yaml.Unmarshal([]byte(yamlDoc), &cfg))

	assert.Equal(t, "test-legacy", cfg.Name)
	assert.Nil(t, cfg.Validation.ExpectedStatus)
	assert.Nil(t, cfg.Validation.RetryOnStatus)
	assert.Nil(t, cfg.Validation.ResponseMatch)
	assert.Nil(t, cfg.Validation.FailureWhen)
}

func TestMatchesResponseCriteria(t *testing.T) {
	v := &GenericValidator{}

	t.Run("nil matcher accepts everything", func(t *testing.T) {
		assert.True(t, v.matchesResponseCriteria([]byte(`{"x":1}`), nil, nil))
	})

	t.Run("body regex matches", func(t *testing.T) {
		body := []byte(`{"active": true, "name": "ok"}`)
		rm := &ResponseMatch{BodyRegex: `"active":\s*true`}
		assert.True(t, v.matchesResponseCriteria(body, nil, rm))
	})

	t.Run("body regex mismatch", func(t *testing.T) {
		body := []byte(`{"active": false}`)
		rm := &ResponseMatch{BodyRegex: `"active":\s*true`}
		assert.False(t, v.matchesResponseCriteria(body, nil, rm))
	})

	t.Run("body jsonpath matches expected value", func(t *testing.T) {
		body := []byte(`{"status": "active"}`)
		rm := &ResponseMatch{BodyJSONPath: "$.status", JSONPathValue: "active"}
		assert.True(t, v.matchesResponseCriteria(body, nil, rm))
	})

	t.Run("body jsonpath mismatch", func(t *testing.T) {
		body := []byte(`{"status": "suspended"}`)
		rm := &ResponseMatch{BodyJSONPath: "$.status", JSONPathValue: "active"}
		assert.False(t, v.matchesResponseCriteria(body, nil, rm))
	})

	t.Run("header contains", func(t *testing.T) {
		headers := map[string][]string{"X-Tier": {"pro"}}
		body := []byte(`{}`)
		rm := &ResponseMatch{HeaderContains: "x-tier: pro"}
		assert.True(t, v.matchesResponseCriteria(body, headers, rm))
	})

	t.Run("header contains missing header fails", func(t *testing.T) {
		headers := map[string][]string{"X-Other": {"v"}}
		body := []byte(`{}`)
		rm := &ResponseMatch{HeaderContains: "x-tier: pro"}
		assert.False(t, v.matchesResponseCriteria(body, headers, rm))
	})

	t.Run("invalid regex treated as mismatch", func(t *testing.T) {
		body := []byte(`anything`)
		rm := &ResponseMatch{BodyRegex: "[unclosed"}
		assert.False(t, v.matchesResponseCriteria(body, nil, rm))
	})
}

func TestMatchesFailureWhen(t *testing.T) {
	v := &GenericValidator{}

	t.Run("nil failureWhen never triggers", func(t *testing.T) {
		body := []byte(`{"error":"invalid_api_key"}`)
		assert.False(t, v.matchesFailureWhen(body, nil))
	})

	t.Run("body regex triggers", func(t *testing.T) {
		body := []byte(`{"error":"invalid_api_key"}`)
		fw := &FailureWhen{BodyRegex: "invalid_api_key"}
		assert.True(t, v.matchesFailureWhen(body, fw))
	})

	t.Run("jsonpath + value matches", func(t *testing.T) {
		body := []byte(`{"error":{"code":"unauthorized"}}`)
		fw := &FailureWhen{BodyJSONPath: "$.error.code", JSONPathValue: "unauthorized"}
		assert.True(t, v.matchesFailureWhen(body, fw))
	})

	t.Run("jsonpath present but value differs", func(t *testing.T) {
		body := []byte(`{"error":{"code":"rate_limited"}}`)
		fw := &FailureWhen{BodyJSONPath: "$.error.code", JSONPathValue: "unauthorized"}
		assert.False(t, v.matchesFailureWhen(body, fw))
	})
}

func TestStatusInList(t *testing.T) {
	v := &GenericValidator{}
	assert.True(t, v.statusInList(429, []int{429, 503}))
	assert.False(t, v.statusInList(200, []int{429, 503}))
	assert.False(t, v.statusInList(200, nil))
}
