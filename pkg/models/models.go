package models

import "fmt"

type ValidationResult struct {
	Key          string                 `json:"key"`
	Provider     string                 `json:"provider"`
	IsValid      bool                   `json:"is_valid"`
	InvalidReason string                 `json:"invalid_reason,omitempty"`
	StatusCode   int                    `json:"status_code"`

	StatusNote   string                 `json:"status_note,omitempty"`
	ResponseTime float64                `json:"response_time_sec"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	Balance      float64                `json:"balance,omitempty"`
	AccountName  string                 `json:"account_name,omitempty"`
	Email        string                 `json:"email,omitempty"`
	RetryAfter   int                    `json:"-"`
	ModelAccess  []string               `json:"model_access,omitempty"`
	Extra        map[string]interface{} `json:"extra,omitempty"`
}

func (r *ValidationResult) GetExtraString(key string) string {
	if r.Extra == nil {
		return ""
	}
	if val, ok := r.Extra[key]; ok {
		if s, ok := val.(string); ok {
			return s
		}
		if f, ok := val.(float64); ok {
			return fmt.Sprintf("%.2f", f)
		}
		if i, ok := val.(int); ok {
			return fmt.Sprintf("%d", i)
		}
		return fmt.Sprintf("%v", val)
	}
	return ""
}
