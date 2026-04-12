# Kunji - Complete Usage Guide

Kunji is a CLI tool that validates API keys across 351+ services. This guide covers everything you need to know.

---

## Quick Start

```bash
# Single key validation
./kunji validate -k "sk-your-key-here"

# From file (one key per line)
./kunji validate -f keys.txt

# Interactive Paste Mode (great for quickly pasting a block of text)
./kunji interactive
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

# Stream JSONL output (best for huge files, saves memory)
./kunji validate -f keys.txt -o results.jsonl

# Only save valid keys
./kunji validate -f keys.txt -o valid.txt --only-valid

# Only save valid keys with a minimum balance
./kunji validate -f keys.txt -o funded.txt --only-valid --min-balance 5.0

# Encrypted output
./kunji validate -f keys.txt -o results.json --password "my-secret"

# JSON streaming to stdout
./kunji validate -f keys.txt --format json --quiet
```

### Dry Run (Detection Mode)

Want to know what providers your keys belong to without making network requests?

```bash
./kunji validate -f keys.txt --dry-run
```

### Custom Providers

If you have your own YAML provider definitions:

```bash
./kunji validate -f keys.txt --custom-providers ./my-definitions/
```

### Deep Scan

When key detection is ambiguous, probe across all potential providers:

```bash
./kunji validate -f keys.txt --deep-scan
```

### Benchmark Mode

Measure average latency per key with 3 consecutive tests:

```bash
./kunji validate -k "sk-proj-..." --bench
```

### Skip Metadata

Skip account metadata extraction (balance, email) for faster validation:

```bash
./kunji validate -f keys.txt --skip-metadata
```

### Output Formats

```bash
# Text (default)
./kunji validate -f keys.txt -o results.txt

# CSV (great for Excel)
./kunji validate -f keys.txt -o results.csv

# JSON (for scripts)
./kunji validate -f keys.txt -o results.json

# JSON Lines (best for large scales)
./kunji validate -f keys.txt -o results.jsonl
```

### Proxy Checking

To make sure your proxies are functioning properly before starting a huge scan:

```bash
./kunji check-proxies --proxy proxy.txt
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
./kunji validate -k "rk_live_..."      # Stripe (restricted)
./kunji validate -k "vercel_..."       # Vercel
./kunji validate -k "dop_v1_..."        # DigitalOcean
./kunji validate -k "PMAK-..."          # Postman
./kunji validate -k "rdme_..."          # ReadMe
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

# Mux (AccessKey:SecretKey)
./kunji validate -k "access_key:secret_key"

# Confluent (APIKey:APISecret)
./kunji validate -k "api_key:api_secret"
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
- `{{key.client_id}}` -> first part
- `{{key.secret}}` -> second part

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
./kunji validate -k "AIza..." -p google_maps
./kunji validate -k "AIza..." -p google_geocoding
./kunji validate -k "AIza..." -p google_elevation
./kunji validate -k "AIza..." -p google_distance_matrix
./kunji validate -k "AIza..." -p google_places
./kunji validate -k "AIza..." -p google_timezone
./kunji validate -k "AIza..." -p google_directions
./kunji validate -k "AIza..." -p google_translate
./kunji validate -k "AIza..." -p google_firebase
./kunji validate -k "AIza..." -p google_sheets
./kunji validate -k "AIza..." -p google_custom_search
./kunji validate -k "AIza..." -p google_pagespeed
./kunji validate -k "AIza..." -p google_geolocate
./kunji validate -k "AIza..." -p google_books
./kunji validate -k "AIza..." -p google_safe_browsing
./kunji validate -k "AIza..." -p google_recaptcha_enterprise
./kunji validate -k "AIza..." -p youtube
./kunji validate -k "AIza..." -p gemini
```

### Filter by Category

Limit detection to specific categories:

```bash
# Only LLM providers
./kunji validate -f keys.txt -c llm

# Only payment providers
./kunji validate -f keys.txt -c payments

# Only communication services
./kunji validate -f keys.txt -c communication

# Available categories: llm, payments, communication, cloud, database, monitoring, security, api, developer, devops, productivity, ecommerce, crm, analytics, email, container, storage, dns, cdn, ai, feature_flags, cms, search, blockchain, testing, deployment, iac, cicd, secrets, web3, ai_ml, collaboration, documentation, api_gateway, auth, auth, localization, marketing, support, social, imaging
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

## Cloud & Infrastructure

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

### Render
```bash
./kunji validate -k "rnd_..."
```

### Northflank
```bash
./kunji validate -k "nf-..."
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
./kunji validate -k "do_pat_..."
```

### Cloudflare
```bash
./kunji validate -k "cloudflare_..."
```

### ngrok
```bash
./kunji validate -k "ak_..."
```

---

## AI/ML Services

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
./kunji validate -k "lsv2_..."
```

