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
[![Version](https://img.shields.io/badge/Version-1.0.9-magenta?style=flat-square)](https://github.com/Grey-Magic/kunji/releases)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey?style=flat-square)](#installation)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)](LICENSE)

<p align="center">
  <a href="#features">Features</a> •
  <a href="#installation">Installation</a> •
  <a href="#usage">Usage</a> •
  <a href="#security">Security</a> •
  <a href="#supported-providers">Providers</a> •
  <a href="./USAGE.md">Full Manual</a>
</p>

</div>

---

Kunji is a concurrent CLI tool for validating API keys across 350+ services. It uses a scoring-based detection engine and multi-threaded execution to verify credentials and extract associated metadata.

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
```

## Features

### Detection and Analysis
- **Scoring-based Auto-Detection** — Evaluates prefixes, regex specificity, and structural characteristics to identify providers.
- **Structural Decoding** — Decodes JWTs (`eyJ...`) and identifies Hex/UUID fingerprints to resolve provider collisions.
- **GraphQL Introspection** — Identifies root types and schema statistics for GraphQL-based services upon successful validation.
- **Custom Templates** — Supports loading provider definitions from external YAML files via the `--templates` flag.

### Security and Evasion
- **Request Randomization** — Rotates HTTP headers and TLS fingerprints to prevent identification by WAFs or rate-limiters.
- **Canary Detection** — Identifies common AWS and Slack canary tokens and high-entropy strings to prevent triggering security alerts.
- **Secret Scrubbing** — Automatically masks API keys in output and logs.

### Performance Optimizations
- **Aho-Corasick Scanning** — Uses a trie-based automaton to match multiple provider prefixes in a single pass.
- **Zero-Copy Processing** — Minimizes memory allocations during string processing and key detection.
- **Adaptive Throttling** — Adjusts request rates per-provider based on `429` (Too Many Requests) responses.
- **Connection Warming** — Pre-resolves DNS and establishes TCP/TLS handshakes for common providers at startup.
- **Persistent Negative Cache** — Uses a Bloom filter to store and skip confirmed invalid keys across sessions.

## Installation

### Go Install
```bash
go install github.com/Grey-Magic/kunji@latest
```

### Prebuilt Binaries
Download the release for your platform:
```bash
# Example for Linux/macOS
curl -sL https://github.com/Grey-Magic/kunji/releases/latest/download/kunji_1.0.9.zip -o kunji.zip
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
```

### Advanced Options
| Flag | Description |
|---|---|
| `-T, --templates` | Path to directory containing custom provider YAML files. |
| `--deep-scan` | Test multiple providers if detection is ambiguous. |
| `--proxy` | Set a proxy URL or a file for rotation. |
| `--dry-run` | Identify providers without sending network requests. |
| `--skip-metadata` | Skip metadata enrichment steps. |

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
