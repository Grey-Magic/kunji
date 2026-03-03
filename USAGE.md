# Kunji - Complete Usage Guide

Kunji is a CLI tool that validates API keys across 100+ services. This guide covers everything you need to know.

---

## Quick Start

```bash
# Single key validation
./kunji validate -k "sk-your-key-here"

# From file (one key per line)
./kunji validate -f keys.txt
```

---

## Basic Examples

### Single Key Validation

```bash
# Validate a single OpenAI key
./kunji validate -k "sk-proj-..."

# Validate Anthropic key
./kunji validate -k "sk-ant-api03-..."

# Validate Groq key
./kunji validate -k "gsk_..."
```

### Bulk Validation

```bash
# Validate all keys in a file
./kunji validate -f keys.txt

# With output file
./kunji validate -f keys.txt -o results.csv

# JSON output
./kunji validate -f keys.txt -o results.json
```

### Output Formats

```bash
# Text (default)
./kunji validate -f keys.txt -o results.txt

# CSV (great for Excel)
./kunji validate -f keys.txt -o results.csv

# JSON (for scripts)
./kunji validate -f keys.txt -o results.json
```

---

## Provider Detection

Kunji auto-detects providers using:
1. **Key Prefix** - First few characters (e.g., `sk-` = OpenAI, `sk-ant-` = Anthropic)
2. **Regex Pattern** - Full key format validation

### Auto-Detection Examples

```bash
# These keys will be auto-detected
./kunji validate -k "sk-..."          # OpenAI
./kunji validate -k "sk-ant-..."       # Anthropic
./kunji validate -k "gsk_..."          # Groq
./kunji validate -k "github_pat_..."   # GitHub
./kunji validate -k "sk_live_..."      # Stripe
```

### Composite Keys (ID:Secret)

Some providers require two values (client_id and secret). Use colon `:` separator:

```bash
# Format: client_id:secret

# Twilio (AccountSID:AuthToken)
./kunji validate -k "ACxxxxxxxx:auth_token"

# Algolia (AppID:AdminKey)
./kunji validate -k "APP_ID:admin_api_key"

# Freshdesk (Subdomain:APIKey)
./kunji validate -k "subdomain:api_key"

# Zendesk (Subdomain:Token)
./kunji validate -k "subdomain:token"
```

**From file (keys.txt):**
```
# One key per line
ACxxxxxxxx:auth_token
APP_ID:admin_api_key
sk-normal-key
subdomain:api_key
```

```bash
./kunji validate -f keys.txt
```

The code automatically splits by `:` and interpolates:
- `{{key.client_id}}` → first part
- `{{key.secret}}` → second part

### Force Specific Provider

Skip auto-detection and force a provider:

```bash
# Force Stripe validation (even if prefix doesn't match)
./kunji validate -k "rk_live_..." -p stripe

# Force GitHub
./kunji validate -k "ghp_..." -p github

# Force OpenAI
./kunji validate -k "anything" -p openai
```

**Important:** Some keys work for multiple services (e.g., Google API keys). Use `-p` to test specific service:

```bash
# Google Services (all use AIza... key format)
./kunji validate -k "AIza..." -p google_maps          # Maps
./kunji validate -k "AIza..." -p google_geocoding    # Geocoding
./kunji validate -k "AIza..." -p google_elevation    # Elevation
./kunji validate -k "AIza..." -p google_distance_matrix  # Distance Matrix
./kunji validate -k "AIza..." -p google_places       # Places
./kunji validate -k "AIza..." -p google_timezone      # Timezone
./kunji validate -k "AIza..." -p google_directions   # Directions
./kunji validate -k "AIza..." -p google_translate     # Translate
./kunji validate -k "AIza..." -p google_firebase      # Firebase
./kunji validate -k "AIza..." -p google_sheets        # Sheets
./kunji validate -k "AIza..." -p google_custom_search # Custom Search
./kunji validate -k "AIza..." -p google_pagespeed     # PageSpeed
./kunji validate -k "AIza..." -p google_geolocate      # Geolocation

# Test YouTube API key
./kunji validate -k "AIza..." -p youtube

# Test Gemini/AI API key
./kunji validate -k "AIza..." -p gemini
```

### Filter by Category

Limit detection to specific categories:

