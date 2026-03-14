package utils

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func ValidateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %v", err)
	}

	host := u.Hostname()
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return fmt.Errorf("SSRF prevention: local hostnames are not allowed")
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		// If DNS lookup fails, we can't verify IP but the request will likely fail anyway.
		// For dynamic subdomains, this might happen.
		return nil
	}

	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsPrivate() {
			return fmt.Errorf("SSRF prevention: private/local IP addresses are not allowed (%s)", ip.String())
		}
	}

	return nil
}

type ValidationError struct {
	Code    string
	Message string
	Detail  string
}

func (e *ValidationError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("[%s] %s: %s", e.Code, e.Message, e.Detail)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

var (
	ErrInvalidKey    = &ValidationError{Code: "INVALID_KEY", Message: "Invalid API key"}
	ErrDisabled      = &ValidationError{Code: "DISABLED", Message: "API key is disabled"}
	ErrExpired       = &ValidationError{Code: "EXPIRED", Message: "API key has expired"}
	ErrRevoked       = &ValidationError{Code: "REVOKED", Message: "API key was revoked"}
	ErrBlocked       = &ValidationError{Code: "BLOCKED", Message: "API key is blocked"}
	ErrRateLimited   = &ValidationError{Code: "RATE_LIMITED", Message: "Rate limit exceeded"}
	ErrQuotaExceeded = &ValidationError{Code: "QUOTA_EXCEEDED", Message: "Quota exceeded"}
	ErrNotFound      = &ValidationError{Code: "NOT_FOUND", Message: "Resource not found"}
	ErrForbidden     = &ValidationError{Code: "FORBIDDEN", Message: "Access forbidden"}
	ErrUnauthorized  = &ValidationError{Code: "UNAUTHORIZED", Message: "Unauthorized - check API key"}
	ErrServerError   = &ValidationError{Code: "SERVER_ERROR", Message: "Server error"}
	ErrNetworkError  = &ValidationError{Code: "NETWORK_ERROR", Message: "Network error"}
	ErrTimeout       = &ValidationError{Code: "TIMEOUT", Message: "Request timed out"}
	ErrUnknown       = &ValidationError{Code: "UNKNOWN", Message: "Unknown error"}
)

func ParseErrorFromResponse(body []byte, statusCode int, apiKey string) *ValidationError {
	msg := ParseAPIError(body, apiKey)

	lowerMsg := strings.ToLower(msg)

	switch {
	case statusCode == 401 || strings.Contains(lowerMsg, "unauthorized") || strings.Contains(lowerMsg, "invalid"):
		return &ValidationError{Code: "INVALID_KEY", Message: "Invalid API key", Detail: msg}
	case statusCode == 403 || strings.Contains(lowerMsg, "forbidden") || strings.Contains(lowerMsg, "access denied"):
		return &ValidationError{Code: "FORBIDDEN", Message: "Access forbidden", Detail: msg}
	case strings.Contains(lowerMsg, "disabled"):
		return &ValidationError{Code: "DISABLED", Message: "API key is disabled", Detail: msg}
	case strings.Contains(lowerMsg, "expired"):
		return &ValidationError{Code: "EXPIRED", Message: "API key has expired", Detail: msg}
	case strings.Contains(lowerMsg, "revok"):
		return &ValidationError{Code: "REVOKED", Message: "API key was revoked", Detail: msg}
	case strings.Contains(lowerMsg, "block"):
		return &ValidationError{Code: "BLOCKED", Message: "API key is blocked", Detail: msg}
	case statusCode == 429 || strings.Contains(lowerMsg, "rate limit"):
		return &ValidationError{Code: "RATE_LIMITED", Message: "Rate limit exceeded", Detail: msg}
	case strings.Contains(lowerMsg, "quota") || strings.Contains(lowerMsg, "limit exceeded"):
		return &ValidationError{Code: "QUOTA_EXCEEDED", Message: "Quota exceeded", Detail: msg}
	case statusCode == 404 || strings.Contains(lowerMsg, "not found"):
		return &ValidationError{Code: "NOT_FOUND", Message: "Resource not found", Detail: msg}
	case statusCode >= 500:
		return &ValidationError{Code: "SERVER_ERROR", Message: "Server error", Detail: msg}
	default:
		return &ValidationError{Code: "UNKNOWN", Message: msg}
	}
}

func ParseAPIError(body []byte, apiKey string) string {
	if len(body) == 0 {
		return "Unknown error (empty body)"
	}

	var msg string
	var jsonErr map[string]interface{}
	if err := json.Unmarshal(body, &jsonErr); err == nil {

		if m, ok := jsonErr["error"].(string); ok {
			msg = enhanceErrorMessage(m, jsonErr)
		} else if errObj, ok := jsonErr["error"].(map[string]interface{}); ok {
			if m, ok := errObj["message"].(string); ok {
				msg = enhanceErrorMessage(m, jsonErr)
			} else if m, ok := errObj["type"].(string); ok {
				msg = enhanceErrorMessage(m, jsonErr)
			}
		} else if m, ok := jsonErr["message"].(string); ok {
			msg = enhanceErrorMessage(m, jsonErr)
		} else if m, ok := jsonErr["msg"].(string); ok {
			msg = enhanceErrorMessage(m, jsonErr)
		} else if code, ok := jsonErr["code"].(string); ok {
			msg = enhanceErrorMessage(code, jsonErr)
		} else if status, ok := jsonErr["status"].(string); ok {
			msg = enhanceErrorMessage(status, jsonErr)
		} else if detail, ok := jsonErr["detail"].(string); ok {
			msg = enhanceErrorMessage(detail, jsonErr)
		} else {
			strBytes, _ := json.Marshal(jsonErr)
			msg = string(strBytes)
		}
	} else {
		msg = strings.TrimSpace(string(body))
		msg = strings.ReplaceAll(msg, "\n", " ")
		msg = strings.ReplaceAll(msg, "\r", "")
	}

	if apiKey != "" {
		msg = strings.ReplaceAll(msg, apiKey, "[MASKED_KEY]")
	}

	return truncateStr(msg, 150)
}

func ScrubAPIKey(msg, apiKey string) string {
	if apiKey == "" {
		return msg
	}
	return strings.ReplaceAll(msg, apiKey, "[MASKED_KEY]")
}

func enhanceErrorMessage(msg string, jsonErr map[string]interface{}) string {
	msg = strings.TrimSpace(msg)

	if code, ok := jsonErr["code"].(float64); ok {
		msg = fmt.Sprintf("[%d] %s", int(code), msg)
	} else if codeStr, ok := jsonErr["code"].(string); ok {
		msg = fmt.Sprintf("[%s] %s", codeStr, msg)
	}

	if param, ok := jsonErr["param"].(string); ok {
		msg = msg + fmt.Sprintf(" (param: %s)", param)
	}

	msg = checkAndAppendReason(msg, jsonErr, "reason")
	msg = checkAndAppendReason(msg, jsonErr, "reason_message")
	msg = checkAndAppendReason(msg, jsonErr, "internal_reason")
	msg = checkAndAppendReason(msg, jsonErr, "error_reason")

	return msg
}

func checkAndAppendReason(msg string, jsonErr map[string]interface{}, key string) string {
	if reason, ok := jsonErr[key].(string); ok && reason != "" {
		return msg + " - " + reason
	}
	return msg
}

func truncateStr(str string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(str) > limit {
		return str[:limit] + "..."
	}
	return str
}

func ParseRetryAfter(respHeaders map[string][]string) int {

	if vals, ok := respHeaders["Retry-After"]; ok && len(vals) > 0 {
		sec, err := strconv.Atoi(vals[0])
		if err == nil {
			if sec > 86400 {
				ts := time.Now().Unix()
				if int64(sec) > ts {
					sec = int(int64(sec) - ts)
				} else {
					sec = 0
				}
			}
			if sec < 0 {
				sec = 0
			}
			return sec
		}
	}

	if vals, ok := respHeaders["X-Ratelimit-Reset"]; ok && len(vals) > 0 {
		ts, err := strconv.ParseInt(vals[0], 10, 64)
		if err == nil {
			now := time.Now().Unix()
			if ts > now {
				return int(ts - now)
			}
			if ts > 0 {
				return int(ts)
			}
		}
	}

	if vals, ok := respHeaders["X-RateLimit-Reset"]; ok && len(vals) > 0 {
		ts, err := strconv.ParseInt(vals[0], 10, 64)
		if err == nil {
			now := time.Now().Unix()
			if ts > now {
				return int(ts - now)
			}
			if ts > 0 {
				return int(ts)
			}
		}
	}

	return 0
}
