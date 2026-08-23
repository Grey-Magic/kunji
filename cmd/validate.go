package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Grey-Magic/kunji/pkg/audit"
	"github.com/Grey-Magic/kunji/pkg/client"
	"github.com/Grey-Magic/kunji/pkg/history"
	"github.com/Grey-Magic/kunji/pkg/runner"
	"github.com/Grey-Magic/kunji/pkg/sink"
	"github.com/Grey-Magic/kunji/pkg/source"
	"github.com/Grey-Magic/kunji/pkg/validators"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var (
	singleKey           string
	keysFile            string
	outputFile          string
	provider            string
	category            string
	threads             int
	proxy               string
	retries             int
	timeout             int
	resume              bool
	list                bool
	onlyValid           bool
	minBalance          float64
	templatesDir        string
	skipMetadata        bool
	canaryCheck         bool
	dryRun              bool
	bench               bool
	password            string
	deepScan            bool
	quiet               bool
	format              string
	globalRPS           int
	webhookURL          string
	webhookOn           string
	webhookPlatform     string
	webhookHeaders      []string
	webhookParams       []string
	webhookHeadersMap   map[string]string
	webhookRetries      int
	webhookBackoffMs    int
	webhookMaxBackoffMs int
	webhookSecret       string
	webhookSigPrefix    string
	telegramChatID      string
	pagerDutyRoutingKey string
	ntfyTopic           string
	gotifyPriority      int
	pushoverUserKey     string
	pushoverAppToken    string
	sinkDir             string
	sourceKind          string
	sourceArgs          []string
	noCache             bool
	cacheFile           string
	cacheTTLSecs        int
	filterExprs         []string
	failIf              []string
	auditFile           string
	noAudit             bool
	dedupeEmit          bool
	validateHistoryFile string
	noHistory           bool
	shardedPool         bool
	useHTTP3            bool
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate API keys",
	Long:  `A lightning-fast engine for validating API keys individually or in bulk, with built-in proxy rotation and metadata extraction.`,
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if !quiet && format != "json" {
			PrintBanner()
		}

		if templatesDir != "" {
			validators.CustomProvidersDir = templatesDir
		}

		if list {
			listProviders()
			return
		}

		hasInput := singleKey != "" || keysFile != ""
		stat, _ := os.Stdin.Stat()
		hasStdin := (stat.Mode() & os.ModeCharDevice) == 0

		if !hasInput && !hasStdin {
			pterm.Error.Println("Error: No input provided. Use -k/--key for a single key, -f/--keys for a file, or pipe keys via stdin.")
			pterm.Info.Println("Run 'kunji validate --help' for usage information.")
			os.Exit(1)
		}

		if singleKey != "" && keysFile != "" {
			pterm.Error.Println("Error: Cannot use both -k/--key and -f/--keys. Please provide only one input source.")
			os.Exit(1)
		}

		if singleKey == "" && keysFile == "" && hasStdin {
			if !quiet && format != "json" {
				pterm.Info.Println("Reading keys from stdin...")
			}
		}

		if threads < 1 || threads > 100 {
			pterm.Error.Printfln("Error: threads must be between 1 and 100 (got %d)", threads)
			os.Exit(1)
		}

		if timeout < 5 || timeout > 120 {
			pterm.Error.Printfln("Error: timeout must be between 5 and 120 seconds (got %d)", timeout)
			os.Exit(1)
		}

		if retries < 0 || retries > 10 {
			pterm.Error.Printfln("Error: retries must be between 0 and 10 (got %d)", retries)
			os.Exit(1)
		}

		parsedFilter, err := runner.ParseFilterExprs(filterExprs)
		if err != nil {
			pterm.Error.Printfln("Invalid --filter: %v", err)
			os.Exit(1)
		}

		parsedThresholds, err := runner.ParseThresholds(failIf)
		if err != nil {
			pterm.Error.Printfln("Invalid --fail-if: %v", err)
			os.Exit(1)
		}

		runr, err := runner.NewRunnerWithOptions(threads, proxy, retries, timeout, outputFile, provider, category, resume, onlyValid, minBalance, skipMetadata, canaryCheck, validators.FactoryOptions{
			PersistentCachePath: cacheFile,
			PersistentCacheTTL:  time.Duration(cacheTTLSecs) * time.Second,
			DisableCache:        noCache,
		})
		if err != nil {
			pterm.Error.Printfln("Error initializing runner: %v", err)
			return
		}
		runr.DeepScan = deepScan
		runr.Password = password
		runr.Bench = bench
		runr.Quiet = quiet
		runr.Format = format
		runr.NoCache = noCache
		runr.FilterExprs = parsedFilter
		runr.Thresholds = parsedThresholds
		runr.EmitDedupe = dedupeEmit
		runr.Sharded = shardedPool

		if useHTTP3 {
			client.EnableHTTP3(true)
			// The actual QUIC client requires github.com/quic-go/quic-go;
			// until it's added the request falls back to HTTP/2 silently.
			pterm.Info.Println("HTTP/3 requested: quic-go not vendored, falling back to HTTP/2.")
		}

		if !noAudit {
			auditPath := auditFile
			if auditPath == "" {
				auditPath = audit.DefaultPath()
			}
			al, err := audit.Open(auditPath)
			if err != nil {
				pterm.Warning.Printfln("audit log disabled: %v", err)
			} else {
				runr.Audit = al
				defer al.Close()
			}
		}
		if !noHistory {
			histPath := validateHistoryFile
			if histPath == "" {
				histPath = history.DefaultPath()
			}
			hl, err := history.Open(histPath, history.NewRunID())
			if err != nil {
				pterm.Warning.Printfln("history log disabled: %v", err)
			} else {
				runr.History = hl
				runr.RunID = hl.RunID()
				defer hl.Close()
			}
		}
		if format == "jsonl" {
			runr.Quiet = true
		}
		if globalRPS > 0 {
			runr.Factory.SharedLimiter().SetGlobalLimit(globalRPS)
		}

		params, paramErrs := parseParamList(webhookParams)
		if len(paramErrs) > 0 {
			pterm.Error.Printfln("%v", paramErrs[0])
			os.Exit(1)
		}
		if err := configureSinks(runr, sinkConfig{
			webhookHeaders:      parseHeaderList(webhookHeaders),
			webhookParams:       params,
			webhookURL:          webhookURL,
			webhookOn:           webhookOn,
			webhookPlatform:     webhookPlatform,
			webhookRetries:      webhookRetries,
			webhookBackoffMs:    webhookBackoffMs,
			webhookMaxBackoffMs: webhookMaxBackoffMs,
			webhookSecret:       webhookSecret,
			webhookSigPrefix:    webhookSigPrefix,
			telegramChatID:      telegramChatID,
			pagerDutyRoutingKey: pagerDutyRoutingKey,
			ntfyTopic:           ntfyTopic,
			gotifyPriority:      gotifyPriority,
			pushoverUserKey:     pushoverUserKey,
			pushoverAppToken:    pushoverAppToken,
			sinkDir:             sinkDir,
		}); err != nil {
			pterm.Error.Printfln("Sink configuration failed: %v", err)
			os.Exit(1)
		}

		if !quiet {
			runr.PreflightProxyCheck()
		}

		stream, count, err := runr.GetKeyStream(singleKey, keysFile)
		if err != nil {
			pterm.Error.Printfln("Error opening key stream: %v", err)
			return
		}
		defer stream.Close()

		if count == 0 {
			pterm.Warning.Println("No keys found to process.")
			return
		}

		if dryRun {
			pterm.Info.Printfln("Dry Run Mode: Analyzing %d keys...", count)
			scanner := bufio.NewScanner(stream)
			for scanner.Scan() {
				k := strings.TrimSpace(scanner.Text())
				if k == "" {
					continue
				}
				detection := runr.Detector.DetectProviderWithSuggestion(k, category)
				if detection.Provider != "unknown" {
					pName := detection.Provider
					val, exists := runr.Factory.GetValidator(pName)
					endpoint := "multiple potential"
					if exists {

						dummyRes, _ := val.Validate(context.TODO(), k)
						if dummyRes != nil {
							endpoint = dummyRes.Endpoint
						}
					}
					pterm.Success.Printfln("Key: %s... -> Provider: %s (%s)", k[:min(len(k), 8)], pName, endpoint)
				} else {
					pterm.Warning.Printfln("Key: %s... -> Unknown (Suggestions: %v)", k[:min(len(k), 8)], detection.Suggestions)
				}
			}
			return
		}

		if count == 1 {
		}

		if sourceKind != "" {
			src, err := source.Default.Open(sourceKind, sourceArgs)
			if err != nil {
				pterm.Error.Printfln("Source open failed: %v", err)
				os.Exit(1)
			}
			defer src.Close()
			runr.RunSource(context.Background(), src)
			return
		}

		runr.Run(stream, count)

		// CI threshold check. Any violated --fail-if clause exits non-zero
		// so this run can gate CI pipelines.
		if len(parsedThresholds) > 0 {
			violated, ok := runner.Evaluate(parsedThresholds, runr.Stats)
			if !ok {
				pterm.Error.Printfln("CI threshold violated: %s",
					runner.FormatViolations(violated, runr.Stats))
				os.Exit(2)
			}
		}
	},
}

