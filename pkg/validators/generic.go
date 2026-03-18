package validators

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/tidwall/gjson"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Grey-Magic/kunji/pkg/client"
	"github.com/Grey-Magic/kunji/pkg/models"
	"github.com/Grey-Magic/kunji/pkg/utils"
)

type GenericValidator struct {
	config    ProviderConfig
	client    *http.Client
	limiter   *client.RateLimiterManager
	bodyBytes []byte
}

func NewGenericValidatorWithClient(cfg ProviderConfig, httpClient *http.Client, limiter *client.RateLimiterManager) *GenericValidator {
	return &GenericValidator{
		config:    cfg,
		client:    httpClient,
		limiter:   limiter,
		bodyBytes: []byte(cfg.Validation.Body),
	}
}

func NewGenericValidator(cfg ProviderConfig, proxy string, timeout int) (*GenericValidator, error) {
	httpClient, err := client.NewHTTPClient(proxy, timeout)
	if err != nil {
		return nil, err
	}

	limiter := client.NewRateLimiterManager(10, 10)
	return &GenericValidator{
		config:    cfg,
		client:    httpClient,
		limiter:   limiter,
		bodyBytes: []byte(cfg.Validation.Body),
	}, nil
}

func (v *GenericValidator) Name() string          { return v.config.Name }
func (v *GenericValidator) KeyPatterns() []string { return v.config.KeyPatterns }