```bash
# Only LLM providers
./kunji validate -f keys.txt -c llm

# Only common services
./kunji validate -f keys.txt -c common

# Available categories: llm, common
```

---

## LLM Providers (Foundation)

### OpenAI
```bash
./kunji validate -k "sk-proj-..."
# Also: sk-... , sk1-..., sk2-...
```

### Anthropic (Claude)
```bash
./kunji validate -k "sk-ant-api03-..."
```

### Google Gemini
```bash
./kunji validate -k "AIza..."
```

### xAI (Grok)
```bash
./kunji validate -k "xai-..."
```

### Mistral
```bash
./kunji validate -k "..."
# Prefix: starts with letter, no specific prefix
```

---

## LLM Providers (Inference APIs)

### Groq
```bash
./kunji validate -k "gsk_..."
```

### Together AI
```bash
./kunji validate -k "..."
# Prefix: tock_ or general
```

### Fireworks AI
```bash
./kunji validate -k "fw_..."
```

### Novita AI
```bash
./kunji validate -k "..."
# Prefix: novita_
```

### Replicate
```bash
./kunji validate -k "r8_..."
```

---

## LLM Providers (Aggregators)

### OpenRouter
```bash
./kunji validate -k "sk-or-..."
```

### HuggingFace
```bash
./kunji validate -k "hf_..."
```

---

## LLM Providers (Regional)

### DeepSeek
```bash
./kunji validate -k "sk-..."
```

### Qwen (Alibaba)
```bash
./kunji validate -k "sk-..."
```

### GLM (Zhipu)
```bash
./kunji validate -k "..."
```

### Kimi (Moonshot)
```bash
./kunji validate -k "..."
```

### MiniMax
```bash
./kunji validate -k "..."
```

---

## LLM Providers (Tools)

### Cohere
```bash
./kunji validate -k "..."
# Prefix: cohere_
```

### Perplexity
```bash
./kunji validate -k "pplx-..."
```

### ElevenLabs
```bash
./kunji validate -k "..."
# Prefix: elabs_
```

### Venice AI
```bash
./kunji validate -k "venice_..."
```

### Midjourney
```bash
./kunji validate -k "..."
# Prefix: mj_
```

---

## Security & OSINT

### FOFA
```bash
./kunji validate -k "..."
```

### ZoomEye
```bash
./kunji validate -k "..."
```

### Netlas
```bash
./kunji validate -k "..."
```

### Intelligence X
```bash
./kunji validate -k "..."
```

### VirusTotal
```bash
./kunji validate -k "..."
```

### Censys
```bash
./kunji validate -k "..."
```

### Shodan
```bash
./kunji validate -k "..."
```

---

## Cloud & Hosting

### Cloudflare
```bash
./kunji validate -k "..."
```

### Heroku
```bash
./kunji validate -k "..."
```

### DigitalOcean
```bash
./kunji validate -k "..."
```

### JumpCloud
```bash
./kunji validate -k "..."
```

---

## Monetization

### Stripe
```bash
./kunji validate -k "sk_live_..."
# Also: sk_test_..., rk_live_...
```

### PayPal
```bash
./kunji validate -k "..."
```

### Square
```bash
./kunji validate -k "..."
```

---

## Developers

### GitHub
```bash
./kunji validate -k "ghp_..."
# Also: github_pat_...
```

### NPM
```bash
./kunji validate -k "..."
```

### Supabase
```bash
./kunji validate -k "eyJ..."
# JWT format
```

### CircleCI
```bash
./kunji validate -k "..."
```

---

## Productivity & Communication

### Slack
```bash
./kunji validate -k "xoxb-..."
# Also: xoxp-..., xoxa-...
```

### Discord
```bash
./kunji validate -k "..."
```

### Telegram
```bash
./kunji validate -k "..."
```

### Twilio
```bash
./kunji validate -k "..."
```

### Notion
```bash
./kunji validate -k "secret_..."
```

### SendGrid
```bash
./kunji validate -k "SG...."
```

### Asana
```bash
./kunji validate -k "..."
```

---

## Analytics & Operations

### DataDog
```bash
./kunji validate -k "..."
```

### PagerDuty
```bash
./kunji validate -k "..."
```

### OpsGenie
```bash
./kunji validate -k "..."
```

