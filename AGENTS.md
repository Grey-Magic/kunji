# AGENTS.md

Go CLI (`github.com/Grey-Magic/kunji`, go.mod says 1.25). Cobra + pterm. Entry `main.go` → `cmd.Execute()`; main flow is `cmd/validate.go` → `pkg/runner.Runner`.

## Verify

```bash
go build ./...
go vet ./...
go test ./...
go test ./pkg/<name>/ -run TestX -count=1   # focused; -count=1 (caches hide provider-embed changes)
```

No CI, lint, or pre-commit config in repo. `make test` is just verbose `go test`; prefer direct commands. `make build` stamps the version via ldflags — plain `go build` leaves the default `1.2.0`.

## Architecture

- `cmd/` holds one file per subcommand (`validate`, `lint-providers`, `history`, `doctor`, `providers`, `dedupe`, `diff`, …). `validate` wires all flags into `runner.NewRunnerWithOptions` + `validators.FactoryOptions`.
- Provider definitions are YAML embedded at compile time (`//go:embed providers` in `pkg/validators/config.go`), loaded once per process via `sync.Once` in `LoadProviderConfigs`.
- `validators.CustomProvidersDir` (the `--templates` value) must be set **before** the first `LoadProviderConfigs` call. `~/.kunji/providers` is auto-loaded too. Tests inject stubs via `factory.RegisterConfig` — never mutate the global cache.
- `ValidatorFactory` shares one HTTP client, one rate-limiter manager, and one layered (memory + disk) cache; validators are built lazily via `GetValidator`. Detection is `Detector.DetectProviderWithSuggestion`; per-provider validation lives in `generic.go`.
- `pkg/client` owns HTTP, proxy rotation, rate limiting, and caches. `pkg/config` resolves `~/.kunji/config.yaml` (+ `KUNJI_CONFIG`, `KUNJI_PROFILE`, `KUNJI_<FLAG>` env — see below).

## Gotchas

- `lint-providers` only globs `*.yaml` (not `*.yml`); dirs only. Exit 0 = no errors, 1 = errors. It currently flags built-in `pkg/validators/providers/*.yaml` (templated `{{key…}}` URLs, `auth: body`) — don't bulk-"fix" those.
- `--http3` is a stub: `quic-go` is not in go.mod, requests silently fall back to HTTP/2 (`pkg/client/http3.go`, `cmd/validate.go`).
- `--no-canary-check` is inverted: it defaults to `true`, and `true` **enables** the canary check (`SetCanaryCheck(true)` → `checkCanary=true` in `generic.go`). Disable with `--no-canary-check=false`.
- `validate` writes local state: `~/.kunji/audit.jsonl`, `~/.kunji/history.jsonl`, and `.kunji_neg_cache` in cwd (all gitignored). For safe local checks use `--dry-run` (no network) and/or `--no-audit --no-history --no-cache`.
- `validate` exits `1` on misuse (threads must be 1–100, timeout 5–120, retries 0–10) and `2` when a `--fail-if` threshold is violated. `--format jsonl` forces quiet mode.
- Config resolution, highest first: CLI flag → `KUNJI_<FLAG>` env → active profile → built-in default. Profile: `--profile` → `KUNJI_PROFILE` → `default_profile`. Path: `--config`/`KUNJI_CONFIG` → `~/.kunji/config.yaml` → `.kunji/config.yaml`. Missing file is fine; malformed file exits 1.
- `pkg/runner/integration_test.go` hits the network: valid-key cases skip unless `KUNJI_TEST_OPENAI_KEY` / `KUNJI_TEST_GITHUB_KEY` are set, but invalid-key cases still make live requests.
- Never use or commit real API keys in commands, tests, or logs. Output masks keys (`sk-****xyz789`); caches/audit store SHA-256 hashes only.
