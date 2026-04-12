package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Grey-Magic/kunji/pkg/runner"
	"github.com/Grey-Magic/kunji/pkg/validators"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var (
	singleKey       string
	keysFile        string
	outputFile      string
	provider        string
	category        string
	threads         int
	proxy           string
	retries         int
	timeout         int
	resume          bool
	list            bool
	onlyValid       bool
	minBalance      float64
	customProviders string
	skipMetadata    bool
	canaryCheck     bool
	dryRun          bool
	bench           bool
	password        string
	deepScan        bool
	quiet           bool
	format          string
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

		if customProviders != "" {
			validators.CustomProvidersDir = customProviders
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

		runr, err := runner.NewRunner(threads, proxy, retries, timeout, outputFile, provider, category, resume, onlyValid, minBalance, skipMetadata, canaryCheck)
		if err != nil {
			pterm.Error.Printfln("Error initializing runner: %v", err)
			return
		}
		runr.DeepScan = deepScan
		runr.Password = password
		runr.Bench = bench
		runr.Quiet = quiet
		runr.Format = format

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

		runr.Run(stream, count)
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
	validateCmd.Flags().IntVarP(&threads, "threads", "t", 10, "Number of concurrent validation workers (1-100)")
	validateCmd.Flags().StringVar(&proxy, "proxy", "", "Proxy string (http://... or socks5://...) or path to proxy file")
	validateCmd.Flags().IntVarP(&retries, "retries", "r", 3, "Number of retries for failures or 429 Too Many Requests (0-10)")
	validateCmd.Flags().IntVar(&timeout, "timeout", 15, "Timeout in seconds per validation request (5-120)")
	validateCmd.Flags().BoolVar(&resume, "resume", false, "Resume from previous checkpoint file")
	validateCmd.Flags().BoolVarP(&list, "list", "l", false, "List all supported providers")

	validateCmd.Flags().BoolVar(&onlyValid, "only-valid", false, "Only output valid keys to file/console")
	validateCmd.Flags().BoolVar(&skipMetadata, "skip-metadata", false, "Skip fetching account metadata (balance, name, etc.) for speed")
	validateCmd.Flags().BoolVar(&canaryCheck, "no-canary-check", true, "Disable automated canary/honeypot token detection")
	validateCmd.Flags().Float64Var(&minBalance, "min-balance", 0.0, "Minimum balance required to consider key valid and output it")
	validateCmd.Flags().StringVar(&customProviders, "custom-providers", "", "Path to directory containing custom provider YAML files")
	validateCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Detect providers without making network requests")
	validateCmd.Flags().BoolVar(&deepScan, "deep-scan", false, "Try multiple providers if detection is ambiguous or fails")
	validateCmd.Flags().StringVar(&password, "password", "", "Password to encrypt output files or decrypt resume files")
	validateCmd.Flags().BoolVar(&bench, "bench", false, "Run 3 consecutive tests per key to measure average latency")
	validateCmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress banner, progress bar, and summary table")
	validateCmd.Flags().StringVar(&format, "format", "text", "Output format: text or json")

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
}
