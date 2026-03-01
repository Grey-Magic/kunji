package utils

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

func ParseAPIError(body []byte) string {
	if len(body) == 0 {
		return "Unknown error (empty body)"
	}

	var jsonErr map[string]interface{}
	if err := json.Unmarshal(body, &jsonErr); err == nil {

		if msg, ok := jsonErr["error"].(string); ok {
			return msg
		}

		if errObj, ok := jsonErr["error"].(map[string]interface{}); ok {
			if msg, ok := errObj["message"].(string); ok {
				return msg
			}
		}

		if msg, ok := jsonErr["message"].(string); ok {
			return msg
		}

		if msg, ok := jsonErr["msg"].(string); ok {
			return msg
		}

		strBytes, _ := json.Marshal(jsonErr)
		return truncateStr(string(strBytes), 150)
	}

	rawStr := strings.TrimSpace(string(body))

	rawStr = strings.ReplaceAll(rawStr, "\n", " ")
	rawStr = strings.ReplaceAll(rawStr, "\r", "")
	return truncateStr(rawStr, 150)
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
					sec = int(ts) - sec
				}
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
