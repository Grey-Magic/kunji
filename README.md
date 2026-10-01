<div align="center">

<pre>
  ██   ██ ██    ██ ███    ██      ██ ██
  ██  ██  ██    ██ ████   ██      ██ ██
  █████   ██    ██ ██ ██  ██      ██ ██
  ██  ██  ██    ██ ██  ██ ██ ██   ██ ██
  ██   ██  ██████  ██   ████  █████  ██
</pre>

# Kunji

**Universal API key validation engine — a concurrent Go CLI for testing credentials across 350+ services.**

[![Release](https://img.shields.io/github/v/release/Grey-Magic/kunji?style=flat-square&color=magenta)](https://github.com/Grey-Magic/kunji/releases)
[![License: MIT](https://img.shields.io/github/license/Grey-Magic/kunji?style=flat-square&color=green)](./LICENSE)
[![Go 1.21+](https://img.shields.io/badge/Go-1.21%2B-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev)
[![Stars](https://img.shields.io/github/stars/Grey-Magic/kunji?style=flat-square)](https://github.com/Grey-Magic/kunji/stargazers)

</div>

Kunji validates API keys in bulk against 350+ providers. Point it at a file of mixed-format keys; it auto-detects the provider, runs a concurrent validation pass with proxy rotation and per-provider rate limiting, and emits results as text, JSON, or a stable JSONL event stream for piping into CI and webhook sinks.

```text
Validating API Keys [348/351] ███████████████████████████████████░ 99%
  » supabase        ✓ Valid    eyJhbGciOiJIUzI1Ni... (JWT Decoded)
  » openai          ✓ Valid    sk-proj-****xyz789
  » stripe          ✗ Invalid  sk_live_****123456
  » deepseek        ✓ Valid    sk-****def456 (Hex Fingerprint)
```

## Install

```bash
go install github.com/Grey-Magic/kunji@latest
```

Or grab a prebuilt binary from [Releases](https://github.com/Grey-Magic/kunji/releases), unzip, and put `kunji` on your `$PATH`.

## Usage

```bash
# Single key
kunji validate -k "sk-proj-..."

# Bulk from a file
kunji validate -f keys.txt -t 20

# Pipe JSONL events into jq
kunji validate -f keys.txt --format jsonl | jq 'select(.event=="result" and .result.is_valid)'

# Custom provider templates + dry-run (no network)
kunji validate -f keys.txt -T ./my-templates/ --dry-run

# Gate CI with thresholds
kunji validate -f keys.txt --fail-if "invalid > 5%"
```

Run `kunji --help` or `kunji validate --help` for the full flag list. See [USAGE.md](./USAGE.md) for the complete manual covering JSONL envelopes, webhook sinks (Slack/Discord/Telegram/PagerDuty/…), `~/.kunji/config.yaml` profiles, the positive/negative caches, the audit log, and the cross-run `kunji history` command.

## What it does

- **Auto-detection** — Aho-Corasick single-pass prefix matching plus scoring against regex specificity and structural fingerprints (JWT decode, hex/UUID). Per-provider `detection.min_score` knobs in YAML stop neighbors stealing hits.
- **Concurrency** — tunable worker pools, proxy rotation, adaptive throttling per `429`, `--global-rps` cap, `--sharded` mode for per-provider pools.
- **Caches** — persistent positive cache (gzip + base64 JSONL at `~/.kunji/pos_cache.jsonl`) and a cross-run negative Bloom filter. `--no-cache` forces a fresh check; `--cache-ttl` controls freshness.
- **Outputs** — `text` (default, live progress + summary table), `json` (legacy per-result), `jsonl` (stable `start` / `progress` / `result` / `summary` envelope, schema `1.0.0`).
- **Sinks & webhooks** — `--webhook` + `--webhook-platform slack|discord|teams|telegram|pagerduty|ntfy|gotify|pushover`, HMAC-signed bodies, `--sink <dir>` for filesystem triage.
- **CI gating** — repeatable `--fail-if "metric op value[%]"` returns exit code `2` on violation.
- **Safe by default** — masked output (`sk-****xyz789`), canary-token detection, audit log stores only SHA-256 key hashes.
- **Custom providers** — drop YAML files in a directory and pass `--templates`; validate with `kunji lint-providers --path <dir>`.

## License

MIT — see [LICENSE](./LICENSE).