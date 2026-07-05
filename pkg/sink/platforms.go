package sink

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/Grey-Magic/kunji/pkg/models"
)

// Platform identifies a built-in webhook payload formatter. The generic
// "raw" platform posts the ValidationResult unchanged. Other platforms
// translate the result into the JSON body shape their endpoint expects.
type Platform string

const (
	PlatformRaw       Platform = "raw"       // raw ValidationResult JSON
	PlatformSlack     Platform = "slack"     // Slack-compatible {text, attachments}
	PlatformDiscord   Platform = "discord"   // Discord-compatible {content, embeds}
	PlatformTeams     Platform = "teams"     // Microsoft Teams MessageCard
	PlatformTelegram  Platform = "telegram"  // Telegram Bot API sendMessage
	PlatformPagerDuty Platform = "pagerduty" // PagerDuty Events API v2
	PlatformNtfy      Platform = "ntfy"      // ntfy.sh plain body + Topic header
	PlatformGotify    Platform = "gotify"    // Gotify {message, title}
	PlatformPushover  Platform = "pushover"  // Pushover form-encoded body
)

// allPlatforms returns the supported platform identifiers for help/error text.
func allPlatforms() []Platform {
	return []Platform{
		PlatformRaw, PlatformSlack, PlatformDiscord, PlatformTeams,
		PlatformTelegram, PlatformPagerDuty, PlatformNtfy, PlatformGotify, PlatformPushover,
	}
}

// ParsePlatform validates a string against the supported platforms.
func ParsePlatform(s string) (Platform, error) {
	if s == "" {
		return PlatformRaw, nil
	}
	p := Platform(strings.ToLower(strings.TrimSpace(s)))
	for _, candidate := range allPlatforms() {
		if p == candidate {
			return p, nil
		}
	}
	return "", fmt.Errorf("unknown webhook platform %q (supported: %s)",
		s, strings.Join(asStringSlice(allPlatforms()), ", "))
}

func asStringSlice(ps []Platform) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = string(p)
	}
	return out
}

// Formatter produces the HTTP body and any platform-specific headers for one
// validation result. It returns the body bytes and any extra headers that
// should be merged with the user-supplied ones (Content-Type is added by the
// HTTPSink if not set by the formatter).
type Formatter interface {
	Platform() Platform
	Format(r *models.ValidationResult) (body []byte, headers map[string]string, err error)
}

// buildFormatter returns the formatter for the given platform. Extra options:
// telegram_chat_id is required for telegram; pagerduty_routing_key for
// pagerduty. Unknown extras are ignored — they are surfaced by the caller.
func buildFormatter(p Platform, opts FormatterOptions) (Formatter, error) {
	resolved := ResolveFormatterParams(opts)
	switch p {
	case PlatformRaw:
		return &rawFormatter{}, nil
	case PlatformSlack:
		return &slackFormatter{}, nil
	case PlatformDiscord:
		return &discordFormatter{}, nil
	case PlatformTeams:
		return &teamsFormatter{}, nil
	case PlatformTelegram:
		chatID := paramString(resolved, ParamChatID, "")
		if chatID == "" {
			return nil, fmt.Errorf("telegram platform requires --webhook-param chat_id=<id>")
		}
		return &telegramFormatter{chatID: chatID}, nil
	case PlatformPagerDuty:
		rk := paramString(resolved, ParamRoutingKey, "")
		if rk == "" {
			return nil, fmt.Errorf("pagerduty platform requires --webhook-param routing_key=<key>")
		}
		return &pagerDutyFormatter{routingKey: rk}, nil
	case PlatformNtfy:
		return &ntfyFormatter{topic: paramString(resolved, ParamTopic, "")}, nil
	case PlatformGotify:
		// priority falls back to the typed int when --webhook-param priority is absent.
		prio := opts.GotifyPriority
		if v, ok := resolved[ParamPriority]; ok && v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("gotify priority must be an integer (got %q)", v)
			}
			prio = n
		}
		return &gotifyFormatter{priority: prio}, nil
	case PlatformPushover:
		user := paramString(resolved, ParamUserKey, "")
		app := paramString(resolved, ParamAppToken, "")
		if user == "" || app == "" {
			return nil, fmt.Errorf("pushover platform requires --webhook-param user_key=<key> --webhook-param app_token=<token>")
		}
		return &pushoverFormatter{userKey: user, appToken: app}, nil
	}
	return nil, fmt.Errorf("no formatter for platform %q", p)
}