### Langfuse
```bash
./kunji validate -k "pk-lf-..."
```

### Weights & Biases
```bash
./kunji validate -k "wandb_..."
```

### AssemblyAI (Speech-to-Text)
```bash
./kunji validate -k "..."
```

### Deepgram (Speech-to-Text)
```bash
./kunji validate -k "..."
```

### Stability AI (Image Generation)
```bash
./kunji validate -k "sk-..."
```

### Rev.ai (Transcription)
```bash
./kunji validate -k "rev_..."
```

### Speechmatics (Speech-to-Text)
```bash
./kunji validate -k "..."
```

### RunPod (GPU Cloud)
```bash
./kunji validate -k "..."
```

### Baseten (ML Deployment)
```bash
./kunji validate -k "..."
```

---

## Security & Auth

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

### AbuseIPDB
```bash
./kunji validate -k "..."
```

### GreyNoise
```bash
./kunji validate -k "gn_..."
```

### SecurityTrails
```bash
./kunji validate -k "st_..."
```

### IPQualityScore
```bash
./kunji validate -k "..."
```

### SonarQube
```bash
./kunji validate -k "squ_..."
```

### Semgrep
```bash
./kunji validate -k "semgrep_..."
```

---

## Communication

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

### Ably (Real-time Messaging)
```bash
./kunji validate -k "..."
```

### MessageBird
```bash
./kunji validate -k "live_..."
./kunji validate -k "test_..."
```

### OneSignal
```bash
./kunji validate -k "os_..."
```

### Svix (Webhooks)
```bash
./kunji validate -k "sk_live_..."
./kunji validate -k "sk_test_..."
```

### Telnyx
```bash
./kunji validate -k "KEY..."
```

### Daily.co (Video)
```bash
./kunji validate -k "..."
```

### ClickSend
```bash
./kunji validate -k "..."
```

### TalkJS (Chat SDK)
```bash
./kunji validate -k "sk_live_..."
./kunji validate -k "sk_test_..."
```

### Airship (Push)
```bash
./kunji validate -k "..."
```

### Knock
```bash
./kunji validate -k "knock_..."
```

---

## Payments

### Stripe
```bash
./kunji validate -k "sk_live_..."   # Live
./kunji validate -k "sk_test_..."   # Test
./kunji validate -k "rk_live_..."   # Restricted (live)
./kunji validate -k "rk_test_..."   # Restricted (test)
```

### Adyen
```bash
./kunji validate -k "..."
```

### Checkout.com
```bash
./kunji validate -k "sk_test_..."
./kunji validate -k "sk_live_..."
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

### Recharge (Subscriptions)
```bash
./kunji validate -k "..."
```

---

## Email

### Brevo (Sendinblue)
```bash
./kunji validate -k "xkeysib-..."
```

### Mailtrap
```bash
./kunji validate -k "mt_..."
```

### Mailersend
```bash
./kunji validate -k "mlsn_..."
```

### Mailjet
```bash
./kunji validate -k "..."  # Composite: apikey:secretkey
```

### SparkPost
```bash
./kunji validate -k "sp_..."
```

### ConvertKit
```bash
./kunji validate -k "ck_..."
```

### Customer.io
```bash
./kunji validate -k "..."  # Composite: siteid:apikey
```

### Plunk
```bash
./kunji validate -k "plln_sk_..."
./kunji validate -k "sk_plnk_..."
```

### Mailchimp Transactional (Mandrill)
```bash
./kunji validate -k "..."
```

---

## DevOps

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

## Monitoring

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

### Rollbar
```bash
./kunji validate -k "pk_..."
./kunji validate -k "ps_..."
```

### Bugsnag
```bash
./kunji validate -k "..."
```

### Papertrail
```bash
./kunji validate -k "..."
```

### FullStory
```bash
./kunji validate -k "..."
```

### Pingdom
```bash
./kunji validate -k "..."
```

### Instatus
```bash
./kunji validate -k "..."
```

---

## Database

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

### Tinybird
```bash
./kunji validate -k "p_..."
```

### CockroachDB
```bash
./kunji validate -k "crdb_..."
```

### TiDB
```bash
./kunji validate -k "tidb_..."
```

### Yugabyte
```bash
./kunji validate -k "yb-..."
```

### TimescaleDB
```bash
./kunji validate -k "tsdb-..."
```

---

## CRM & Productivity

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
./kunji validate -k "key..."
./kunji validate -k "pat..."
```

