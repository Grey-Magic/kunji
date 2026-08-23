package sink

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Grey-Magic/kunji/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- Discord ----------

func TestDiscordFormatter_Shape(t *testing.T) {
	f, err := buildFormatter(PlatformDiscord, FormatterOptions{})
	require.NoError(t, err)
	body, hdrs, err := f.Format(&models.ValidationResult{Provider: "openai", StatusCode: 200, IsValid: true})
	require.NoError(t, err)
	assert.Equal(t, "application/json", hdrs["Content-Type"])

	var p map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &p))
	assert.Contains(t, p["content"], "openai")
	embeds, ok := p["embeds"].([]interface{})
	require.True(t, ok, "discord payload must include embeds array")
	require.Len(t, embeds, 1)
	embed := embeds[0].(map[string]interface{})
	assert.EqualValues(t, 0x2ecc71, embed["color"], "valid = green")
}

func TestDiscordFormatter_InvalidUsesRedColor(t *testing.T) {
	f, err := buildFormatter(PlatformDiscord, FormatterOptions{})
	require.NoError(t, err)
	body, _, err := f.Format(&models.ValidationResult{Provider: "openai", IsValid: false})
	require.NoError(t, err)
	var p map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &p))
	embed := p["embeds"].([]interface{})[0].(map[string]interface{})
	assert.EqualValues(t, 0xe74c3c, embed["color"], "invalid = red")
}

// ---------- Telegram ----------

func TestTelegramFormatter_RequiresChatID(t *testing.T) {
	_, err := buildFormatter(PlatformTelegram, FormatterOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat_id")
}

func TestTelegramFormatter_Shape(t *testing.T) {
	f, err := buildFormatter(PlatformTelegram, FormatterOptions{TelegramChatID: "12345"})
	require.NoError(t, err)
	body, _, err := f.Format(&models.ValidationResult{Provider: "openai", IsValid: true})
	require.NoError(t, err)

	var p map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &p))
	assert.Equal(t, "12345", p["chat_id"])
	assert.Equal(t, "Markdown", p["parse_mode"])
	assert.Contains(t, p["text"], "openai")
}

// ---------- Slack / Teams / PagerDuty / Ntfy / Gotify / Pushover ----------

func TestSlackFormatter_Shape(t *testing.T) {
	f, _ := buildFormatter(PlatformSlack, FormatterOptions{})
	body, _, err := f.Format(&models.ValidationResult{Provider: "openai", IsValid: true})
	require.NoError(t, err)
	var p map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &p))
	assert.Contains(t, p["text"], "openai")
	atts := p["attachments"].([]interface{})
	require.Len(t, atts, 1)
}

func TestTeamsFormatter_MessageCard(t *testing.T) {
	f, _ := buildFormatter(PlatformTeams, FormatterOptions{})
	body, _, err := f.Format(&models.ValidationResult{Provider: "x", IsValid: true})
	require.NoError(t, err)
	var p map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &p))
	assert.Equal(t, "MessageCard", p["@type"])
	assert.NotEmpty(t, p["themeColor"])
}

func TestPagerDutyFormatter_RequiresRoutingKey(t *testing.T) {
	_, err := buildFormatter(PlatformPagerDuty, FormatterOptions{})
	require.Error(t, err)
}

func TestPagerDutyFormatter_TriggerOnInvalid(t *testing.T) {
	f, _ := buildFormatter(PlatformPagerDuty, FormatterOptions{PagerDutyRoutingKey: "rk"})
	body, _, err := f.Format(&models.ValidationResult{Provider: "openai", IsValid: false})
	require.NoError(t, err)
	var p map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &p))
	assert.Equal(t, "trigger", p["event_action"])
	pl := p["payload"].(map[string]interface{})
	assert.Equal(t, "kunji", pl["source"])
	assert.Equal(t, "error", pl["severity"])
}

func TestPagerDutyFormatter_AckOnValid(t *testing.T) {
	f, _ := buildFormatter(PlatformPagerDuty, FormatterOptions{PagerDutyRoutingKey: "rk"})
	body, _, err := f.Format(&models.ValidationResult{Provider: "openai", IsValid: true})
	require.NoError(t, err)
	var p map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &p))
	assert.Equal(t, "acknowledge", p["event_action"])
}

