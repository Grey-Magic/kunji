package validators

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// LintSeverity ranks issues. Errors block a run; warnings surface problems
// the user should know about but are not necessarily fatal.
type LintSeverity string

const (
	SeverityError   LintSeverity = "error"
	SeverityWarning LintSeverity = "warning"
)

// LintIssue is a single finding produced by LintProvider / LintAll.
type LintIssue struct {
	Severity LintSeverity `json:"severity"`
	Provider string       `json:"provider,omitempty"`
	Field    string       `json:"field,omitempty"`
	Message  string       `json:"message"`
}

func (i LintIssue) String() string {
	if i.Provider == "" {
		return fmt.Sprintf("[%s] %s", i.Severity, i.Message)
	}
	if i.Field == "" {
		return fmt.Sprintf("[%s] %s: %s", i.Severity, i.Provider, i.Message)
	}
	return fmt.Sprintf("[%s] %s.%s: %s", i.Severity, i.Provider, i.Field, i.Message)
}

// validAuthSchemes enumerates the auth modes GenericValidator.applyAuth
// understands. Anything else will silently produce an unauthenticated request.
var validAuthSchemes = map[string]bool{
	"":                true,
	"none":            true,
	"bearer":          true,
	"basic":           true,
	"basic_composite": true,
}

// LintAll validates every provider in the given configs and returns the
// combined issue list. The bool return is true when no errors were found.
func LintAll(configs []ProviderConfig) ([]LintIssue, bool) {
	var issues []LintIssue

	names := make(map[string]string) // name → first file
	for _, cfg := range configs {
		if existing, ok := names[cfg.Name]; ok {
			issues = append(issues, LintIssue{
				Severity: SeverityError,
				Provider: cfg.Name,
				Message:  fmt.Sprintf("duplicate provider name (also defined in %s)", existing),
			})
			continue
		}
		names[cfg.Name] = "(embedded)"
		issues = append(issues, LintProvider(cfg)...)
	}

	hasErrors := false
	for _, i := range issues {
		if i.Severity == SeverityError {
			hasErrors = true
			break
		}
	}
	return issues, !hasErrors
}

