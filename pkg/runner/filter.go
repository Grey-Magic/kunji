package runner

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Grey-Magic/kunji/pkg/models"
)

// FilterExpr is a parsed filter clause.
//
// Syntax: "field=value" or "field!=value". value can be a single string or a
// comma-separated list ("openai,stripe") which is treated as "any of these".
// Currently supported fields: provider, is_valid, status_code, error_code, key.
type FilterExpr struct {
	Field     string
	Op        string // "=" or "!="
	Values    []string
	IsNumeric bool
}

// ParseFilterExprs parses the raw --filter arguments into a slice of
// expressions. Returns an error for unknown fields or malformed input.
//
// An empty input returns a nil slice, which matches everything (no filter).
func ParseFilterExprs(raw []string) ([]FilterExpr, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]FilterExpr, 0, len(raw))
	for _, r := range raw {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		// Split on = or != (first occurrence)
		var op string
		var rest string
		if idx := strings.Index(r, "!="); idx >= 0 {
			op = "!="
			rest = r[idx+2:]
		} else if idx := strings.Index(r, "="); idx >= 0 {
			op = "="
			rest = r[idx+1:]
		} else {
			return nil, fmt.Errorf("invalid filter %q (expected field=value)", r)
		}
		field := strings.TrimSpace(r[:strings.Index(r, op)])
		values := splitTrimmed(rest, ",")

		expr := FilterExpr{Field: field, Op: op, Values: values}
		switch field {
		case "provider", "error_code", "key":
			if len(values) == 0 || values[0] == "" {
				return nil, fmt.Errorf("filter %q: empty value", r)
			}
		case "is_valid":
			if len(values) != 1 {
				return nil, fmt.Errorf("filter %q: is_valid expects exactly one value (true|false)", r)
			}
			switch values[0] {
			case "true", "1", "yes":
				values[0] = "true"
			case "false", "0", "no":
				values[0] = "false"
			default:
				return nil, fmt.Errorf("filter %q: is_valid must be true or false", r)
			}
		case "status_code":
			expr.IsNumeric = true
			for _, v := range values {
				if _, err := strconv.Atoi(v); err != nil {
					return nil, fmt.Errorf("filter %q: status_code must be numeric", r)
				}
			}
		default:
			return nil, fmt.Errorf("filter %q: unknown field %q", r, field)
		}
		out = append(out, expr)
	}
	return out, nil
}

// Match reports whether result satisfies every expression. A nil/empty exprs
// matches everything.
func Match(exprs []FilterExpr, result *models.ValidationResult) bool {
	if len(exprs) == 0 || result == nil {
		return true
	}
	for _, e := range exprs {
		if !matchOne(e, result) {
			return false
		}
	}
	return true
}

func matchOne(e FilterExpr, r *models.ValidationResult) bool {
	var fieldVal string

	switch e.Field {
	case "provider":
		fieldVal = r.Provider
	case "is_valid":
		if r.IsValid {
			fieldVal = "true"
		} else {
			fieldVal = "false"
		}
	case "status_code":
		fieldVal = strconv.Itoa(r.StatusCode)
	case "error_code":
		fieldVal = r.InvalidReason
	case "key":
		fieldVal = r.Key
	default:
		return false
	}

	matched := contains(e.Values, fieldVal)
	if e.Op == "!=" {
		return !matched
	}
	return matched
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

func splitTrimmed(s, sep string) []string {
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