### ClickUp
```bash
./kunji validate -k "pk_..."
```

### Figma
```bash
./kunji validate -k "figma_..."
```

### Pipedrive
```bash
./kunji validate -k "..."
```

### Close.io
```bash
./kunji validate -k "close_..."
```

### Todoist
```bash
./kunji validate -k "td_..."
```

### Cal.com
```bash
./kunji validate -k "cal_..."
./kunji validate -k "cal_live_..."
```

### Trello
```bash
./kunji validate -k "..."  # Composite: apikey:token
```

---

## Analytics

### Fathom
```bash
./kunji validate -k "fth_..."
```

### Plausible
```bash
./kunji validate -k "pls_..."
```

### Mouseflow
```bash
./kunji validate -k "..."
```

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

## Container & Storage

### Docker Hub
```bash
./kunji validate -k "dckr_pat_..."
```

### Quay.io
```bash
./kunji validate -k "..."
```

### Backblaze B2
```bash
./kunji validate -k "..."  # Composite: keyid:key
```

---

## DNS & CDN

### NS1
```bash
./kunji validate -k "..."
```

### Fastly
```bash
./kunji validate -k "..."
```

### Bunny.net
```bash
./kunji validate -k "bny_..."
```

---

## Developer Tools

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

### Postman
```bash
./kunji validate -k "PMAK-..."
```

### ReadMe
```bash
./kunji validate -k "rdme_..."
```

### Hookdeck (Webhooks)
```bash
./kunji validate -k "hkdk_..."
```

### Zapier
```bash
./kunji validate -k "..."
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

## API & Data

### Hunter.io (Email Intelligence)
```bash
./kunji validate -k "hunter_..."
```

### Radar (Geofencing)
```bash
./kunji validate -k "prj_live_sk_..."
./kunji validate -k "prj_test_sk_..."
```

### Formspree
```bash
./kunji validate -k "fsr_..."
```

### ScrapingBee
```bash
./kunji validate -k "sb_..."
```

### ScreenshotOne
```bash
./kunji validate -k "so_..."
```

### WeatherAPI
```bash
./kunji validate -k "wa_..."
```

### HERE Maps
```bash
./kunji validate -k "here_..."
```

### IPinfo
```bash
./kunji validate -k "..."
```

### Clearbit
```bash
./kunji validate -k "..."
```

### Apollo.io (B2B Data)
```bash
./kunji validate -k "..."
```

### Jotform
```bash
./kunji validate -k "..."
```

### Typeform
```bash
./kunji validate -k "tfp_..."
./kunji validate -k "tfa_..."
```

### Duffel (Travel)
```bash
./kunji validate -k "duffel_test_..."
./kunji validate -k "duffel_live_..."
```

### Giphy
```bash
./kunji validate -k "..."
```

### Unsplash
```bash
./kunji validate -k "..."
```

### Tinify / TinyPNG
```bash
./kunji validate -k "..."
```

### PDFShift
```bash
./kunji validate -k "sk_..."
```

### Vimeo
```bash
./kunji validate -k "..."
```

### Mux (Video Streaming)
```bash
./kunji validate -k "..."  # Composite: access_key:secret_key
```

### Confluent (Kafka)
```bash
./kunji validate -k "..."  # Composite: api_key:api_secret
```

### GoDaddy
```bash
./kunji validate -k "..."  # Composite: key:secret
```

### Porkbun
```bash
./kunji validate -k "..."  # Composite: apikey:secretapikey
```

---

## Localization

### Crowdin
```bash
./kunji validate -k "..."
```

### Transifex
```bash
./kunji validate -k "..."
```

### Phrase
```bash
./kunji validate -k "..."
```

---

## Feature Flags

### LaunchDarkly
```bash
./kunji validate -k "ld-..."
```

### Split.io
```bash
./kunji validate -k "..."
```

---

## E-commerce & CMS

### Shopify
```bash
./kunji validate -k "shpat_..."
./kunji validate -k "shpss_..."
```

### WooCommerce
```bash
./kunji validate -k "..."  # Composite: domain:ck_...
```

### BigCommerce
```bash
./kunji validate -k "bc_..."
```

### Recharge (Subscriptions)
```bash
./kunji validate -k "..."
```

### Strapi
```bash
./kunji validate -k "strapi_..."
```

### Sanity
```bash
./kunji validate -k "sk..."
```

### Webflow
```bash
./kunji validate -k "wf_..."
```

### Storyblok
```bash
./kunji validate -k "storyblok_..."
```

### DatoCMS
```bash
./kunji validate -k "..."
```

### Prismic
```bash
./kunji validate -k "..."
```

---

## Blockchain

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

### CoinMarketCap
```bash
./kunji validate -k "cmc_..."
```

---

## Testing

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

### TestingBot
```bash
./kunji validate -k "testingbot_..."
```

### Chromatic
```bash
./kunji validate -k "chromatic_..."
```

---

## Deployment

### Koyeb
```bash
./kunji validate -k "koyeb_..."
```

### Coolify
```bash
./kunji validate -k "coolify_..."
```

---

## Search

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

# Resume with encrypted output
./kunji validate -f keys.txt --resume -o results.csv --password "my-secret"
```