func (v *GenericValidator) Validate(ctx context.Context, apiKey string) (*models.ValidationResult, error) {
	if v.limiter != nil {
		if err := v.limiter.Wait(ctx, v.Name()); err != nil {
			return nil, err
		}
	}
	start := time.Now()
	cfg := v.config.Validation

	url := strings.ReplaceAll(cfg.URL, "{{key}}", apiKey)

	bodyStr := string(v.bodyBytes)
	bodyStr = strings.ReplaceAll(bodyStr, "{{key}}", apiKey)

	if strings.Contains(apiKey, ":") {
		parts := strings.SplitN(apiKey, ":", 2)
		url = strings.ReplaceAll(url, "{{key.client_id}}", parts[0])
		url = strings.ReplaceAll(url, "{{key.secret}}", parts[1])

		bodyStr = strings.ReplaceAll(bodyStr, "{{key.client_id}}", parts[0])
		bodyStr = strings.ReplaceAll(bodyStr, "{{key.secret}}", parts[1])
	}

	var bodyReader io.Reader
	if len(bodyStr) > 0 {
		bodyReader = strings.NewReader(bodyStr)
	}

	if err := utils.ValidateURL(url); err != nil {
		return nil, fmt.Errorf("security error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, cfg.Method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	v.applyAuth(req, cfg.Auth, apiKey)

	if len(v.bodyBytes) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, val := range cfg.Headers {
		req.Header.Set(k, val)
	}
	req.Header.Set("User-Agent", client.GetRandomUserAgent())

	resp, err := v.client.Do(req)
	duration := time.Since(start).Seconds()

	result := &models.ValidationResult{
		Key:          apiKey,
		Provider:     v.Name(),
		ResponseTime: duration,
	}

	if err != nil {
		result.IsValid = false
		result.ErrorMessage = fmt.Sprintf("Request failed: %v", err)
		return result, nil
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	result.RetryAfter = utils.ParseRetryAfter(resp.Header)
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		result.IsValid = false
		result.ErrorMessage = fmt.Sprintf("Failed to read response body: %v", err)
		return result, nil
	}

	switch {
	case resp.StatusCode == 200:
		result.IsValid = true
		v.extractValidationMetadata(bodyBytes, result)
		v.fetchChainMetadata(ctx, apiKey, resp, result)

	case resp.StatusCode == 402:
		result.IsValid = true
		result.StatusNote = "No Balance"
		result.ErrorMessage = utils.ParseAPIError(bodyBytes, apiKey)

	case resp.StatusCode == 429:
		result.IsValid = true
		result.StatusNote = "Rate Limited"
		result.ErrorMessage = utils.ParseAPIError(bodyBytes, apiKey)

	case resp.StatusCode >= 500:
		result.IsValid = true
		result.StatusNote = fmt.Sprintf("Server Error (%d)", resp.StatusCode)
		result.ErrorMessage = utils.ParseAPIError(bodyBytes, apiKey)

	case resp.StatusCode == 401 || resp.StatusCode == 403:
		result.IsValid = false
		result.ErrorMessage = utils.ParseAPIError(bodyBytes, apiKey)

	default:
		result.IsValid = false
		result.ErrorMessage = utils.ParseAPIError(bodyBytes, apiKey)
	}

	return result, nil
}

func (v *GenericValidator) applyAuth(req *http.Request, auth string, apiKey string) {
	if auth == "" || auth == "none" {
		return
	}
	if auth == "bearer" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
		return
	}
	if auth == "basic" {
		req.SetBasicAuth(apiKey, "")
		return
	}
	if auth == "basic_composite" {
		parts := strings.SplitN(apiKey, ":", 2)
		if len(parts) == 2 {
			req.SetBasicAuth(parts[0], parts[1])
		} else {
			req.SetBasicAuth(apiKey, "")
		}
		return
	}
	if strings.HasPrefix(auth, "header:") {
		headerName := strings.TrimPrefix(auth, "header:")
		req.Header.Set(headerName, apiKey)
		return
	}
	if strings.HasPrefix(auth, "query:") {
		paramName := strings.TrimPrefix(auth, "query:")
		q := req.URL.Query()
		q.Set(paramName, apiKey)
		req.URL.RawQuery = q.Encode()
	}
}

func (v *GenericValidator) extractValidationMetadata(bodyBytes []byte, result *models.ValidationResult) {
	mfv := v.config.MetadataFromValidation
	if mfv == nil {
		return
	}

	bodyStr := string(bodyBytes)

	if mfv.RegexExtract != "" {
		re, err := regexp.Compile(mfv.RegexExtract)
		if err == nil {
			matches := re.FindStringSubmatch(bodyStr)
			if len(matches) > mfv.RegexExtractMatch {
				if result.Extra == nil {
					result.Extra = make(map[string]interface{})
				}
				result.Extra["regex_extracted"] = matches[mfv.RegexExtractMatch]
			}
		}
	}

	if mfv.BalancePath != "" {
		bal := gjson.GetBytes(bodyBytes, mfv.BalancePath).Value()
		if mfv.BalanceSubtractPath != "" {
			sub := gjson.GetBytes(bodyBytes, mfv.BalanceSubtractPath).Value()
			result.Balance = toFloat64(bal) - toFloat64(sub)
		} else {
			result.Balance = toFloat64(bal)
		}
	}

	if mfv.QuotaPath != "" {
		if result.Extra == nil {
			result.Extra = make(map[string]interface{})
		}
		result.Extra["quota"] = gjson.GetBytes(bodyBytes, mfv.QuotaPath).Value()
	}

	if mfv.CreditsPath != "" {
		if result.Extra == nil {
			result.Extra = make(map[string]interface{})
		}
		result.Extra["credits"] = gjson.GetBytes(bodyBytes, mfv.CreditsPath).Value()
	}

	if mfv.VIPLevelPath != "" {
		if result.Extra == nil {
			result.Extra = make(map[string]interface{})
		}
		result.Extra["vip_level"] = gjson.GetBytes(bodyBytes, mfv.VIPLevelPath).Value()
	}

	if mfv.TeamNamePath != "" {
		if result.Extra == nil {
			result.Extra = make(map[string]interface{})
		}
		result.Extra["team_name"] = gjson.GetBytes(bodyBytes, mfv.TeamNamePath).Value()
	}

	if mfv.UsernamePath != "" {
		if result.Extra == nil {
			result.Extra = make(map[string]interface{})
		}
		result.Extra["username"] = gjson.GetBytes(bodyBytes, mfv.UsernamePath).Value()
	}

	if mfv.NamePath != "" {
		nameRes := gjson.GetBytes(bodyBytes, mfv.NamePath)
		if nameRes.Exists() && nameRes.String() != "" {
			result.AccountName = nameRes.String()
		} else if mfv.NameFallbackPath != "" {
			fallbackRes := gjson.GetBytes(bodyBytes, mfv.NameFallbackPath)
			if fallbackRes.Exists() {
				result.AccountName = fallbackRes.String()
			}
		}
	}

	if mfv.EmailPath != "" {
		emailRes := gjson.GetBytes(bodyBytes, mfv.EmailPath)
		if emailRes.Exists() {
			result.Email = emailRes.String()
		}
	}
}

func (v *GenericValidator) fetchChainMetadata(ctx context.Context, apiKey string, validationResp *http.Response, result *models.ValidationResult) {
	if len(v.config.Metadata) == 0 {
		return
	}

	variables := map[string]string{
		"key": apiKey,
	}

	if strings.Contains(apiKey, ":") {
		parts := strings.SplitN(apiKey, ":", 2)
		variables["key.client_id"] = parts[0]
		variables["key.secret"] = parts[1]
	}

	var mu sync.Mutex

	for _, step := range v.config.Metadata {

		url := step.URL
		for k, val := range variables {
			url = strings.ReplaceAll(url, "{{"+k+"}}", val)
		}

		if strings.Contains(url, "{{header.") {
			for _, part := range strings.Split(url, "{{header.") {
				if idx := strings.Index(part, "}}"); idx > 0 {
					headerName := part[:idx]
					headerVal := validationResp.Header.Get(headerName)
					if headerVal == "" {
						result.ErrorMessage = fmt.Sprintf("Metadata error: missing response header %s", headerName)
						return
					}
					url = strings.ReplaceAll(url, "{{header."+headerName+"}}", headerVal)
				}
			}
		}

		method := step.Method
		if method == "" {
			method = "GET"
		}

		metadataCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(metadataCtx, method, url, nil)
		if err != nil {
			result.ErrorMessage = fmt.Sprintf("Metadata error: %v", err)
			return
		}

		v.applyAuth(req, step.Auth, apiKey)
		for k, val := range step.Headers {
			req.Header.Set(k, val)
		}
		req.Header.Set("User-Agent", client.GetRandomUserAgent())

		resp, err := v.client.Do(req)
		if err != nil {
			result.ErrorMessage = fmt.Sprintf("Metadata fetch failed: %v", err)
			return
		}
		defer resp.Body.Close()

		respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
		if err != nil {
			result.ErrorMessage = fmt.Sprintf("Metadata error: failed to read response: %v", err)
			return
		}

		if resp.StatusCode != 200 {
			result.ErrorMessage = "Metadata error: " + utils.ParseAPIError(respBytes, apiKey)
			return
		}

		if step.RegexExtract != "" {
			re, err := regexp.Compile(step.RegexExtract)
			if err == nil {
				matches := re.FindStringSubmatch(string(respBytes))
				if len(matches) > step.RegexExtractMatch {
					if step.StoreAs != "" {
						variables[step.StoreAs] = matches[step.RegexExtractMatch]
					}
				}
			}
		}

		if step.Extract != "" && step.StoreAs != "" {
			val := gjson.GetBytes(respBytes, step.Extract)
			if val.Exists() {
				variables[step.StoreAs] = val.String()
			} else if step.RegexExtract == "" {
				result.ErrorMessage = fmt.Sprintf("Metadata error: path %s not found in response", step.Extract)
				return
			}
		}

		if step.BalancePath != "" {
			bal := gjson.GetBytes(respBytes, step.BalancePath)
			if bal.Exists() {
				mu.Lock()
				result.Balance = toFloat64(bal.Value())
				mu.Unlock()
			}
		}
	}
}

func toFloat64(val interface{}) float64 {
	if val == nil {
		return 0
	}
	switch v := val.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	case json.Number:
		f, _ := v.Float64()
		return f
	default:
		return 0
	}
}