// LintProvider runs all schema checks against one provider config.
func LintProvider(cfg ProviderConfig) []LintIssue {
	var issues []LintIssue

	if strings.TrimSpace(cfg.Name) == "" {
		issues = append(issues, LintIssue{
			Severity: SeverityError,
			Field:    "name",
			Message:  "name is required",
		})
	}

	if cfg.Category == "" {
		issues = append(issues, LintIssue{
			Severity: SeverityWarning,
			Provider: cfg.Name,
			Field:    "category",
			Message:  "no category set; detection will be unrestricted",
		})
	}

	if len(cfg.KeyPrefixes) == 0 && len(cfg.KeyPatterns) == 0 {
		issues = append(issues, LintIssue{
			Severity: SeverityError,
			Provider: cfg.Name,
			Message:  "provider has neither key_prefixes nor key_patterns; it can never match",
		})
	}

	for i, p := range cfg.KeyPatterns {
		if _, err := regexp.Compile(p); err != nil {
			issues = append(issues, LintIssue{
				Severity: SeverityError,
				Provider: cfg.Name,
				Field:    fmt.Sprintf("key_patterns[%d]", i),
				Message:  fmt.Sprintf("invalid regex: %v", err),
			})
		}
	}

	if cfg.Validation.Method == "" {
		issues = append(issues, LintIssue{
			Severity: SeverityError,
			Provider: cfg.Name,
			Field:    "validation.method",
			Message:  "validation.method is required",
		})
	}

	if cfg.Validation.URL == "" && len(cfg.Validation.Endpoints) == 0 {
		issues = append(issues, LintIssue{
			Severity: SeverityError,
			Provider: cfg.Name,
			Field:    "validation.url",
			Message:  "validation.url (or validation.endpoints) is required",
		})
	} else if cfg.Validation.URL != "" {
		issues = append(issues, lintURL(cfg.Name, "validation.url", cfg.Validation.URL)...)
	}

	for i, ep := range cfg.Validation.Endpoints {
		if ep.URL == "" {
			issues = append(issues, LintIssue{
				Severity: SeverityError,
				Provider: cfg.Name,
				Field:    fmt.Sprintf("validation.endpoints[%d].url", i),
				Message:  "endpoint URL is required",
			})
			continue
		}
		issues = append(issues, lintURL(cfg.Name, fmt.Sprintf("validation.endpoints[%d].url", i), ep.URL)...)
	}

	method := strings.ToUpper(cfg.Validation.Method)
	if method != "" && method != "GET" && method != "POST" && method != "PUT" &&
		method != "DELETE" && method != "PATCH" && method != "GRAPHQL" {
		issues = append(issues, LintIssue{
			Severity: SeverityError,
			Provider: cfg.Name,
			Field:    "validation.method",
			Message:  fmt.Sprintf("unsupported HTTP method %q (allowed: GET, POST, PUT, DELETE, PATCH, GRAPHQL)", cfg.Validation.Method),
		})
	}

	if !validAuthSchemes[cfg.Validation.Auth] &&
		!strings.HasPrefix(cfg.Validation.Auth, "header:") &&
		!strings.HasPrefix(cfg.Validation.Auth, "query:") {
		issues = append(issues, LintIssue{
			Severity: SeverityError,
			Provider: cfg.Name,
			Field:    "validation.auth",
			Message: fmt.Sprintf(
				"unknown auth scheme %q (allowed: none, bearer, basic, basic_composite, header:<name>, query:<name>)",
				cfg.Validation.Auth),
		})
	}

	if cfg.SyntaxCheck != "" && cfg.SyntaxCheck != "base64" {
		issues = append(issues, LintIssue{
			Severity: SeverityError,
			Provider: cfg.Name,
			Field:    "syntax_check",
			Message:  fmt.Sprintf("unsupported syntax_check %q (only 'base64' is recognized)", cfg.SyntaxCheck),
		})
	}

	for i, p := range cfg.CanaryPatterns {
		if _, err := regexp.Compile(p); err != nil {
			issues = append(issues, LintIssue{
				Severity: SeverityError,
				Provider: cfg.Name,
				Field:    fmt.Sprintf("canary_patterns[%d]", i),
				Message:  fmt.Sprintf("invalid regex: %v", err),
			})
		}
	}

	for i, m := range cfg.Metadata {
		if m.URL == "" {
			issues = append(issues, LintIssue{
				Severity: SeverityError,
				Provider: cfg.Name,
				Field:    fmt.Sprintf("metadata[%d].url", i),
				Message:  "metadata URL is required",
			})
			continue
		}
		issues = append(issues, lintURL(cfg.Name, fmt.Sprintf("metadata[%d].url", i), m.URL)...)

		if m.Auth != "" && !validAuthSchemes[m.Auth] &&
			!strings.HasPrefix(m.Auth, "header:") &&
			!strings.HasPrefix(m.Auth, "query:") {
			issues = append(issues, LintIssue{
				Severity: SeverityError,
				Provider: cfg.Name,
				Field:    fmt.Sprintf("metadata[%d].auth", i),
				Message:  fmt.Sprintf("unknown auth scheme %q", m.Auth),
			})
		}

		if m.RegexExtract != "" {
			if _, err := regexp.Compile(m.RegexExtract); err != nil {
				issues = append(issues, LintIssue{
					Severity: SeverityError,
					Provider: cfg.Name,
					Field:    fmt.Sprintf("metadata[%d].regex_extract", i),
					Message:  fmt.Sprintf("invalid regex: %v", err),
				})
			}
		}
	}

	if cfg.MetadataFromValidation != nil {
		mfv := cfg.MetadataFromValidation
		if mfv.RegexExtract != "" {
			if _, err := regexp.Compile(mfv.RegexExtract); err != nil {
				issues = append(issues, LintIssue{
					Severity: SeverityError,
					Provider: cfg.Name,
					Field:    "metadata_from_validation.regex_extract",
					Message:  fmt.Sprintf("invalid regex: %v", err),
				})
			}
		}
	}

	if cfg.Detection != nil {
		if cfg.Detection.MinScore < 0 {
			issues = append(issues, LintIssue{
				Severity: SeverityError,
				Provider: cfg.Name,
				Field:    "detection.min_score",
				Message:  "min_score cannot be negative",
			})
		}
	}

	if cfg.CacheTTLSeconds < 0 {
		issues = append(issues, LintIssue{
			Severity: SeverityError,
			Provider: cfg.Name,
			Field:    "cache_ttl_seconds",
			Message:  "cache_ttl_seconds cannot be negative",
		})
	}

	if cfg.RetryPolicy != nil {
		rp := cfg.RetryPolicy
		if rp.MaxAttempts < 1 {
			issues = append(issues, LintIssue{
				Severity: SeverityError,
				Provider: cfg.Name,
				Field:    "retry_policy.max_attempts",
				Message:  "max_attempts must be >= 1",
			})
		}
		if rp.InitialBackoffMs < 0 || rp.MaxBackoffMs < 0 {
			issues = append(issues, LintIssue{
				Severity: SeverityError,
				Provider: cfg.Name,
				Field:    "retry_policy.backoff_ms",
				Message:  "backoff values cannot be negative",
			})
		}
		if rp.MaxBackoffMs > 0 && rp.InitialBackoffMs > rp.MaxBackoffMs {
			issues = append(issues, LintIssue{
				Severity: SeverityWarning,
				Provider: cfg.Name,
				Field:    "retry_policy.backoff_ms",
				Message:  "initial_backoff_ms is larger than max_backoff_ms; the cap is ineffective",
			})
		}
	}

	return issues
}

// lintURL flags http(s) scheme mismatches and missing hosts.
func lintURL(provider, field, raw string) []LintIssue {
	u, err := url.Parse(raw)
	if err != nil {
		return []LintIssue{{
			Severity: SeverityError,
			Provider: provider,
			Field:    field,
			Message:  fmt.Sprintf("invalid URL: %v", err),
		}}
	}
	if u.Scheme != "" && u.Scheme != "http" && u.Scheme != "https" {
		return []LintIssue{{
			Severity: SeverityError,
			Provider: provider,
			Field:    field,
			Message:  fmt.Sprintf("URL scheme %q is not allowed (only http and https)", u.Scheme),
		}}
	}
	if u.Host == "" {
		return []LintIssue{{
			Severity: SeverityError,
			Provider: provider,
			Field:    field,
			Message:  "URL has no host",
		}}
	}
	return nil
}

// LintFile reads and lints a YAML file (single file, multiple providers).
func LintFile(path string) ([]LintIssue, bool, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, false, err
	}
	var configs []ProviderConfig
	if err := yaml.Unmarshal(data, &configs); err != nil {
		return nil, false, fmt.Errorf("parsing %s: %w", path, err)
	}
	issues, ok := LintAll(configs)
	return issues, ok, nil
}

// readFile is split out so tests can override it.
var readFile = func(path string) ([]byte, error) {
	return readWholeFile(path)
}