func maskKey(k string) string {
	if len(k) < 12 {
		return "******"
	}
	return k[:6] + "....." + k[len(k)-4:]
}

func listProviders() {
	providers, err := validators.GetAllProviders()
	if err != nil {
		pterm.Error.Printfln("Error loading providers: %v", err)
		return
	}

	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Name < providers[j].Name
	})

	pterm.DefaultHeader.WithFullWidth().WithBackgroundStyle(pterm.NewStyle(pterm.BgLightMagenta)).WithTextStyle(pterm.NewStyle(pterm.FgBlack)).Println("Supported Providers")
	pterm.Println()

	tableData := pterm.TableData{
		{"Provider", "Category", "Prefixes", "Provider", "Category", "Prefixes"},
	}

	for i := 0; i < len(providers); i += 2 {
		p1 := providers[i]
		row := []string{
			pterm.LightCyan(p1.Name),
			pterm.Gray(p1.Category),
			pterm.LightYellow(getPrefixes(p1)),
		}

		if i+1 < len(providers) {
			p2 := providers[i+1]
			row = append(row,
				pterm.LightCyan(p2.Name),
				pterm.Gray(p2.Category),
				pterm.LightYellow(getPrefixes(p2)),
			)
		} else {
			row = append(row, "", "", "")
		}
		tableData = append(tableData, row)
	}

	pterm.DefaultTable.WithHasHeader().WithBoxed().WithData(tableData).Render()
	pterm.Info.Printfln("Total supported providers: %d", len(providers))
}