### WakaTime
```bash
./kunji validate -k "..."
```

### Mixpanel
```bash
./kunji validate -k "..."
```

### Amplitude
```bash
./kunji validate -k "..."
```

---

## Maps & Data

### Google Maps
```bash
./kunji validate -k "AIza..."
```

### Mapbox
```bash
./kunji validate -k "pk...."
```

### Ipstack
```bash
./kunji validate -k "..."
```

### Bing Maps
```bash
./kunji validate -k "..."
```

---

## Miscellaneous SaaS

### Dropbox
```bash
./kunji validate -k "..."
```

### Spotify
```bash
./kunji validate -k "..."
```

### HubSpot
```bash
./kunji validate -k "..."
```

### BrowserStack
```bash
./kunji validate -k "..."
```

### SauceLabs
```bash
./kunji validate -k "..."
```

### Lokalise
```bash
./kunji validate -k "..."
```

---

## Advanced Options

### Concurrency Control

```bash
# Increase threads for faster validation
./kunji validate -f keys.txt -t 50

# Decrease to reduce strain
./kunji validate -f keys.txt -t 5

# Default: 10
```

### Timeout

```bash
# Longer timeout for slow providers (seconds)
./kunji validate -f keys.txt --timeout 30

# Default: 15 seconds
```

### Retries

```bash
# Retry failed requests
./kunji validate -f keys.txt -r 5

# Default: 3 retries
```

### Resume Previous Run

```bash
# Skip keys already validated
./kunji validate -f keys.txt --resume -o results.csv
```

---

## Proxy Support

### Single Proxy

```bash
./kunji validate -f keys.txt --proxy "http://user:pass@host:port"
```

### Proxy List (Rotation)

```bash
# Rotates through proxies.txt
./kunji validate -f keys.txt --proxy proxies.txt
```

Format (proxies.txt):
```
http://user:pass@host1:port
http://user:pass@host2:port
socks5://user:pass@host:port
```

---

## View All Supported Providers

```bash
# List all 103 supported providers
./kunji validate --list
```

---

## Complete Flag Reference

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--key` | `-k` | - | Single API key |
| `--keys` | `-f` | - | File with keys (one per line) |
| `--out` | `-o` | results.txt | Output file (.txt, .csv, .json) |
| `--provider` | `-p` | auto | Force specific provider |
| `--category` | `-c` | all | Filter by category (llm, common) |
| `--threads` | `-t` | 10 | Concurrent workers |
| `--timeout` | - | 15 | Request timeout (seconds) |
| `--retries` | `-r` | 3 | Retry count |
| `--proxy` | - | - | Proxy URL or file |
| `--resume` | - | false | Skip existing results |
| `--list` | `-l` | - | List all providers |

---

## Output Examples

### Valid Key Output
```
✅ OpenAI
   Key: sk-proj-****abc123
   Balance: $12.50
   Email: user@example.com
   Models: gpt-4, gpt-3.5-turbo
```

### Invalid Key Output
```
❌ OpenAI
   Key: sk-proj-****xyz789
   Status: Invalid
```

### CSV Output
```csv
Key,Provider,Valid,Balance,Email,Message
sk-proj-****,openai,true,$12.50,user@example.com,OK
sk-test-****,openai,false,,,Invalid API key
```

### JSON Output
```json
[
  {
    "key": "sk-proj-****abc123",
    "provider": "openai",
    "valid": true,
    "balance": 12.50,
    "email": "user@example.com"
  }
]
```

---

## Tips

1. **Use CSV for large batches** - Easier to analyze in Excel/Google Sheets
2. **Increase threads** for faster validation of many keys
3. **Use proxy rotation** to avoid rate limits
4. **Use --resume** to continue interrupted validations
5. **Force provider** when auto-detection fails

---

## Troubleshooting

### "Provider not detected"
- Use `-p <provider>` to force

### "Too many requests" / Rate Limited
- Use proxy rotation: `--proxy proxies.txt`
- Reduce threads: `-t 5`
- Increase timeout: `--timeout 30`

### "Connection timeout"
- Increase timeout: `--timeout 60`
- Check network/proxy

### "Invalid key" but key works
- Provider may have changed API
- Try forcing provider: `-p <provider>`
