# Changelog

---

## [1.2.0] - Unreleased

### Added
- **13 new providers** (static credentials, single-request validation, `detection.min_score` guards): `cerebras`, `chutes`, `friendli`, `parasail`, `tavily` (LLM); `triggerdotdev`, `unkey`, `rubygems`, `cursor` (dev); `tailscale` (infra); `mercadopago` (payments); `axiom` (monitoring).
- **`auth: body`** support — linter now accepts it, requires a `{{key}}` template so body-auth entries cannot go out unauthenticated.
- **Header value prefixes** — `auth: "header:<name>:<prefix>"` sends `<prefix><key>` (e.g. `header:Authorization:token ` for Snyk-style schemes); `{{key}}`/`{{key.client_id}}`/`{{key.secret}}` also expand inside custom `headers:` values.
- **`expected_status` honored** — listed statuses take the full success path (body checks, metadata); e.g. `expected_status: [200, 404]` for HIBP-style responses.
- **HTTP/3 via quic-go** — `--http3` dials QUIC per host with bounded (~3s) attempt, automatic HTTP/2 fallback, per-host outcome cache. Ignored with `--proxy`. Bodies replayed safely on fallback; unrewindable bodies skip H3.
- **Proxy latency scoring + egress reporting** — `FilterDeadProxies` times probes, captures exit IP from ipify, sorts survivors fastest-first; `Fastest()` / `EgressIPs()` exposed; preflight and `kunji doctor` report fastest proxy + latency + egress.
- **ClientHello diversification** — shared client shuffles cipher-suite and curve order per construction (stdlib only; genuine JA3 cipher/curve segment rotation).
- **Header-set variation** — per-request rotation of `Sec-CH-UA-Platform` plus random benign-header subset (`DNT`, `Upgrade-Insecure-Requests`, `Sec-GPC`).

### Changed
- Dependencies: added `github.com/quic-go/quic-go` (direct) with `qpack`/`x/net` indirect; `x/sys`, `x/term`, `x/text`, `x/crypto`, `testify` moved forward per resolver.

### Removed
- (none)

### Fixed
- **Composite credential in bearer/header auth** — `host:secret` keys (Coolify, Dokku, CapRover, Easypanel) sent the whole `host:secret` string as the Bearer token and always 401'd. When the endpoint URL consumes `{{key.client_id}}`, auth now carries only the secret part.
- **Linter template blindness** — `lintURL` parsed `{{key.client_id}}`/`{{key}}`/`{{header.*}}` placeholders literally, producing 46 false-positive errors. Placeholders are now substituted with dummies before structural checks; full-tree lint is down to 1 issue (the genuinely unimplemented `aws_sigv4` scheme — flagged, not fixed).
- **Auth verification tests** — `allowLoopback` helper flips `utils.SkipSSRFCheck` for httptest servers (matching `regression_test.go` pattern); 12 new tests cover composite secret-split, header prefix, template substitution, basic_composite exemption, expected_status, H3 fallback/body-replay/cache/shape, TLS invariants, cipher shuffling.

### Internals
- `pkg/client/tlsprofile.go` — stdlib TLS fingerprint randomization (no new dep; full uTLS profile rotation deferred).
- `pkg/client/http3.go` — real QUIC racing transport replacing the stub; `pkg/client/client.go` — `ProxyRotator.Fastest()`, `EgressIPs()`, latency-weighted rotation.
- `USAGE.md` — auth schemes table, `expected_status`, header prefixes, HTTP/3 behavior.

---