func TestNtfyFormatter_TopicHeader(t *testing.T) {
	f, _ := buildFormatter(PlatformNtfy, FormatterOptions{NtfyTopic: "alerts"})
	body, hdrs, err := f.Format(&models.ValidationResult{Provider: "openai", IsValid: true})
	require.NoError(t, err)
	assert.Equal(t, "alerts", hdrs["Topic"])
	assert.NotEmpty(t, body)
}

func TestGotifyFormatter_Shape(t *testing.T) {
	f, _ := buildFormatter(PlatformGotify, FormatterOptions{GotifyPriority: 7})
	body, _, err := f.Format(&models.ValidationResult{Provider: "openai", IsValid: false})
	require.NoError(t, err)
	var p map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &p))
	assert.EqualValues(t, 7, p["priority"])
}

func TestPushoverFormatter_RequiresTokens(t *testing.T) {
	_, err := buildFormatter(PlatformPushover, FormatterOptions{})
	require.Error(t, err)

	f, err := buildFormatter(PlatformPushover, FormatterOptions{PushoverUserKey: "u", PushoverAppToken: "t"})
	require.NoError(t, err)
	body, hdrs, err := f.Format(&models.ValidationResult{Provider: "openai", IsValid: false})
	require.NoError(t, err)
	assert.Equal(t, "application/x-www-form-urlencoded", hdrs["Content-Type"])
	assert.Contains(t, string(body), "token=t")
	assert.Contains(t, string(body), "user=u")
	assert.Contains(t, string(body), "priority=1")
}

// ---------- HTTPSink integration: custom headers, retries, signature ----------

func TestHTTPSink_CustomHeadersApplied(t *testing.T) {
	var gotAuth string
	var gotCustom string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCustom = r.Header.Get("X-Tenant")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s, err := NewHTTPSinkWithFormatter(srv.URL, FilterAll, PlatformRaw, FormatterOptions{}, HTTPSinkOptions{
		Headers: map[string]string{
			"Authorization": "Bearer mytoken",
			"X-Tenant":      "acme",
		},
		Timeout: 5 * time.Second,
	})
	require.NoError(t, err)
	require.NoError(t, s.Emit(context.Background(), &models.ValidationResult{Provider: "p", IsValid: true}))

	assert.Equal(t, "Bearer mytoken", gotAuth)
	assert.Equal(t, "acme", gotCustom)
}

func TestHTTPSink_RetriesOn5xx(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s, err := NewHTTPSinkWithFormatter(srv.URL, FilterAll, PlatformRaw, FormatterOptions{}, HTTPSinkOptions{
		Retries:         3,
		RetryBackoff:    10 * time.Millisecond,
		RetryMaxBackoff: 50 * time.Millisecond,
		Timeout:         2 * time.Second,
	})
	require.NoError(t, err)

	require.NoError(t, s.Emit(context.Background(), &models.ValidationResult{Provider: "p", IsValid: true}))
	assert.EqualValues(t, 3, hits)
}

func TestHTTPSink_GivesUpAfterMaxRetries(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	s, _ := NewHTTPSinkWithFormatter(srv.URL, FilterAll, PlatformRaw, FormatterOptions{}, HTTPSinkOptions{
		Retries:         2,
		RetryBackoff:    5 * time.Millisecond,
		RetryMaxBackoff: 20 * time.Millisecond,
		Timeout:         1 * time.Second,
	})

	err := s.Emit(context.Background(), &models.ValidationResult{Provider: "p", IsValid: true})
	require.Error(t, err)
	assert.EqualValues(t, 3, hits, "1 initial + 2 retries")
}

func TestHTTPSink_NoRetryOn4xx(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	s, _ := NewHTTPSinkWithFormatter(srv.URL, FilterAll, PlatformRaw, FormatterOptions{}, HTTPSinkOptions{
		Retries:         5,
		RetryBackoff:    5 * time.Millisecond,
		RetryMaxBackoff: 20 * time.Millisecond,
	})
	err := s.Emit(context.Background(), &models.ValidationResult{Provider: "p", IsValid: true})
	require.Error(t, err)
	assert.EqualValues(t, 1, hits, "4xx is not retried")
}

