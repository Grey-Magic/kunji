package validators

import (
	"encoding/json"
	"testing"

	"github.com/Grey-Magic/kunji/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestExtractJSONPath(t *testing.T) {
	tests := []struct {
		name     string
		data     interface{}
		path     string
		expected interface{}
	}{
		{
			name:     "simple field",
			data:     map[string]interface{}{"name": "test"},
			path:     "name",
			expected: "test",
		},
		{
			name:     "nested field",
			data:     map[string]interface{}{"user": map[string]interface{}{"name": "john"}},
			path:     "user.name",
			expected: "john",
		},
		{
			name:     "array index",
			data:     []interface{}{"a", "b", "c"},
			path:     "1",
			expected: "b",
		},
		{
			name:     "nested array",
			data:     map[string]interface{}{"users": []interface{}{map[string]interface{}{"name": "alice"}}},
			path:     "users.0.name",
			expected: "alice",
		},
		{
			name:     "non-existent path",
			data:     map[string]interface{}{"name": "test"},
			path:     "email",
			expected: nil,
		},
		{
			name:     "deeply nested",
			data:     map[string]interface{}{"a": map[string]interface{}{"b": map[string]interface{}{"c": "deep"}}},
			path:     "a.b.c",
			expected: "deep",
		},
		{
			name:     "index out of bounds",
			data:     []interface{}{"a", "b"},
			path:     "5",
			expected: nil,
		},
		{
			name:     "empty path",
			data:     map[string]interface{}{"name": "test"},
			path:     "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractJSONPath(tt.data, tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected float64
	}{
		{"float64", 3.14, 3.14},
		{"float32", float32(2.5), 2.5},
		{"int", 10, 10.0},
		{"int64", int64(100), 100.0},
		{"string number", "5.5", 5.5},
		{"string non-number", "abc", 0.0},
		{"json number", json.Number("123.45"), 123.45},
		{"nil", nil, 0.0},
		{"boolean", true, 0.0},
		{"array", []interface{}{}, 0.0},
		{"map", map[string]interface{}{}, 0.0},
		{"negative number", -5.5, -5.5},
		{"zero", 0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toFloat64(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenericValidator_ExtractValidationMetadata(t *testing.T) {
	t.Run("balance extraction", func(t *testing.T) {
		cfg := ProviderConfig{
			Name: "test",
			Validation: ValidationConfig{
				URL: "https://api.example.com",
			},
			MetadataFromValidation: &MetadataFromValidation{
				BalancePath: "balance",
			},
		}

		validator := &GenericValidator{config: cfg}
		result := &models.ValidationResult{}

		body := []byte(`{"balance": 100.50}`)
		validator.extractValidationMetadata(body, result)

		assert.Equal(t, 100.50, result.Balance)
	})

	t.Run("balance with subtract path", func(t *testing.T) {
		cfg := ProviderConfig{
			Name: "test",
			Validation: ValidationConfig{
				URL: "https://api.example.com",
			},
			MetadataFromValidation: &MetadataFromValidation{
				BalancePath:         "total",
				BalanceSubtractPath: "used",
			},
		}

		validator := &GenericValidator{config: cfg}
		result := &models.ValidationResult{}

		body := []byte(`{"total": 100.0, "used": 25.5}`)
		validator.extractValidationMetadata(body, result)

		assert.Equal(t, 74.5, result.Balance, "should subtract used from total")
	})

	t.Run("name extraction", func(t *testing.T) {
		cfg := ProviderConfig{
			Name: "test",
			Validation: ValidationConfig{
				URL: "https://api.example.com",
			},
			MetadataFromValidation: &MetadataFromValidation{
				NamePath: "user.name",
			},
		}

		validator := &GenericValidator{config: cfg}
		result := &models.ValidationResult{}

		body := []byte(`{"user": {"name": "John"}}`)
		validator.extractValidationMetadata(body, result)

		assert.Equal(t, "John", result.AccountName)
	})

	t.Run("name fallback path", func(t *testing.T) {
		cfg := ProviderConfig{
			Name: "test",
			Validation: ValidationConfig{
				URL: "https://api.example.com",
			},
			MetadataFromValidation: &MetadataFromValidation{
				NamePath:         "user.display_name",
				NameFallbackPath: "user.username",
			},
		}

		validator := &GenericValidator{config: cfg}
		result := &models.ValidationResult{}

		body := []byte(`{"user": {"username": "johndoe"}}`)
		validator.extractValidationMetadata(body, result)

		assert.Equal(t, "johndoe", result.AccountName, "should use fallback when primary is empty")
	})

	t.Run("email extraction", func(t *testing.T) {
		cfg := ProviderConfig{
			Name: "test",
			Validation: ValidationConfig{
				URL: "https://api.example.com",
			},
			MetadataFromValidation: &MetadataFromValidation{
				EmailPath: "email",
			},
		}

		validator := &GenericValidator{config: cfg}
		result := &models.ValidationResult{}

		body := []byte(`{"email": "test@example.com"}`)
		validator.extractValidationMetadata(body, result)

		assert.Equal(t, "test@example.com", result.Email)
	})

	t.Run("no metadata config", func(t *testing.T) {
		cfg := ProviderConfig{
			Name: "test",
			Validation: ValidationConfig{
				URL: "https://api.example.com",
			},
		}

		validator := &GenericValidator{config: cfg}
		result := &models.ValidationResult{}

		body := []byte(`{"balance": 100}`)
		validator.extractValidationMetadata(body, result)

		assert.Equal(t, 0.0, result.Balance)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		cfg := ProviderConfig{
			Name: "test",
			Validation: ValidationConfig{
				URL: "https://api.example.com",
			},
			MetadataFromValidation: &MetadataFromValidation{
				BalancePath: "balance",
			},
		}

		validator := &GenericValidator{config: cfg}
		result := &models.ValidationResult{}

		body := []byte(`not valid json`)
		validator.extractValidationMetadata(body, result)

		assert.Equal(t, 0.0, result.Balance, "should handle invalid JSON gracefully")
	})
}