// FormatterOptions carries platform-specific configuration. All fields are
// optional except where the formatter requires them.
//
// Typed fields (TelegramChatID, PagerDutyRoutingKey, ...) are kept for
// backward compatibility with code that constructed FormatterOptions
// directly. New code should populate Params via the CLI helper
// --webhook-param key=value. The two views are merged by
// ResolveFormatterParams.
type FormatterOptions struct {
	TelegramChatID      string
	PagerDutyRoutingKey string
	NtfyTopic           string
	GotifyPriority      int
	PushoverUserKey     string
	PushoverAppToken    string
	// Params is a free-form key/value map. Both the typed fields above and
	// any well-known keys here are merged into one resolved map by
	// ResolveFormatterParams.
	Params map[string]string
}

// Well-known param keys accepted via --webhook-param key=value.
const (
	ParamChatID     = "chat_id"
	ParamRoutingKey = "routing_key"
	ParamTopic      = "topic"
	ParamPriority   = "priority"
	ParamUserKey    = "user_key"
	ParamAppToken   = "app_token"
)

// ResolveFormatterParams returns a flat key->string map covering every
// platform-specific configuration source: typed FormatterOptions fields plus
// the user-supplied Params. The Param* constants document the recognized
// keys. Values are stringified so callers don't need to care about types.
func ResolveFormatterParams(o FormatterOptions) map[string]string {
	out := make(map[string]string, len(o.Params)+6)
	for k, v := range o.Params {
		out[k] = v
	}
	if v := o.TelegramChatID; v != "" {
		if _, exists := out[ParamChatID]; !exists {
			out[ParamChatID] = v
		}
	}
	if v := o.PagerDutyRoutingKey; v != "" {
		if _, exists := out[ParamRoutingKey]; !exists {
			out[ParamRoutingKey] = v
		}
	}
	if v := o.NtfyTopic; v != "" {
		if _, exists := out[ParamTopic]; !exists {
			out[ParamTopic] = v
		}
	}
	if v := o.PushoverUserKey; v != "" {
		if _, exists := out[ParamUserKey]; !exists {
			out[ParamUserKey] = v
		}
	}
	if v := o.PushoverAppToken; v != "" {
		if _, exists := out[ParamAppToken]; !exists {
			out[ParamAppToken] = v
		}
	}
	// GotifyPriority is special: it's an int, but we stringified it for callers
	// that only want to look at strings. Honor the typed field only if the
	// user didn't provide --webhook-param priority=<n>.
	if _, exists := out[ParamPriority]; !exists && o.GotifyPriority > 0 {
		out[ParamPriority] = ""
		// (Empty string here signals "use typed field"; the formatter reads
		// FormatterOptions.GotifyPriority directly when this key is empty.)
	}
	return out
}

// paramString reads a string param from a resolved map with fallback.
func paramString(m map[string]string, key, fallback string) string {
	if v, ok := m[key]; ok && v != "" {
		return v
	}
	return fallback
}

// ---------- raw ----------

type rawFormatter struct{}

func (f *rawFormatter) Platform() Platform { return PlatformRaw }
func (f *rawFormatter) Format(r *models.ValidationResult) ([]byte, map[string]string, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return nil, nil, err
	}
	return b, map[string]string{"Content-Type": "application/json"}, nil
}

// ---------- Slack ----------

type slackFormatter struct{}

func (f *slackFormatter) Platform() Platform { return PlatformSlack }