func getPrefixes(p validators.ProviderInfo) string {
	if len(p.KeyPrefixes) == 0 {
		return "-"
	}
	res := p.KeyPrefixes[0]
	if len(p.KeyPrefixes) > 1 {
		res += fmt.Sprintf(" (+%d)", len(p.KeyPrefixes)-1)
	}
	return res
}

func init() {
	rootCmd.AddCommand(validateCmd)

	validateCmd.Flags().StringVarP(&singleKey, "key", "k", "", "Single API key to validate")
	validateCmd.Flags().StringVarP(&keysFile, "keys", "f", "", "File containing multiple API keys (one per line)")
	validateCmd.Flags().StringVarP(&outputFile, "out", "o", "", "Output file for valid keys/results (can be .txt, .csv, .json, or .jsonl)")
	validateCmd.Flags().StringVarP(&provider, "provider", "p", "", "Force a specific provider (e.g. 'stripe', 'openai') to bypass regex auto-detection")
	validateCmd.Flags().StringVarP(&category, "category", "c", "", "Limit regex auto-detection to a specific category (e.g. 'llm', 'payments')")
	validateCmd.Flags().IntVarP(&threads, "threads", "t", 40, "Number of concurrent validation workers (1-100)")
	validateCmd.Flags().StringVar(&proxy, "proxy", "", "Proxy string (http://... or socks5://...) or path to proxy file")
	validateCmd.Flags().IntVarP(&retries, "retries", "r", 3, "Number of retries for failures or 429 Too Many Requests (0-10)")
	validateCmd.Flags().IntVar(&timeout, "timeout", 15, "Timeout in seconds per validation request (5-120)")
	validateCmd.Flags().BoolVar(&resume, "resume", false, "Resume from previous checkpoint file")
	validateCmd.Flags().BoolVarP(&list, "list", "l", false, "List all supported providers")

	validateCmd.Flags().BoolVar(&onlyValid, "only-valid", false, "Only output valid keys to file/console")
	validateCmd.Flags().BoolVar(&skipMetadata, "skip-metadata", false, "Skip fetching account metadata (balance, name, etc.) for speed")
	validateCmd.Flags().BoolVar(&canaryCheck, "no-canary-check", true, "Disable automated canary/honeypot token detection")
	validateCmd.Flags().Float64Var(&minBalance, "min-balance", 0.0, "Minimum balance required to consider key valid and output it")
	validateCmd.Flags().StringVarP(&templatesDir, "templates", "T", "", "Path to directory containing custom provider YAML files")
	validateCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Detect providers without making network requests")
	validateCmd.Flags().BoolVar(&deepScan, "deep-scan", false, "Try multiple providers if detection is ambiguous or fails")
	validateCmd.Flags().StringVar(&password, "password", "", "Password to encrypt output files or decrypt resume files")
	validateCmd.Flags().BoolVar(&bench, "bench", false, "Run 3 consecutive tests per key to measure average latency")
	validateCmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress banner, progress bar, and summary table")
	validateCmd.Flags().StringVar(&format, "format", "text", "Output format: text, json, or jsonl (newline-delimited JSON envelope events)")
	validateCmd.Flags().IntVar(&globalRPS, "global-rps", 0, "Cap aggregate outbound requests per second across all providers (0 = disabled)")

	defaultCacheFile := defaultPersistentCachePath()
	validateCmd.Flags().BoolVar(&noCache, "no-cache", false, "Skip both positive and negative caches; force network revalidation of every key")
	validateCmd.Flags().StringVar(&cacheFile, "cache-file", defaultCacheFile, "Path to the persistent positive cache (JSONL). Empty disables persistence; default is ~/.kunji/pos_cache.jsonl")
	validateCmd.Flags().IntVar(&cacheTTLSecs, "cache-ttl", 300, "Positive-cache freshness window in seconds (default 300 = 5 minutes)")

	validateCmd.Flags().StringArrayVar(&filterExprs, "filter", nil, "Pre-output filter: 'field=value' or 'field!=value'. Repeatable. Supported fields: provider, is_valid, status_code, error_code, key.")
	validateCmd.Flags().StringArrayVar(&failIf, "fail-if", nil, "CI threshold: 'metric op value[%]'. Repeatable. Metrics: invalid, valid, error, rate_limit, skipped, total. Example: 'invalid > 5%'.")
	validateCmd.Flags().StringVar(&auditFile, "audit-file", "", "Path to the audit log JSONL (default ~/.kunji/audit.jsonl; '' disables)")
	validateCmd.Flags().BoolVar(&noAudit, "no-audit", false, "Disable audit logging for this run")
	validateCmd.Flags().BoolVar(&dedupeEmit, "dedupe-emit", false, "Suppress duplicate (provider, key, is_valid) emits in the JSONL stream")
	validateCmd.Flags().StringVar(&validateHistoryFile, "history-file", "", "Path to the cross-run history JSONL (default ~/.kunji/history.jsonl; '' disables)")
	validateCmd.Flags().BoolVar(&noHistory, "no-history", false, "Disable cross-run history persistence for this run")
	validateCmd.Flags().BoolVar(&shardedPool, "sharded", false, "Use per-provider worker pools (reduces lock contention on the proxy rotator and rate limiter for mixed-provider inputs)")
	validateCmd.Flags().BoolVar(&useHTTP3, "http3", false, "Prefer HTTP/3 (QUIC) for outbound requests. Requires the github.com/quic-go/quic-go dependency; falls back to HTTP/2 if unavailable.")

	validateCmd.RegisterFlagCompletionFunc("provider", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		providers, err := validators.GetAllProviders()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		var names []string
		for _, p := range providers {
			if toComplete == "" || strings.HasPrefix(p.Name, toComplete) {
				names = append(names, p.Name)
			}
		}
		sort.Strings(names)
		return names, cobra.ShellCompDirectiveNoFileComp
	})

	validateCmd.RegisterFlagCompletionFunc("category", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		providers, err := validators.GetAllProviders()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		cats := make(map[string]bool)
		for _, p := range providers {
			cats[p.Category] = true
		}
		var names []string
		for c := range cats {
			if toComplete == "" || strings.HasPrefix(c, toComplete) {
				names = append(names, c)
			}
		}
		sort.Strings(names)
		return names, cobra.ShellCompDirectiveNoFileComp
	})

	validateCmd.Flags().StringVar(&webhookURL, "webhook", "", "POST each result to this URL. Use {provider} in the URL to route per-provider (e.g. https://hooks.example/{provider})")
	validateCmd.Flags().StringVar(&webhookOn, "webhook-on", "all", "Filter for --webhook: all, valid, or invalid")
	validateCmd.Flags().StringVar(&webhookPlatform, "webhook-platform", "raw", "Webhook platform: raw, slack, discord, teams, telegram, pagerduty, ntfy, gotify, pushover")
	validateCmd.Flags().StringArrayVar(&webhookHeaders, "webhook-header", nil, "Custom HTTP header 'Key: Value' (repeatable)")
	validateCmd.Flags().StringArrayVar(&webhookParams, "webhook-param", nil, "Platform-specific webhook parameter 'key=value' (repeatable). Recognized keys: chat_id, routing_key, topic, priority, user_key, app_token. Replaces --webhook-telegram-chat-id, --webhook-pagerduty-routing-key, --webhook-ntfy-topic, --webhook-gotify-priority, --webhook-pushover-user-key, --webhook-pushover-app-token.")
	validateCmd.Flags().IntVar(&webhookRetries, "webhook-retries", 3, "Number of retries on transient HTTP failures (5xx, 429)")
	validateCmd.Flags().IntVar(&webhookBackoffMs, "webhook-backoff-ms", 500, "Initial backoff between retries in milliseconds (doubles each attempt)")
	validateCmd.Flags().IntVar(&webhookMaxBackoffMs, "webhook-max-backoff-ms", 10000, "Max backoff between retries in milliseconds")
	validateCmd.Flags().StringVar(&webhookSecret, "webhook-secret", "", "HMAC-SHA256 signing secret; emits X-Kunji-Signature: <prefix><hex>")
	validateCmd.Flags().StringVar(&webhookSigPrefix, "webhook-sig-prefix", "", "Prefix for the HMAC signature header value (e.g. 'sha256=')")

	// Per-platform flags below are hidden but still accepted for backward
	// compatibility with scripts that used the 1.1.0 release. Prefer
	// --webhook-param key=value instead.
	validateCmd.Flags().StringVar(&telegramChatID, "webhook-telegram-chat-id", "", "Deprecated: use --webhook-param chat_id=<id>")
	_ = validateCmd.Flags().MarkHidden("webhook-telegram-chat-id")
	validateCmd.Flags().StringVar(&pagerDutyRoutingKey, "webhook-pagerduty-routing-key", "", "Deprecated: use --webhook-param routing_key=<key>")
	_ = validateCmd.Flags().MarkHidden("webhook-pagerduty-routing-key")
	validateCmd.Flags().StringVar(&ntfyTopic, "webhook-ntfy-topic", "", "Deprecated: use --webhook-param topic=<name>")
	_ = validateCmd.Flags().MarkHidden("webhook-ntfy-topic")
	validateCmd.Flags().IntVar(&gotifyPriority, "webhook-gotify-priority", 0, "Deprecated: use --webhook-param priority=<0..10>")
	_ = validateCmd.Flags().MarkHidden("webhook-gotify-priority")
	validateCmd.Flags().StringVar(&pushoverUserKey, "webhook-pushover-user-key", "", "Deprecated: use --webhook-param user_key=<key>")
	_ = validateCmd.Flags().MarkHidden("webhook-pushover-user-key")
	validateCmd.Flags().StringVar(&pushoverAppToken, "webhook-pushover-app-token", "", "Deprecated: use --webhook-param app_token=<token>")
	_ = validateCmd.Flags().MarkHidden("webhook-pushover-app-token")

	validateCmd.Flags().StringVar(&sinkDir, "sink", "", "Write each result as a JSON file into this directory")

	validateCmd.Flags().StringVar(&sourceKind, "source", "", "Key source plugin (file, stdin). When set, takes precedence over -k/-f.")
	validateCmd.Flags().StringArrayVar(&sourceArgs, "source-arg", nil, "Argument to pass to the source plugin (repeatable)")
}