func TestHTTPSink_HMACSignatureHeader(t *testing.T) {
	var gotSig string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-Kunji-Signature")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s, _ := NewHTTPSinkWithFormatter(srv.URL, FilterAll, PlatformRaw, FormatterOptions{}, HTTPSinkOptions{
		SignatureSecret: "topsecret",
		SignaturePrefix: "sha256=",
	})
	require.NoError(t, s.Emit(context.Background(), &models.ValidationResult{Provider: "p", IsValid: true}))

	assert.True(t, strings.HasPrefix(gotSig, "sha256="), "signature header must carry the prefix")
	assert.Len(t, gotSig, len("sha256=")+64, "sha256 hex digest is 64 chars")
	assert.NotEmpty(t, gotBody)
}

func TestHTTPSink_DiscordEndToEnd(t *testing.T) {
	var gotContentType string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	s, err := NewHTTPSinkWithFormatter(srv.URL, FilterAll, PlatformDiscord, FormatterOptions{}, HTTPSinkOptions{Timeout: 5 * time.Second})
	require.NoError(t, err)
	require.NoError(t, s.Emit(context.Background(), &models.ValidationResult{Provider: "openai", StatusCode: 200, IsValid: true}))

	assert.Equal(t, "application/json", gotContentType)
	var p map[string]interface{}
	require.NoError(t, json.Unmarshal(gotBody, &p))
	assert.Contains(t, p["content"], "openai")
}

func TestHTTPSink_TelegramEndToEnd(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s, _ := NewHTTPSinkWithFormatter(srv.URL, FilterAll, PlatformTelegram, FormatterOptions{TelegramChatID: "999"}, HTTPSinkOptions{Timeout: 5 * time.Second})
	require.NoError(t, s.Emit(context.Background(), &models.ValidationResult{Provider: "openai", IsValid: true}))

	var p map[string]interface{}
	require.NoError(t, json.Unmarshal(gotBody, &p))
	assert.Equal(t, "999", p["chat_id"])
}

func TestParsePlatform(t *testing.T) {
	for _, name := range []string{"raw", "slack", "discord", "teams", "telegram", "pagerduty", "ntfy", "gotify", "pushover"} {
		p, err := ParsePlatform(name)
		require.NoError(t, err)
		assert.Equal(t, Platform(name), p)
	}
	_, err := ParsePlatform("nope")
	require.Error(t, err)
}

func TestResolveFormatterParams_PrefersParamsOverTyped(t *testing.T) {
	resolved := ResolveFormatterParams(FormatterOptions{
		TelegramChatID:      "from-typed",
		PagerDutyRoutingKey: "rk-typed",
		NtfyTopic:           "topic-typed",
		GotifyPriority:      9,
		PushoverUserKey:     "user-typed",
		PushoverAppToken:    "app-typed",
		Params: map[string]string{
			ParamChatID:   "from-param",
			ParamPriority: "5",
		},
	})
	assert.Equal(t, "from-param", resolved[ParamChatID], "Params must win over typed field")
	assert.Equal(t, "rk-typed", resolved[ParamRoutingKey], "typed-only fields surface in resolved map")
	assert.Equal(t, "topic-typed", resolved[ParamTopic])
	assert.Equal(t, "5", resolved[ParamPriority], "Param string overrides typed int when provided")
	assert.Equal(t, "user-typed", resolved[ParamUserKey])
	assert.Equal(t, "app-typed", resolved[ParamAppToken])
}

func TestResolveFormatterParams_EmptyWhenNoValues(t *testing.T) {
	resolved := ResolveFormatterParams(FormatterOptions{})
	_, hasChatID := resolved[ParamChatID]
	assert.False(t, hasChatID)
}

func TestFormatterFromParams_Discord(t *testing.T) {
	// Discord needs no extras; verify the unified params API still works.
	f, err := buildFormatter(PlatformDiscord, FormatterOptions{Params: map[string]string{"unused": "ignored"}})
	require.NoError(t, err)
	body, _, err := f.Format(&models.ValidationResult{Provider: "openai", IsValid: true})
	require.NoError(t, err)
	var p map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &p))
	assert.Contains(t, p["content"], "openai")
}

func TestFormatterFromParams_PushoverMissingUser(t *testing.T) {
	_, err := buildFormatter(PlatformPushover, FormatterOptions{Params: map[string]string{
		ParamAppToken: "t",
		// missing ParamUserKey
	}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user_key")
}

func TestFormatterFromParams_GotifyBadPriority(t *testing.T) {
	_, err := buildFormatter(PlatformGotify, FormatterOptions{Params: map[string]string{
		ParamPriority: "notanumber",
	}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be an integer")
}
