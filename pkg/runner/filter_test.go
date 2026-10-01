package runner

import (
	"testing"

	"github.com/Grey-Magic/kunji/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFilterExprs_EmptyIsAll(t *testing.T) {
	exprs, err := ParseFilterExprs(nil)
	require.NoError(t, err)
	assert.Empty(t, exprs)

	exprs, err = ParseFilterExprs([]string{"", "  "})
	require.NoError(t, err)
	assert.Empty(t, exprs)
}

func TestParseFilterExprs_ProviderList(t *testing.T) {
	exprs, err := ParseFilterExprs([]string{"provider=openai,stripe"})
	require.NoError(t, err)
	require.Len(t, exprs, 1)
	assert.Equal(t, "provider", exprs[0].Field)
	assert.Equal(t, "=", exprs[0].Op)
	assert.Equal(t, []string{"openai", "stripe"}, exprs[0].Values)
}

func TestParseFilterExprs_NotEqual(t *testing.T) {
	exprs, err := ParseFilterExprs([]string{"provider!=openai"})
	require.NoError(t, err)
	require.Len(t, exprs, 1)
	assert.Equal(t, "!=", exprs[0].Op)
}

func TestParseFilterExprs_IsValidCoerces(t *testing.T) {
	exprs, err := ParseFilterExprs([]string{"is_valid=1", "is_valid=no"})
	require.NoError(t, err)
	assert.Equal(t, "true", exprs[0].Values[0])
	assert.Equal(t, "false", exprs[1].Values[0])
}

func TestParseFilterExprs_UnknownField(t *testing.T) {
	_, err := ParseFilterExprs([]string{"bogus=1"})
	assert.Error(t, err)
}

func TestParseFilterExprs_StatusCodeNumeric(t *testing.T) {
	_, err := ParseFilterExprs([]string{"status_code=200,401"})
	require.NoError(t, err)
	_, err = ParseFilterExprs([]string{"status_code=abc"})
	assert.Error(t, err)
}

func TestMatch_NilExprsAlwaysTrue(t *testing.T) {
	r := &models.ValidationResult{Provider: "openai", IsValid: true}
	assert.True(t, Match(nil, r))
}

func TestMatch_ProviderList(t *testing.T) {
	exprs, _ := ParseFilterExprs([]string{"provider=openai,stripe"})
	mustMatch := &models.ValidationResult{Provider: "openai"}
	miss := &models.ValidationResult{Provider: "github"}
	assert.True(t, Match(exprs, mustMatch))
	assert.False(t, Match(exprs, miss))
}

func TestMatch_CombinedWithAND(t *testing.T) {
	exprs, _ := ParseFilterExprs([]string{"provider=openai", "is_valid=true"})
	assert.True(t, Match(exprs, &models.ValidationResult{Provider: "openai", IsValid: true}))
	assert.False(t, Match(exprs, &models.ValidationResult{Provider: "openai", IsValid: false}))
	assert.False(t, Match(exprs, &models.ValidationResult{Provider: "stripe", IsValid: true}))
}

func TestMatch_NotEqual(t *testing.T) {
	exprs, _ := ParseFilterExprs([]string{"provider!=openai"})
	assert.True(t, Match(exprs, &models.ValidationResult{Provider: "stripe"}))
	assert.False(t, Match(exprs, &models.ValidationResult{Provider: "openai"}))
}

func TestMatch_StatusCode(t *testing.T) {
	exprs, _ := ParseFilterExprs([]string{"status_code=200,401"})
	assert.True(t, Match(exprs, &models.ValidationResult{StatusCode: 200}))
	assert.True(t, Match(exprs, &models.ValidationResult{StatusCode: 401}))
	assert.False(t, Match(exprs, &models.ValidationResult{StatusCode: 500}))
}
