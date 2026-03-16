# Kunji - Complete Usage Guide

Kunji is a CLI tool that validates API keys across 258 services. This guide covers everything you need to know.

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
./kunji validate -k "sk_test_..."      # Stripe (test)
./kunji validate -k "vercel_..."       # Vercel
./kunji validate -k "dop_v1_..."        # DigitalOcean
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

## LLM Providers (24 Providers)

### OpenAI
```bash
./kunji validate -k "sk-proj-..."
./kunji validate -k "sk-svcacct-..."
./kunji validate -k "sk-ant-..."
./kunji validate -k "sk-..."  # Legacy format
```

### Anthropic (Claude)
```bash
./kunji validate -k "sk-ant-api03-..."
./kunji validate -k "anthropic-..."
```

### Google Gemini
```bash
./kunji validate -k "AIza..."
```

### xAI (Grok)
```bash
./kunji validate -k "xai-..."
```

### DeepSeek
```bash
./kunji validate -k "sk-deepseek-..."
```

### Mistral
```bash
./kunji validate -k "mistral-..."
```

### Cohere
```bash
./kunji validate -k "cohere-..."
```

### Perplexity
```bash
./kunji validate -k "pplx-..."
```

### Groq
```bash
./kunji validate -k "gsk_..."
```

### Together AI
```bash
./kunji validate -k "together-..."
```

### Fireworks AI
```bash
./kunji validate -k "fw_..."
```

### HuggingFace
```bash
./kunji validate -k "hf_..."
```

### OpenRouter
```bash
./kunji validate -k "sk-or-..."
```

### Novita AI
```bash
./kunji validate -k "novita-..."
```

### Replicate
```bash
./kunji validate -k "r8_..."
```

### ElevenLabs
```bash
./kunji validate -k "sk_..."  # Voice AI
```

### Midjourney
```bash
./kunji validate -k "mj-..."
./kunji validate -k "goapi-..."
./kunji validate -k "piapi-..."
```

### Venice AI
```bash
./kunji validate -k "venice-..."
```

### Qwen (Alibaba)
```bash
./kunji validate -k "sk-qwen-..."
./kunji validate -k "sk-dashscope-..."
```

### Kimi (Moonshot)
```bash
./kunji validate -k "sk-kimi-..."
./kunji validate -k "sk-moonshot-..."
```

### MiniMax
```bash
./kunji validate -k "minimax-..."
```

### GLM (Zhipu)
```bash
./kunji validate -k "..."  # No prefix, check balance
```

### Cline
```bash
./kunji validate -k "cline-..."
```

### Aider
```bash
./kunji validate -k "aider-..."
```

### Kilo
```bash
./kunji validate -k "kilo-..."
```

---

## Cloud & Infrastructure (New)

### Vercel
```bash
./kunji validate -k "vercel_..."
```

### Netlify
```bash
./kunji validate -k "nfp_..."
./kunji validate -k "nf_..."
```

### Railway
```bash
./kunji validate -k "railway_..."
```

### Fly.io
```bash
./kunji validate -k "fly_..."
```

### Upstash (Redis/Kafka)
```bash
./kunji validate -k "upstash_..."
```

### PlanetScale (MySQL)
```bash
./kunji validate -k "pscale_..."
```

### Neon (PostgreSQL)
```bash
./kunji validate -k "neon_..."
```

### Turso (SQLite)
```bash
./kunji validate -k "turso_..."
```

### DigitalOcean
```bash
./kunji validate -k "dop_v1_..."
```

### Cloudflare
```bash
./kunji validate -k "cloudflare_..."
```

---

## AI/ML Services (New)

### Pinecone (Vector DB)
```bash
./kunji validate -k "pc-..."
./kunji validate -k "pinecone_..."
```

### Weaviate
```bash
./kunji validate -k "weaviate-..."
```

### LangSmith
```bash
./kunji validate -k "ls-..."
```

### Langfuse
```bash
./kunji validate -k "lf-..."
```

### Weights & Biases
```bash
./kunji validate -k "wandb_..."
```

---

## Security & Auth (New)

### Auth0
```bash
./kunji validate -k "auth0_..."
```

### Clerk
```bash
./kunji validate -k "clerk_..."
```

### Doppler
```bash
./kunji validate -k "dp-..."
```

---

## Communication (New)

### Discord Bot
```bash
./kunji validate -k "Bot ..."  # Bot token
```

### Resend
```bash
./kunji validate -k "re_..."
```

### Postmark
```bash
./kunji validate -k "PM-..."
```

### Loops
```bash
./kunji validate -k "loop-..."
```

### Novu
```bash
./kunji validate -k "novu_..."
```

### Stream
```bash
./kunji validate -k "stream-..."
```

---

## Payments (Updated)

### Stripe
```bash
./kunji validate -k "sk_live_..."   # Live
./kunji validate -k "sk_test_..."   # Test (NEW)
```

### PayPal
```bash
./kunji validate -k "..."  # Composite key format
```

### Square
```bash
./kunji validate -k "..."  # Composite key format
```

### LemonSqueezy
```bash
./kunji validate -k "api_key_..."
```