func parseSinkFilter(s string) (sink.Filter, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "all", "":
		return sink.FilterAll, nil
	case "valid":
		return sink.FilterValid, nil
	case "invalid":
		return sink.FilterInvalid, nil
	}
	return 0, fmt.Errorf("unknown filter %q (expected all, valid, or invalid)", s)
}

func configureSinks(r *runner.Runner, cfg sinkConfig) error {
	filter, err := parseSinkFilter(cfg.webhookOn)
	if err != nil {
		return err
	}
	if cfg.webhookURL != "" {
		platform, err := sink.ParsePlatform(cfg.webhookPlatform)
		if err != nil {
			return err
		}
		fmtOpts := sink.FormatterOptions{
			TelegramChatID:      cfg.telegramChatID,
			PagerDutyRoutingKey: cfg.pagerDutyRoutingKey,
			NtfyTopic:           cfg.ntfyTopic,
			GotifyPriority:      cfg.gotifyPriority,
			PushoverUserKey:     cfg.pushoverUserKey,
			PushoverAppToken:    cfg.pushoverAppToken,
			Params:              cfg.webhookParams,
		}
		sinkOpts := sink.HTTPSinkOptions{
			Headers:         cfg.webhookHeaders,
			Retries:         cfg.webhookRetries,
			RetryBackoff:    time.Duration(cfg.webhookBackoffMs) * time.Millisecond,
			RetryMaxBackoff: time.Duration(cfg.webhookMaxBackoffMs) * time.Millisecond,
			SignatureSecret: cfg.webhookSecret,
			SignaturePrefix: cfg.webhookSigPrefix,
			Timeout:         10 * time.Second,
		}
		hs, err := sink.NewHTTPSinkWithFormatter(cfg.webhookURL, filter, platform, fmtOpts, sinkOpts)
		if err != nil {
			return err
		}
		r.Sinks = append(r.Sinks, hs)
	}
	if cfg.sinkDir != "" {
		fs, err := sink.NewFileSink(cfg.sinkDir, filter)
		if err != nil {
			return err
		}
		r.Sinks = append(r.Sinks, fs)
	}
	return nil
}

