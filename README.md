<div align="center">

```
  ██   ██ ██    ██ ███    ██      ██ ██
  ██  ██  ██    ██ ████   ██      ██ ██
  █████   ██    ██ ██ ██  ██      ██ ██
  ██  ██  ██    ██ ██  ██ ██ ██   ██ ██
  ██   ██  ██████  ██   ████  █████  ██
```

**Universal API Key Validation Engine**

[![Go](https://img.shields.io/badge/Go-1.21%2B-00ADD8?style=flat-square&logo=go)](https://golang.org)
[![Version](https://img.shields.io/badge/Version-1.0.5-magenta?style=flat-square)](https://github.com/Grey-Magic/kunji/releases)
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

**Kunji** is a high-performance, concurrent CLI tool designed to rapidly validate API keys across 260+ services. Whether you're auditing infrastructure, testing integrations, or cleaning up configuration dumps, Kunji provides a safe, fast, and automated way to verify credentials.

## 🚀 Terminal Experience

Kunji features a modern, interactive UI built with `pterm`, providing real-time feedback during bulk operations.

```text
  ██   ██ ██    ██ ███    ██      ██ ██
  ██  ██  ██    ██ ████   ██      ██ ██
  █████   ██    ██ ██ ██  ██      ██ ██
  ██  ██  ██    ██ ██  ██ ██ ██   ██ ██
  ██   ██  ██████  ██   ████  █████  ██

Validating API Keys [256/260] ███████████████████████████████████░ 98%
  » shopify         ✓ Valid    myshop:shpat_****abc123
  » openai          ✓ Valid    sk-proj-****xyz789
  » stripe          ✗ Invalid  sk_live_****123456
  » deepseek        ✓ Valid    sk-****def456
```

## ✨ Key Features

- 🔍 **Smart Auto-Detection** — Instantly identifies 260+ services via a high-speed prefix trie and sensitive regex fallback.
- ⚡ **Concurrent Engine** — Multi-threaded worker pool handles thousands of keys in seconds with configurable throughput.
- 📊 **Metadata Extraction** — Automatically retrieves account balance, email, usage limits, and organization names.
- 🛡️ **Hardened Security** — Built-in SSRF protection, restrictive file permissions, and automatic secret scrubbing in logs.
- 🔄 **Smart Resume & Retry** — Skip already-validated keys and handle intermittent network failures or rate limits with jittered backoff.
- 📤 **Clean Export** — Generate structured reports in `.txt`, `.csv`, `.json`, or memory-efficient `.jsonl`.
- 🕹️ **Interactive Paste Mode** — Paste blocks of text and let Kunji auto-extract and validate the keys.
- 🕵️ **Dry Run Mode** — Detect providers without sending a single network request.

## 📦 Installation

### Go Install (Recommended)
```bash
go install github.com/Grey-Magic/kunji@latest
```

### Prebuilt Binaries
Download the latest release for your platform:
```bash
# Example for Linux/macOS
curl -sL https://github.com/Grey-Magic/kunji/releases/latest/download/kunji_1.0.5.zip -o kunji.zip
unzip kunji.zip && chmod +x kunji
sudo mv kunji /usr/local/bin/
```

---

## 🛠️ Usage

For a comprehensive guide including service-specific examples, see [**USAGE.md**](./USAGE.md).

### Basic Commands
```bash
# Validate a single key (auto-detects provider)
kunji validate -k "sk-proj-..."

# Bulk validation from a file with 20 workers
kunji validate -f keys.txt -o results.csv -t 20

# Resume an interrupted run, only keeping valid keys
kunji validate -f keys.txt --resume --only-valid -o results.jsonl

# Quick interactive paste mode
kunji interactive

# Check your proxies
kunji check-proxies --proxy proxies.txt
```

### Advanced Options
| Flag | Description |
|---|---|
| `-p, --provider` | Force a specific provider (e.g., `openai`) to skip detection. |
| `-c, --category` | Limit detection to a category (e.g., `llm`, `payments`). |
| `--proxy` | Provide a single proxy or a file for automatic rotation. |
| `--timeout` | Set custom request timeout (default: 15s). |
| `--dry-run` | Detect providers without making network requests. |
| `--custom-providers` | Load extra YAML provider definitions from a directory. |

---

## 🔒 Security First

Kunji is designed with data privacy and security as a core mandate:

1. **User-Only Permissions:** All result files are created with `0600` permissions (`-rw-------`), ensuring only you can read your validated keys.
2. **SSRF Protection:** Built-in validation blocks requests to `localhost` and private IP ranges, preventing the tool from being used as an internal scanner.
3. **Secret Scrubbing:** Kunji automatically detects and masks API keys in error messages (`[MASKED_KEY]`) before they are saved to disk.
4. **No Data Exfiltration:** Validation happens directly between your machine and the provider API.

---

## 🏛️ Supported Providers (260+)

Kunji supports an extensive array of services across multiple domains:

| Category | Top Services |
|---|---|
| **Foundation LLMs** | OpenAI, Anthropic, Google Gemini, xAI, Mistral, DeepSeek |
| **Cloud & Hosting** | Cloudflare, Vercel, Netlify, Railway, DigitalOcean, Heroku, Render |
| **Databases** | MongoDB Atlas, Redis Cloud, ClickHouse, CockroachDB, TiDB, Neon |
| **Security & OSINT** | Shodan, VirusTotal, Censys, FOFA, ZoomEye, Intelligence X |
| **Auth & Identity** | Auth0, Clerk, WorkOS, Stytch, Frontegg, FusionAuth |
| **DevOps & CICD** | GitHub, GitLab, NPM, Supabase, CircleCI, Travis CI, Pulumi, ArgoCD |
| **Payments** | Stripe, PayPal, Square, LemonSqueezy, Paddle, Plaid |
| **CMS & E-com** | Shopify, WooCommerce, Strapi, Sanity, Contentful, Webflow, Prismic |
| **Blockchain** | Alchemy, Infura, QuickNode, Etherscan, Moralis, Thirdweb |

---

## 🤝 Contributing

Adding a new provider is simple and requires **zero Go code**. Simply add a YAML entry to `pkg/validators/providers/`:

```yaml
- name: new_service
  key_prefixes: ["ns-"]
  key_patterns: ["^ns-[a-zA-Z0-9]{32}$"]
  validation:
    method: GET
    url: "https://api.newservice.com/v1/user"
    auth: "bearer"
```

See [**AGENTS.md**](./AGENTS.md) for full development guidelines.

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
