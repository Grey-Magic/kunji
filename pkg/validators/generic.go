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
	config       ProviderConfig
	client       *http.Client
	limiter      *client.RateLimiterManager
	bodyBytes    []byte
	skipMetadata bool
	checkCanary  bool
}

func NewGenericValidatorWithClient(cfg ProviderConfig, httpClient *http.Client, limiter *client.RateLimiterManager) *GenericValidator {
	return &GenericValidator{
		config:      cfg,
		client:      httpClient,
		limiter:     limiter,
		bodyBytes:   []byte(cfg.Validation.Body),
		checkCanary: true,
	}
}

func NewGenericValidator(cfg ProviderConfig, proxy string, timeout int) (*GenericValidator, error) {
	httpClient, _, err := client.NewHTTPClient(proxy, timeout)
	if err != nil {
		return nil, err
	}

	limiter := client.NewRateLimiterManager(10, 10)
	return &GenericValidator{
		config:      cfg,
		client:      httpClient,
		limiter:     limiter,
		bodyBytes:   []byte(cfg.Validation.Body),
		checkCanary: true,
	}, nil
}

func (v *GenericValidator) Name() string          { return v.config.Name }
func (v *GenericValidator) KeyPatterns() []string { return v.config.KeyPatterns }

func (v *GenericValidator) SetSkipMetadata(skip bool) {
	v.skipMetadata = skip
}

func (v *GenericValidator) SetCanaryCheck(check bool) {
	v.checkCanary = check
}

func (v *GenericValidator) isCanaryToken(key string) bool {
	if !v.checkCanary || len(v.config.CanaryPatterns) == 0 {
		return false
	}

	for _, p := range v.config.CanaryPatterns {
		re, err := regexp.Compile(p)
		if err != nil {
			continue
		}
		if re.MatchString(key) {
			return true
		}
	}
	return false
}

func (v *GenericValidator) performSyntaxCheck(key string) error {
	if v.config.SyntaxCheck == "" {
		return nil
	}

	switch v.config.SyntaxCheck {
	case "base64":
		parts := strings.Split(key, ":")
		toCheck := key
		if len(parts) == 2 {
			toCheck = parts[1]
		}
		
		toCheck = strings.TrimPrefix(toCheck, "sk_live_")
		toCheck = strings.TrimPrefix(toCheck, "sk_test_")
		
		_, err := utils.Base64Decode(toCheck)
		if err != nil {
			return fmt.Errorf("invalid syntax: not a valid base64 string")
		}
	}
	return nil
}

func (v *GenericValidator) categorizeInvalidKey(statusCode int, body []byte) string {
	bodyStr := strings.ToLower(string(body))
	
	switch {
	case strings.Contains(bodyStr, "expired"):
		return "Expired"
	case strings.Contains(bodyStr, "revoked") || strings.Contains(bodyStr, "deactivated") || strings.Contains(bodyStr, "disabled"):
		return "Revoked"
	case strings.Contains(bodyStr, "suspended") || strings.Contains(bodyStr, "blocked") || strings.Contains(bodyStr, "banned"):
		return "Suspended"
	case strings.Contains(bodyStr, "invalid") || strings.Contains(bodyStr, "malformed") || strings.Contains(bodyStr, "format"):
		return "Malformed"
	}

	if statusCode == 401 || statusCode == 403 {
		return "Invalid"
	}
	
	return ""
}