// Format produces a Slack-compatible payload using legacy "attachments".
// Slack accepts arbitrary JSON with a top-level "text" or blocks; we keep
// this minimal so it works on every workspace.
func (f *slackFormatter) Format(r *models.ValidationResult) ([]byte, map[string]string, error) {
	status := "✅ Valid"
	color := "good"
	if !r.IsValid {
		status = "❌ Invalid"
		color = "danger"
		if r.StatusCode == 429 {
			status = "⚠ Rate Limited"
			color = "warning"
		}
	}
	payload := map[string]interface{}{
		"text": fmt.Sprintf("*kunji*: %s key for *%s*", status, r.Provider),
		"attachments": []map[string]interface{}{
			{
				"color": color,
				"fields": []map[string]interface{}{
					{"title": "Provider", "value": r.Provider, "short": true},
					{"title": "Status", "value": status, "short": true},
					{"title": "HTTP", "value": fmt.Sprintf("%d", r.StatusCode), "short": true},
					{"title": "Latency", "value": fmt.Sprintf("%.2fs", r.ResponseTime), "short": true},
				},
			},
		},
	}
	if r.ErrorMessage != "" {
		payload["attachments"].([]map[string]interface{})[0]["text"] = r.ErrorMessage
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	return b, map[string]string{"Content-Type": "application/json"}, nil
}

// ---------- Discord ----------

type discordFormatter struct{}

func (f *discordFormatter) Platform() Platform { return PlatformDiscord }

func (f *discordFormatter) Format(r *models.ValidationResult) ([]byte, map[string]string, error) {
	status := "✅ Valid"
	color := 0x2ecc71 // green
	if !r.IsValid {
		status = "❌ Invalid"
		color = 0xe74c3c // red
		if r.StatusCode == 429 {
			status = "⚠ Rate Limited"
			color = 0xf39c12 // yellow
		}
	}
	payload := map[string]interface{}{
		"content": fmt.Sprintf("**kunji**: %s key for **%s**", status, r.Provider),
		"embeds": []map[string]interface{}{
			{
				"color": color,
				"fields": []map[string]interface{}{
					{"name": "Provider", "value": r.Provider, "inline": true},
					{"name": "Status", "value": status, "inline": true},
					{"name": "HTTP", "value": fmt.Sprintf("%d", r.StatusCode), "inline": true},
				},
			},
		},
	}
	if r.ErrorMessage != "" {
		payload["embeds"].([]map[string]interface{})[0]["description"] = r.ErrorMessage
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	return b, map[string]string{"Content-Type": "application/json"}, nil
}

// ---------- Microsoft Teams (MessageCard) ----------

type teamsFormatter struct{}

func (f *teamsFormatter) Platform() Platform { return PlatformTeams }

func (f *teamsFormatter) Format(r *models.ValidationResult) ([]byte, map[string]string, error) {
	status := "Valid"
	themeColor := "00FF00"
	if !r.IsValid {
		status = "Invalid"
		themeColor = "FF0000"
		if r.StatusCode == 429 {
			status = "Rate Limited"
			themeColor = "FFAA00"
		}
	}
	payload := map[string]interface{}{
		"@type":      "MessageCard",
		"@context":   "https://schema.org/extensions",
		"themeColor": themeColor,
		"summary":    fmt.Sprintf("kunji: %s for %s", status, r.Provider),
		"title":      fmt.Sprintf("kunji — %s", status),
		"sections": []map[string]interface{}{
			{
				"facts": []map[string]interface{}{
					{"name": "Provider", "value": r.Provider},
					{"name": "HTTP", "value": fmt.Sprintf("%d", r.StatusCode)},
					{"name": "Latency", "value": fmt.Sprintf("%.2fs", r.ResponseTime)},
				},
				"text": r.ErrorMessage,
			},
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	return b, map[string]string{"Content-Type": "application/json"}, nil
}

// ---------- Telegram Bot API ----------

type telegramFormatter struct{ chatID string }

func (f *telegramFormatter) Platform() Platform { return PlatformTelegram }

func (f *telegramFormatter) Format(r *models.ValidationResult) ([]byte, map[string]string, error) {
	status := "✅ Valid"
	if !r.IsValid {
		status = "❌ Invalid"
	}
	text := fmt.Sprintf("*kunji*: %s\n*Provider:* `%s`\n*HTTP:* `%d`",
		status, r.Provider, r.StatusCode)
	payload := map[string]interface{}{
		"chat_id":    f.chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}
	if r.ErrorMessage != "" {
		payload["text"] = text + "\n*Error:* `" + r.ErrorMessage + "`"
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	return b, map[string]string{"Content-Type": "application/json"}, nil
}

// ---------- PagerDuty Events API v2 ----------

type pagerDutyFormatter struct{ routingKey string }

func (f *pagerDutyFormatter) Platform() Platform { return PlatformPagerDuty }

func (f *pagerDutyFormatter) Format(r *models.ValidationResult) ([]byte, map[string]string, error) {
	// PagerDuty Events API v2: trigger only on invalid results. We always
	// emit, but use trigger/acknowledge based on validity so operators see
	// every state change in the incident timeline.
	action := "trigger"
	if r.IsValid {
		action = "acknowledge"
	}
	payload := map[string]interface{}{
		"routing_key":  f.routingKey,
		"event_action": action,
		"dedup_key":    "kunji:" + r.Provider + ":" + shortHash(r.Key),
		"payload": map[string]interface{}{
			"summary":   fmt.Sprintf("kunji: %s key for %s", stateForPD(r), r.Provider),
			"source":    "kunji",
			"severity":  severityFor(r),
			"component": r.Provider,
			"custom_details": map[string]interface{}{
				"http_status":   r.StatusCode,
				"response_time": r.ResponseTime,
				"error":         r.ErrorMessage,
			},
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	return b, map[string]string{"Content-Type": "application/json"}, nil
}

func stateForPD(r *models.ValidationResult) string {
	if r.IsValid {
		return "valid"
	}
	if r.StatusCode == 429 {
		return "rate-limited"
	}
	return "invalid"
}

func severityFor(r *models.ValidationResult) string {
	if r.IsValid {
		return "info"
	}
	if r.StatusCode == 429 {
		return "warning"
	}
	return "error"
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:6])
}

// ---------- ntfy ----------

type ntfyFormatter struct{ topic string }

func (f *ntfyFormatter) Platform() Platform { return PlatformNtfy }

func (f *ntfyFormatter) Format(r *models.ValidationResult) ([]byte, map[string]string, error) {
	status := "✅ Valid"
	priority := "default"
	if !r.IsValid {
		status = "❌ Invalid"
		priority = "high"
		if r.StatusCode == 429 {
			status = "⚠ Rate Limited"
			priority = "default"
		}
	}
	body := fmt.Sprintf("kunji: %s key for %s (HTTP %d)\n%s",
		status, r.Provider, r.StatusCode, r.ErrorMessage)
	headers := map[string]string{
		"Title":    fmt.Sprintf("kunji — %s", r.Provider),
		"Priority": priority,
		"Tags":     "key,validation",
	}
	if f.topic != "" {
		headers["Topic"] = f.topic
	}
	return []byte(body), headers, nil
}

// ---------- Gotify ----------

type gotifyFormatter struct{ priority int }

func (f *gotifyFormatter) Platform() Platform { return PlatformGotify }

func (f *gotifyFormatter) Format(r *models.ValidationResult) ([]byte, map[string]string, error) {
	status := "Valid"
	prio := 5 // default normal
	if !r.IsValid {
		status = "Invalid"
		prio = 8 // high
		if r.StatusCode == 429 {
			status = "Rate Limited"
			prio = 5
		}
	}
	if f.priority > 0 {
		prio = f.priority
	}
	payload := map[string]interface{}{
		"message":  fmt.Sprintf("%s key for %s: HTTP %d", status, r.Provider, r.StatusCode),
		"title":    fmt.Sprintf("kunji — %s", r.Provider),
		"priority": prio,
	}
	if r.ErrorMessage != "" {
		payload["message"] = fmt.Sprintf("%s key for %s: HTTP %d — %s", status, r.Provider, r.StatusCode, r.ErrorMessage)
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	return b, map[string]string{"Content-Type": "application/json"}, nil
}

// ---------- Pushover (form-encoded) ----------

type pushoverFormatter struct {
	userKey  string
	appToken string
}

func (f *pushoverFormatter) Platform() Platform { return PlatformPushover }

func (f *pushoverFormatter) Format(r *models.ValidationResult) ([]byte, map[string]string, error) {
	status := "✅ Valid"
	if !r.IsValid {
		status = "❌ Invalid"
	}
	form := url.Values{}
	form.Set("token", f.appToken)
	form.Set("user", f.userKey)
	form.Set("title", fmt.Sprintf("kunji — %s", r.Provider))
	form.Set("message", fmt.Sprintf("%s key (HTTP %d)\n%s", status, r.StatusCode, r.ErrorMessage))
	form.Set("priority", "0")
	if !r.IsValid {
		form.Set("priority", "1")
	}
	return []byte(form.Encode()), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	}, nil
}

// ---------- shared helpers ----------

// SignHMAC adds an HMAC-SHA256 signature header to the body. Used for webhooks
// that verify request authenticity (e.g. custom intake endpoints). The
// signature is hex-encoded and prefixed with the given algorithm label.
func SignHMAC(body []byte, secret, headerName, prefix string) (string, string) {
	if secret == "" {
		return "", ""
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))
	header := sig
	if prefix != "" {
		header = prefix + sig
	}
	return headerName, header
}

// bodyAndHeaders merges formatter-supplied headers, user headers, and the
// signature header into one map. User headers win on conflict so operators
// can override platform defaults.
func mergeHeaders(formatterHeaders, userHeaders map[string]string, sigName, sigValue string) map[string]string {
	out := make(map[string]string, len(formatterHeaders)+len(userHeaders)+1)
	for k, v := range formatterHeaders {
		out[k] = v
	}
	for k, v := range userHeaders {
		out[k] = v
	}
	if sigName != "" {
		out[sigName] = sigValue
	}
	return out
}

// helper to keep imports clean when bytes is otherwise unused
var _ = bytes.NewReader(nil)
