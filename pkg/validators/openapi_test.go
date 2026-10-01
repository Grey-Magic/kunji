package validators

import (
	"testing"

	"github.com/Grey-Magic/kunji/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestLooksOpenAPI_OpenAPI3(t *testing.T) {
	body := []byte(`{"openapi":"3.0.0","info":{"title":"X"},"paths":{}}`)
	assert.True(t, LooksOpenAPI(body))
}

func TestLooksOpenAPI_Swagger2(t *testing.T) {
	body := []byte(`{"swagger":"2.0","info":{"title":"X"},"paths":{}}`)
	assert.True(t, LooksOpenAPI(body))
}

func TestLooksOpenAPI_RegularResponseRejected(t *testing.T) {
	body := []byte(`{"choices":[{"text":"hi"}]}`)
	assert.False(t, LooksOpenAPI(body))
}

func TestLooksOpenAPI_EmptyBodyRejected(t *testing.T) {
	assert.False(t, LooksOpenAPI(nil))
	assert.False(t, LooksOpenAPI([]byte("")))
}

func TestExtractOpenAPIMetadata_OpenAPI3(t *testing.T) {
	body := []byte(`{
		"openapi":"3.0.3",
		"info":{"title":"My API"},
		"paths":{
			"/v1/models":{"get":{},"post":{}},
			"/v1/chat":{"post":{}}
		},
		"components":{"schemas":{"Model":{},"User":{},"Token":{}}}
	}`)

	r := &models.ValidationResult{}
	ExtractOpenAPIMetadata(body, r)

	assert.Equal(t, "3.0.3", r.Extra["openapi_version"])
	assert.Equal(t, "My API", r.Extra["openapi_title"])
	assert.EqualValues(t, 3, r.Extra["openapi_paths"])
	assert.EqualValues(t, 3, r.Extra["openapi_schemas"])
}

func TestExtractOpenAPIMetadata_Swagger2(t *testing.T) {
	body := []byte(`{
		"swagger":"2.0",
		"info":{"title":"Old API"},
		"paths":{"/v1/x":{"get":{}}},
		"definitions":{"Foo":{},"Bar":{}}
	}`)
	r := &models.ValidationResult{}
	ExtractOpenAPIMetadata(body, r)
	assert.Equal(t, "2.0", r.Extra["openapi_version"])
	assert.EqualValues(t, 1, r.Extra["openapi_paths"])
	assert.EqualValues(t, 2, r.Extra["openapi_schemas"])
}

func TestExtractOpenAPIMetadata_IgnoresNonOpenAPIBody(t *testing.T) {
	r := &models.ValidationResult{}
	ExtractOpenAPIMetadata([]byte(`{"ok":true}`), r)
	assert.Empty(t, r.Extra)
}

func TestExtractOpenAPIMetadata_NilResultIsSafe(t *testing.T) {
	// No panic, no work.
	ExtractOpenAPIMetadata([]byte(`{"openapi":"3.0.0"}`), nil)
}
