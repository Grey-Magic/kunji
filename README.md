<div align="center">

<pre>
  ██   ██ ██    ██ ███    ██      ██ ██
  ██  ██  ██    ██ ████   ██      ██ ██
  █████   ██    ██ ██ ██  ██      ██ ██
  ██  ██  ██    ██ ██  ██ ██ ██   ██ ██
  ██   ██  ██████  ██   ████  █████  ██
</pre>

**Universal API Key Validation Engine**

[![Go](https://img.shields.io/badge/Go-1.21%2B-00ADD8?style=flat-square&logo=go)](https://golang.org)
[![Version](https://img.shields.io/badge/Version-1.1.0-magenta?style=flat-square)](https://github.com/Grey-Magic/kunji/releases)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%2B%20Windows-lightgrey?style=flat-square)](#installation)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)](LICENSE)

<p align="center">
  <a href="#features">Features</a> •
  <a href="#installation">Installation</a> •
  <a href="#usage">Usage</a> •
  <a href="#output-formats">Output</a> •
  <a href="#webhooks-and-sinks">Webhooks</a> •
  <a href="#security">Security</a> •
  <a href="#supported-providers">Providers</a> •
  <a href="./USAGE.md">Full Manual</a>
</p>

</div>

---

Kunji is a concurrent CLI tool for validating API keys across 350+ services. It uses a scoring-based detection engine and multi-threaded execution to verify credentials and extract associated metadata, with stable JSONL streaming, per-provider rate control, and pluggable webhook sinks for any platform.

## Terminal Experience

Kunji provides real-time feedback during bulk validation operations.

```text
  ██   ██ ██    ██ ███    ██      ██ ██
  ██  ██  ██    ██ ████   ██      ██ ██
  █████   ██    ██ ██ ██  ██      ██ ██
  ██  ██  ██    ██ ██  ██ ██ ██   ██ ██
  ██   ██  ██████  ██   ████  █████  ██

Validating API Keys [348/351] ███████████████████████████████████░ 99%
  » supabase        ✓ Valid    eyJhbGciOiJIUzI1Ni... (JWT Decoded)
  » openai          ✓ Valid    sk-proj-****xyz789
  » stripe          ✗ Invalid  sk_live_****123456
  » deepseek        ✓ Valid    sk-****def456 (Hex Fingerprint)

Per-Provider Breakdown
Provider     Valid  Invalid  Rate Ltd  Skip  Total  Hit %
openai          12       3         0     0     15  80.0%
stripe           5       0         0     0      5  100.0%
supabase         8       1         0     0      9  88.9%
```

## Features

### Detection and Analysis
- **Scoring-based Auto-Detection** — Evaluates prefixes, regex specificity, and structural characteristics to identify providers.
- **Structural Decoding** — Decodes JWTs (`eyJ...`) and identifies Hex/UUID fingerprints to resolve provider collisions.
- **GraphQL Introspection** — Identifies root types and schema statistics for GraphQL-based services upon successful validation.
- **Custom Templates** — Supports loading provider definitions from external YAML files via the `--templates` flag.
- **YAML Provider Schema Extensions** — Custom templates can express `response_match` (body regex / jsonpath / header), `failure_when`, and `retry_on_status` without touching Go code.

### Security and Evasion
- **Request Randomization** — Rotates HTTP headers and TLS fingerprints to prevent identification by WAFs or rate-limiters.
- **Canary Detection** — Identifies common AWS and Slack canary tokens and high-entropy strings to prevent triggering security alerts.
- **Secret Scrubbing** — Automatically masks API keys in output and logs.
- **Retry-After Honoring** — Server-provided backoff hints extend the per-provider rate-limit window past the default schedule.

### Performance Optimizations
- **Aho-Corasick Scanning** — Uses a trie-based automaton to match multiple provider prefixes in a single pass.
- **Zero-Copy Processing** — Minimizes memory allocations during string processing and key detection.
- **Adaptive Throttling** — Adjusts request rates per-provider based on `429` (Too Many Requests) responses.
- **Connection Warming** — Pre-resolves DNS and establishes TCP/TLS handshakes for common providers at startup.
- **Global Rate Budget** — Cap aggregate outbound RPS across all providers with `--global-rps`.
- **Negative Bloom Cache** — Cross-run Bloom filter skips keys known to be invalid.
- **Persistent Positive Cache** — JSONL cache at `~/.kunji/pos_cache.jsonl` skips re-validation within the freshness window.
- **Force Revalidation** — `--no-cache` bypasses both caches for fresh network checks.

### Output and Pipelines
- **JSONL Stream** — `--format jsonl` emits a stable envelope (`start`, `progress`, `result`, `summary`) on one line per event, schema version `1.0.0`.
- **Per-Provider Breakdown** — End-of-run table sorted by hits with hit-rate color coding.
- **Webhook Sinks** — Built-in payload formatters for Slack, Discord, Microsoft Teams, Telegram, PagerDuty, ntfy, Gotify, and Pushover.
- **File Sink** — One JSON file per result into a directory, content-hashed filenames.
- **Custom Headers and HMAC** — Auth headers, signing secrets, and per-platform retry/backoff.
- **Source Plugins** — `--source file | stdin` plus extensible registry for vault-backed keys.

## Installation

### Go Install
```bash
go install github.com/Grey-Magic/kunji@latest
```

### Prebuilt Binaries
Download the release for your platform:
```bash
# Example for Linux/macOS
curl -sL https://github.com/Grey-Magic/kunji/releases/latest/download/kunji_1.1.0.zip -o kunji.zip
unzip kunji.zip && chmod +x kunji
sudo mv kunji /usr/local/bin/
```

---

## Usage

### Basic Commands
```bash
# Validate a single key
kunji validate -k "sk-proj-..."

# Bulk validation with custom templates
kunji validate -f keys.txt -T ./my-templates/ -t 20

# Resume a run and skip known invalid keys
kunji validate -f keys.txt --resume --only-valid -o results.jsonl

# Pipe JSONL events straight into jq / a downstream pipeline
kunji validate -f keys.txt --format jsonl | jq 'select(.event=="result" and .result.is_valid)'
```

### Advanced Options

| Flag | Description |
|---|---|
| `-T, --templates` | Path to directory containing custom provider YAML files. |
| `--deep-scan` | Test multiple providers if detection is ambiguous. |
| `--proxy` | Set a proxy URL or a file for rotation. |
| `--dry-run` | Identify providers without sending network requests. |
| `--skip-metadata` | Skip metadata enrichment steps. |
| `--format` | Output format: `text`, `json`, or `jsonl` (newline-delimited envelope events). |
| `--global-rps` | Cap aggregate outbound requests per second across all providers. |
| `--no-cache` | Skip both positive and negative caches; force network revalidation. |
| `--cache-file` | Path to the persistent positive cache (default `~/.kunji/pos_cache.jsonl`). |
| `--cache-ttl` | Positive-cache freshness window in seconds (default `300`). |
| `--webhook` | POST each result to this URL. `{provider}` is substituted per-provider. |
| `--webhook-platform` | `raw`, `slack`, `discord`, `teams`, `telegram`, `pagerduty`, `ntfy`, `gotify`, `pushover`. |
| `--webhook-header` | Custom HTTP header `Key: Value` (repeatable). |
| `--webhook-retries` | Number of retries on transient HTTP failures (5xx, 429). |
| `--webhook-secret` | HMAC-SHA256 signing secret; emits `X-Kunji-Signature`. |
| `--sink` | Write each result as a JSON file into this directory. |
| `--source` | Key source plugin: `file`, `stdin`. Overrides `-k`/`-f`. |

---

## Output Formats

Kunji supports three output modes.

### `text` (default)
Human-readable progress, rolling recent-results panel, and an end-of-run summary with the per-provider breakdown table.

### `json`
One `ValidationResult` object per result, line-delimited JSON. Backward-compatible with existing scripts.

### `jsonl`
A stable envelope emitted one event per line. Useful for piping into `jq`, Vector, or custom tooling.

```json
{"event":"start","timestamp":"2026-07-01T11:30:00Z","schema_version":"1.0.0","start":{"total":1000,"threads":40}}
{"event":"progress","timestamp":"...","progress":{"done":250,"total":1000,"valid":210,"keys_per_second":42.5,"eta_seconds":17}}
{"event":"result","timestamp":"...","result":{"key":"sk-...","provider":"openai","is_valid":true,"status_code":200,...}}
{"event":"summary","timestamp":"...","summary":{"total":1000,"valid":812,"invalid":175,"duration_ms":23500,"by_provider":{"openai":{"valid":312,...}}}}
```

Selecting `jsonl` automatically suppresses the banner and progress bar so output is pipe-clean.

---

## Webhooks and Sinks

Pipe results into any platform with a webhook. Each platform formats the payload into the JSON shape its endpoint expects.

```bash
# Slack
kunji validate -f keys.txt --webhook https://hooks.slack.com/services/T.../B.../XXX \
  --webhook-platform slack

# Discord
kunji validate -f keys.txt --webhook https://discord.com/api/webhooks/XXX/YYY \
  --webhook-platform discord

# Telegram
kunji validate -f keys.txt --webhook https://api.telegram.org/bot<TOKEN>/sendMessage \
  --webhook-platform telegram --webhook-telegram-chat-id -1001234567890

# PagerDuty (triggers on invalid, acknowledges on valid)
kunji validate -f keys.txt --webhook https://events.pagerduty.com/v2/enqueue \
  --webhook-platform pagerduty --webhook-pagerduty-routing-key <RK>

# ntfy
kunji validate -f keys.txt --webhook https://ntfy.sh/my-alerts \
  --webhook-platform ntfy --webhook-ntfy-topic alerts

# Custom HTTP endpoint with auth header and signed body
kunji validate -f keys.txt --webhook https://intake.example.com/kunji \
  --webhook-header "Authorization: Bearer mytoken" \
  --webhook-secret topsecret --webhook-sig-prefix "sha256="
```

Webhook requests automatically retry on `5xx` and `429` with exponential backoff (`--webhook-retries`, `--webhook-backoff-ms`).

For filesystem-based routing, `--sink <dir>` writes one JSON file per result. Combined with `--webhook-on valid|invalid|all`, you can route only the keys that matter.

```bash
# Drop invalid keys into a triage folder, alert Slack on every valid key
kunji validate -f keys.txt \
  --sink ./triage/ --webhook-on invalid \
  --webhook https://hooks.slack.com/services/... --webhook-platform slack --webhook-on valid
```

---

## Security

- All API keys are masked in terminal output and log streams (`sk-****xyz789`).
- Canary tokens (AWS, Slack, high-entropy decoys) are detected before any network request and skipped automatically.
- Webhook bodies can be HMAC-signed so downstream intake endpoints can verify authenticity.
- Persistent caches store SHA-256 hashes of `(provider, key)`, never the raw key.

---

## Supported Providers (350+)

| Category | Services |
|---|---|
| **LLMs** | OpenAI, Anthropic, Google Gemini, xAI, Mistral, DeepSeek |
| **Hosting** | Cloudflare, Vercel, Netlify, Railway, DigitalOcean, Heroku, Render |
| **Databases** | Supabase, MongoDB Atlas, Redis, ClickHouse, TiDB, Neon |
| **Identity** | Auth0, Clerk, WorkOS, Stytch, Frontegg, FusionAuth |
| **Payments** | Stripe, PayPal, Square, LemonSqueezy, Paddle, Plaid |

---

## License

This project is licensed under the MIT License.