### Paddle
```bash
./kunji validate -k "paddle_..."
```

### Plaid
```bash
./kunji validate -k "plaid-..."
```

### Chargebee
```bash
./kunji validate -k "chargebee_..."
```

---

## DevOps (New)

### GitLab CI
```bash
./kunji validate -k "glcbt-..."
```

### Pulumi
```bash
./kunji validate -k "pul-..."
```

### Terraform Cloud
```bash
./kunji validate -k "tfe-..."
```

### Spacelift
```bash
./kunji validate -k "spacelift_..."
```

---

## Monitoring (Updated)

### Sentry
```bash
./kunji validate -k "sntrys_..."
```

### PostHog
```bash
./kunji validate -k "phc_..."
```

### Highlight.io
```bash
./kunji validate -k "highlight_..."
```

### Better Stack
```bash
./kunji validate -k "better-stack_..."
```

### Checkly
```bash
./kunji validate -k "checkly_..."
```

### DataDog
```bash
./kunji validate -k "DD_API_..."
./kunji validate -k "DD_APP_..."
```

---

## Database (New)

### MongoDB Atlas
```bash
./kunji validate -k "mongodb+srv://..."  # Connection string
```

### Redis Cloud
```bash
./kunji validate -k "redis-..."
```

### ClickHouse
```bash
./kunji validate -k "clickhouse_..."
```

### InfluxDB
```bash
./kunji validate -k "influxdb_..."
```

### Cloudflare R2
```bash
./kunji validate -k "r2_..."
```

---

## Productivity (New)

### Linear
```bash
./kunji validate -k "lin_api_..."
```

### Monday
```bash
./kunji validate -k "monday_..."
```

### Airtable
```bash
./kunji validate -k "key..."  # Personal token
./kunji validate -k "pat..."  # Access token
```

### ClickUp
```bash
./kunji validate -k "pk_..."
```

### Figma
```bash
./kunji validate -k "figma_..."
```

---

## E-commerce & CMS (New)

### Shopify
```bash
./kunji validate -k "shpat_..."
./kunji validate -k "shpss_..."
```

### Strapi
```bash
./kunji validate -k "strapi_..."
```

### Sanity
```bash
./kunji validate -k "sk..."  # Project-specific
```

### Webflow
```bash
./kunji validate -k "wf_..."
```

---

## Blockchain (New)

### Alchemy
```bash
./kunji validate -k "alchemy_..."
```

### Infura
```bash
./kunji validate -k "infura_..."
```

### QuickNode
```bash
./kunji validate -k "qn-..."
```

### CoinGecko
```bash
./kunji validate -k "CG-..."
```

### Etherscan
```bash
./kunji validate -k "etherscan_..."
```

---

## Testing (New)

### LambdaTest
```bash
./kunji validate -k "lambdatest_..."
```

### Percy
```bash
./kunji validate -k "percy_..."
```

### TestRail
```bash
./kunji validate -k "testrail_..."
```

---

## Deployment (New)

### Koyeb
```bash
./kunji validate -k "koyeb_..."
```

### Coolify
```bash
./kunji validate -k "coolify_..."
```

---

## Search (New)

### Algolia
```bash
./kunji validate -k "..."  # Composite: appid:key
```

### Meilisearch
```bash
./kunji validate -k "meilisearch_..."
```

### Typesense
```bash
./kunji validate -k "typesense_..."
```

---

## Developer Tools (Existing)

### GitHub
```bash
./kunji validate -k "ghp_..."
./kunji validate -k "gho_..."
./kunji validate -k "ghu_..."
./kunji validate -k "ghs_..."
./kunji validate -k "ghr_..."
./kunji validate -k "github_pat_..."
```

### GitLab
```bash
./kunji validate -k "glpat-..."
```

### NPM
```bash
./kunji validate -k "npm_..."
```

### Supabase
```bash
./kunji validate -k "sbp_..."
```

---

## Communication (Existing)

### Slack
```bash
./kunji validate -k "xoxb-..."
./kunji validate -k "xoxp-..."
./kunji validate -k "xoxa-..."
./kunji validate -k "xoxr-..."
```

### Telegram
```bash
./kunji validate -k "..."  # Composite: bot_id:token
```

### Twilio
```bash
./kunji validate -k "AC...:..."  # Composite: SID:Token
```

### SendGrid
```bash
./kunji validate -k "SG...."
```

---

## Analytics (Existing)

### Mixpanel
```bash
./kunji validate -k "..."
```

### Amplitude
```bash
./kunji validate -k "..."  # Composite: client:secret
```

### Pendo
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
# List all 258 supported providers
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

---

## 🔒 Security Note

Kunji handles sensitive API keys. Please observe the following security best practices:

1. **Result File Permissions:** Kunji automatically creates result files with restrictive permissions (`0600` - readable only by your user). Do not change these permissions unless necessary.
2. **Plaintext Storage:** Validated keys are stored in plaintext within the output files. **Encrypt or securely delete** these files after use.
3. **Error Masking:** Kunji automatically scrubs API keys from error messages captured from providers to prevent accidental leakage in logs and result files.