func (v *GenericValidator) Validate(ctx context.Context, apiKey string) (*models.ValidationResult, error) {
	if v.isCanaryToken(apiKey) {
		return &models.ValidationResult{
			Key:          apiKey,
			Provider:     v.Name(),
			IsValid:      false,
			ErrorMessage: "Canary/Honeypot token detected. Skipping network request for safety.",
			StatusNote:   "Canary Detected",
		}, nil
	}

	if err := v.performSyntaxCheck(apiKey); err != nil {
		return &models.ValidationResult{
			Key:          apiKey,
			Provider:     v.Name(),
			IsValid:      false,
			ErrorMessage: err.Error(),
			StatusNote:   "Offline Check Failed",
		}, nil
	}

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
		if v.limiter != nil {
			v.limiter.ReportResult(v.Name(), 500)
		}
		result.IsValid = false
		result.ErrorMessage = fmt.Sprintf("Request failed: %v", err)
		return result, nil
	}
	defer resp.Body.Close()

	if v.limiter != nil {
		v.limiter.ReportResult(v.Name(), resp.StatusCode)
	}

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
		if !v.skipMetadata {
			v.fetchChainMetadata(ctx, apiKey, resp, result)
		}

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
		result.InvalidReason = v.categorizeInvalidKey(resp.StatusCode, bodyBytes)
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

	variables := make(map[string]string)
	variables["key"] = apiKey
	if strings.Contains(apiKey, ":") {
		parts := strings.SplitN(apiKey, ":", 2)
		variables["key.client_id"] = parts[0]
		variables["key.secret"] = parts[1]
	}

	var varsMu sync.Mutex
	var resultMu sync.Mutex

	
	currentBatch := []MetadataConfig{}
	producedInPreviousBatches := make(map[string]bool)
	producedInPreviousBatches["key"] = true
	producedInPreviousBatches["key.client_id"] = true
	producedInPreviousBatches["key.secret"] = true

	for i, step := range v.config.Metadata {
		dependsOnNewVar := false
		for varName := range variables {
			if !producedInPreviousBatches[varName] && strings.Contains(step.URL, "{{"+varName+"}}") {
				dependsOnNewVar = true
				break
			}
		}

		if dependsOnNewVar && len(currentBatch) > 0 {
			v.runMetadataBatch(ctx, currentBatch, variables, &varsMu, &resultMu, validationResp, result)
			for k := range variables {
				producedInPreviousBatches[k] = true
			}
			currentBatch = []MetadataConfig{}
		}
		
		currentBatch = append(currentBatch, step)

		if i == len(v.config.Metadata)-1 {
			v.runMetadataBatch(ctx, currentBatch, variables, &varsMu, &resultMu, validationResp, result)
		}
	}
}

func (v *GenericValidator) runMetadataBatch(ctx context.Context, batch []MetadataConfig, variables map[string]string, varsMu *sync.Mutex, resultMu *sync.Mutex, validationResp *http.Response, result *models.ValidationResult) {
	var wg sync.WaitGroup
	for _, step := range batch {
		wg.Add(1)
		go func(s MetadataConfig) {
			defer wg.Done()
			
			varsMu.Lock()
			url := s.URL
			for k, val := range variables {
				url = strings.ReplaceAll(url, "{{"+k+"}}", val)
			}
			varsMu.Unlock()

			if strings.Contains(url, "{{header.") {
				for _, part := range strings.Split(url, "{{header.") {
					if idx := strings.Index(part, "}}"); idx > 0 {
						headerName := part[:idx]
						headerVal := validationResp.Header.Get(headerName)
						if headerVal != "" {
							url = strings.ReplaceAll(url, "{{header."+headerName+"}}", headerVal)
						}
					}
				}
			}

			method := s.Method
			if method == "" {
				method = "GET"
			}

			metadataCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			req, err := http.NewRequestWithContext(metadataCtx, method, url, nil)
			if err != nil {
				return
			}

			v.applyAuth(req, s.Auth, variables["key"])
			for k, val := range s.Headers {
				req.Header.Set(k, val)
			}
			req.Header.Set("User-Agent", client.GetRandomUserAgent())

			resp, err := v.client.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()

			respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
			if err != nil {
				return
			}

			if resp.StatusCode != 200 {
				return
			}

			if s.RegexExtract != "" {
				re, err := regexp.Compile(s.RegexExtract)
				if err == nil {
					matches := re.FindStringSubmatch(string(respBytes))
					if len(matches) > s.RegexExtractMatch {
						if s.StoreAs != "" {
							varsMu.Lock()
							variables[s.StoreAs] = matches[s.RegexExtractMatch]
							varsMu.Unlock()
						}
					}
				}
			}

			if s.Extract != "" && s.StoreAs != "" {
				val := gjson.GetBytes(respBytes, s.Extract)
				if val.Exists() {
					varsMu.Lock()
					variables[s.StoreAs] = val.String()
					varsMu.Unlock()
				}
			}

			if s.BalancePath != "" {
				bal := gjson.GetBytes(respBytes, s.BalancePath)
				if bal.Exists() {
					resultMu.Lock()
					result.Balance = toFloat64(bal.Value())
					resultMu.Unlock()
				}
			}
		}(step)
	}
	wg.Wait()
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
