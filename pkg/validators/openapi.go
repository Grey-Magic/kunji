package validators

import (
	"regexp"

	"github.com/Grey-Magic/kunji/pkg/models"
	"github.com/tidwall/gjson"
)

// LooksOpenAPI returns true when the body has the basic shape of an OpenAPI
// or Swagger document. We only sniff well-known top-level keys to avoid
// false positives on regular API responses.
func LooksOpenAPI(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	// Both "openapi" (3.x) and "swagger" (2.x) roots.
	if gjson.GetBytes(body, "openapi").Exists() {
		return true
	}
	if gjson.GetBytes(body, "swagger").Exists() {
		return true
	}
	return false
}

// ExtractOpenAPIMetadata walks an OpenAPI/Swagger document and stamps the
// results on the ValidationResult. Caller should check LooksOpenAPI first.
//
// Adds (when present):
//
//	extra.openapi_paths     -> number of path entries
//	extra.openapi_schemas  -> number of schemas under components / definitions
//	extra.openapi_version  -> "3.x.x" or "2.x"
//	extra.openapi_title    -> info.title
func ExtractOpenAPIMetadata(body []byte, result *models.ValidationResult) {
	if !LooksOpenAPI(body) || result == nil {
		return
	}
	if result.Extra == nil {
		result.Extra = make(map[string]interface{})
	}

	if v := gjson.GetBytes(body, "openapi"); v.Exists() {
		result.Extra["openapi_version"] = v.String()
	} else if v := gjson.GetBytes(body, "swagger"); v.Exists() {
		result.Extra["openapi_version"] = v.String()
	}
	if v := gjson.GetBytes(body, "info.title"); v.Exists() {
		result.Extra["openapi_title"] = v.String()
	}

	// Number of paths (operations).
	if paths := gjson.GetBytes(body, "paths").Map(); len(paths) > 0 {
		count := 0
		for _, p := range paths {
			methods := regexp.MustCompile(`(?i)^(get|post|put|delete|patch|head|options)$`)
			for k := range p.Map() {
				if methods.MatchString(k) {
					count++
				}
			}
		}
		result.Extra["openapi_paths"] = count
	}

	// Schemas: OpenAPI 3.x uses components.schemas; Swagger 2.x uses definitions.
	schemas := gjson.GetBytes(body, "components.schemas").Map()
	if len(schemas) == 0 {
		schemas = gjson.GetBytes(body, "definitions").Map()
	}
	if len(schemas) > 0 {
		result.Extra["openapi_schemas"] = len(schemas)
	}
}
