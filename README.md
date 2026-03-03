<div align="center">

# Kunji

**A fast, concurrent CLI tool for validating API keys.**

![Go](https://img.shields.io/badge/Go-1.21%2B-00ADD8?style=flat-square&logo=go)
![Version](https://img.shields.io/badge/Version-1.0.0-blue?style=flat-square)
![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey?style=flat-square)

</div>

---

Kunji validates API keys concurrently across dozens of services. It auto-detects the provider, checks key validity, extracts account metadata where supported, and exports results in multiple formats.

> **Note:** For detailed usage examples for each provider, see [USAGE.md](./USAGE.md)

## Features

- **Auto-Detection** — Identifies providers via prefix trie and regex fallback
- **Concurrent** — Worker pool with configurable thread count
- **Metadata Extraction** — Balance, account name, and email for valid keys
- **Proxy Support** — Single proxy or rotating proxy file
- **Smart Resume** — Skip already-validated keys on restart
- **Multiple Exports** — `.txt`, `.csv`, `.json` output formats
- **Self-Update** — Built-in `kunji update` command

## Installation

**Go Install (Recommended):**

```bash
go install github.com/Grey-Magic/kunji@latest
```

**From source:**

```bash
git clone https://github.com/Grey-Magic/kunji.git
cd kunji
go build -o kunji .
sudo mv kunji /usr/local/bin/
```

**Prebuilt binary:**

```bash
curl -sL https://github.com/Grey-Magic/kunji/releases/latest/download/kunji_1.0.0.zip -o kunji.zip
unzip kunji.zip
chmod +x kunji
sudo mv kunji /usr/local/bin/
```

**Update:**

```bash
# Built-in updater
kunji update

# Or reinstall
go install github.com/Grey-Magic/kunji@latest
```

## Usage

```bash
# Single key
kunji validate -k "sk-ant-api03-..."

# Bulk file
kunji validate -f keys.txt -o results.csv -t 20

# With proxy and resume
kunji validate -f keys.txt --proxy proxies.txt --resume

# Force a specific provider (skips Regex detection)
kunji validate -f stripe_dumps.txt -p stripe

# Limit detection to a specific category
kunji validate -f server_logs.txt -c llm
```

## Flags

| Flag | Short | Default | Description |
|---|---|---|---|
| `--key` | `-k` | — | Single API key to validate |
| `--keys` | `-f` | — | File with one key per line |
| `--out` | `-o` | `results.txt` | Output file (`.txt`, `.csv`, `.json`) |
| `--provider` | `-p` | — | Force a specific provider, skip auto-detection |
| `--category` | `-c` | — | Limit auto-detection to a specific category |
| `--threads` | `-t` | `10` | Number of concurrent workers |
| `--proxy` | — | — | Proxy URL or path to proxy list file |
| `--retries` | `-r` | `3` | Retries on failure or HTTP 429 |
| `--timeout` | — | `15` | Request timeout in seconds |
| `--resume` | — | `false` | Skip keys already in the output file |
| `--list` | `-l` | — | List all supported providers |

## Supported Providers (103)

| Category | Providers |
|---|---|
| **Foundation** | OpenAI, Anthropic, Google Gemini, xAI (Grok), Mistral |
| **Inference APIs** | Groq, Together AI, Fireworks AI, Novita AI, Replicate |
| **Aggregators** | OpenRouter, HuggingFace |
| **Regional** | DeepSeek, Qwen, GLM (Zhipu), Kimi (Moonshot), MiniMax |
| **Tools** | Cohere, Perplexity, ElevenLabs, Venice AI, Midjourney |
| **AI Coding Proxies** | Kilo, Cline, RooCode, Aider |
| **Cloud & Hosting** | Cloudflare, Heroku, JumpCloud, DigitalOcean |
| **Security & OSINT** | FOFA, ZoomEye, Netlas, Intelligence X, VirusTotal, Censys, Shodan |
| **Monetization** | Stripe, PayPal, Square |
| **Developers** | GitHub, NPM, Supabase, CircleCI |
| **Productivity/Comm** | Notion, Slack, Twilio, SendGrid, Telegram, Asana, Discord |
| **Analytics/Ops** | DataDog, OpsGenie, PagerDuty, WakaTime, Mixpanel, Amplitude |
| **Maps & Data** | Google Maps, Ipstack, Mapbox, Bing Maps |
| **Misc SaaS** | Dropbox, Lokalise, SauceLabs, HubSpot, BrowserStack, Spotify |

## Adding a Provider

All providers are defined in `pkg/validators/providers/llm.yaml` or `common-services.yaml` — no Go code required.

```yaml
- name: myprovider
  key_prefixes: ["mp-"]
  key_patterns: ["^mp-[a-zA-Z0-9]{32}$"]
  validation:
    method: POST
    url: "https://api.myprovider.com/v1/chat/completions"
    auth: "bearer"
    body: '{"model":"gpt-4o","messages":[{"role":"user","content":"Hi"}],"max_tokens":1}'
```

## License

MIT