// sinkConfig carries the CLI options consumed by configureSinks. Defined as
// a struct so adding new sink options does not break the function signature.
type sinkConfig struct {
	webhookURL          string
	webhookOn           string
	webhookPlatform     string
	webhookHeaders      map[string]string
	webhookParams       map[string]string
	webhookRetries      int
	webhookBackoffMs    int
	webhookMaxBackoffMs int
	webhookSecret       string
	webhookSigPrefix    string
	telegramChatID      string
	pagerDutyRoutingKey string
	ntfyTopic           string
	gotifyPriority      int
	pushoverUserKey     string
	pushoverAppToken    string
	sinkDir             string
}

// defaultPersistentCachePath returns the default location for the cross-run
// positive cache (~/.kunji/pos_cache.jsonl). Falls back to the current
// directory if the home directory cannot be resolved.
func defaultPersistentCachePath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".kunji", "pos_cache.jsonl")
	}
	return filepath.Join(home, ".kunji", "pos_cache.jsonl")
}

// parseHeaderList converts ["Authorization: Bearer xyz", "X-Tenant: acme"]
// into a map[string]string. Malformed entries are skipped (logged via pterm
// debug would be nicer but we keep it quiet to avoid noisy bulk runs).
func parseHeaderList(entries []string) map[string]string {
	if len(entries) == 0 {
		return nil
	}
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		idx := strings.Index(e, ":")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(e[:idx])
		val := strings.TrimSpace(e[idx+1:])
		if key == "" {
			continue
		}
		out[key] = val
	}
	return out
}

// parseParamList converts ["chat_id=12345", "user_key=abc"] into a map and
// returns a slice of errors for malformed entries. Empty input returns
// (nil, nil). Whitespace is trimmed from keys and values.
func parseParamList(entries []string) (map[string]string, []error) {
	if len(entries) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(entries))
	var errs []error
	for _, e := range entries {
		idx := strings.Index(e, "=")
		if idx <= 0 {
			errs = append(errs, fmt.Errorf("--webhook-param %q: expected key=value", e))
			continue
		}
		key := strings.TrimSpace(e[:idx])
		val := strings.TrimSpace(e[idx+1:])
		if key == "" {
			errs = append(errs, fmt.Errorf("--webhook-param %q: empty key", e))
			continue
		}
		out[key] = val
	}
	return out, errs
}