### Quiet Mode

```bash
# Suppress all output except results (useful for piping)
./kunji validate -f keys.txt --quiet --format json
```

### Encrypted Output

```bash
# Encrypt results with a password
./kunji validate -f keys.txt -o results.json --password "my-secret"
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
# List all 351 supported providers
./kunji validate --list
```

---

## Complete Flag Reference

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--key` | `-k` | - | Single API key to validate |
| `--keys` | `-f` | - | File with keys (one per line) |
| `--out` | `-o` | - | Output file (.txt, .csv, .json, .jsonl) |
| `--provider` | `-p` | auto | Force specific provider |
| `--category` | `-c` | all | Filter by category |
| `--threads` | `-t` | 10 | Concurrent workers (1-100) |
| `--timeout` | - | 15 | Request timeout in seconds (5-120) |
| `--retries` | `-r` | 3 | Retry count for failures/429 (0-10) |
| `--proxy` | - | - | Proxy URL or proxy list file |
| `--resume` | - | false | Skip already-validated keys |
| `--list` | `-l` | - | List all supported providers |
| `--only-valid` | - | false | Only output valid keys |
| `--min-balance` | - | 0.0 | Minimum balance to consider key valid |
| `--skip-metadata` | - | false | Skip metadata extraction for speed |
| `--no-canary-check` | - | true | Disable canary/honeypot detection |
| `--custom-providers` | - | - | Path to custom provider YAML files |
| `--dry-run` | - | false | Detect providers without network requests |
| `--deep-scan` | - | false | Try multiple providers on ambiguous keys |
| `--password` | - | - | Encrypt output / decrypt resume files |
| `--bench` | - | false | Benchmark: 3 tests per key for avg latency |
| `--quiet` | - | false | Suppress banner, progress, summary |
| `--format` | - | text | Output format: text or json |

---

## Output Examples

### Valid Key Output
```
OpenAI
   Key: sk-proj-****abc123
   Balance: $12.50
   Email: user@example.com
   Models: gpt-4, gpt-3.5-turbo
```

### Invalid Key Output
```
OpenAI
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
    "is_valid": true,
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
6. **Use --deep-scan** when keys could belong to multiple providers
7. **Use --skip-metadata** for faster bulk validation when you only need valid/invalid status
8. **Use --password** to encrypt output files containing sensitive keys
9. **Pipe with --quiet --format json** for integration with other tools

---

## Troubleshooting

### "Provider not detected"
- Use `-p <provider>` to force
- Use `--deep-scan` to try multiple providers

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

## Security Note

Kunji handles sensitive API keys. Please observe the following security best practices:

1. **Result File Permissions:** Kunji automatically creates result files with restrictive permissions (`0600` - readable only by your user). Do not change these permissions unless necessary.
2. **Plaintext Storage:** Validated keys are stored in plaintext within the output files. **Encrypt or securely delete** these files after use.
3. **Encrypted Output:** Use `--password` to encrypt output files with AES-256-GCM.
4. **SSRF Prevention:** Kunji includes built-in protection to prevent SSRF attacks when using composite keys with custom hosts. It will block requests to local or private IP addresses.
5. **Error Masking:** Kunji automatically scrubs API keys from error messages captured from providers to prevent accidental leakage in logs and result files.
6. **Canary Detection:** Built-in canary/honeypot token detection identifies and skips known honeypot keys (GitHub, AWS, etc.) to avoid triggering alerts.
