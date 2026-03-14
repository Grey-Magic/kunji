package cmd

import (
	"fmt"
	"sort"

	"github.com/Grey-Magic/kunji/pkg/runner"
	"github.com/Grey-Magic/kunji/pkg/validators"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var (
	singleKey  string
	keysFile   string
	outputFile string
	provider   string
	category   string
	threads    int
	proxy      string
	retries    int
	timeout    int
	resume     bool
	list       bool
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate API keys",
	Long:  `A lightning-fast engine for validating API keys individually or in bulk, with built-in proxy rotation and metadata extraction.`,
	Run: func(cmd *cobra.Command, args []string) {
		PrintBanner()

		if list {
			listProviders()
			return
		}

		if threads < 1 {
			threads = 1
			pterm.Warning.Println("Threads set to minimum of 1")
		}
		if threads > 100 {
			threads = 100
			pterm.Warning.Println("Threads capped at 100")
		}

		if timeout < 5 {
			timeout = 5
			pterm.Warning.Println("Timeout set to minimum of 5 seconds")
		}
		if timeout > 120 {
			timeout = 120
			pterm.Warning.Println("Timeout capped at 120 seconds")
		}

		if retries < 0 {
			retries = 0
			pterm.Warning.Println("Retries set to 0 (no retries)")
		}
		if retries > 10 {
			retries = 10
			pterm.Warning.Println("Retries capped at 10")
		}

		pterm.Info.Printfln("Starting validation with %d threads...", threads)

		runr, err := runner.NewRunner(threads, proxy, retries, timeout, outputFile, provider, category, resume)
		if err != nil {
			pterm.Error.Printfln("Error initializing runner: %v", err)
			return
		}

		keys, err := runr.LoadAndFilterKeys(singleKey, keysFile)
		if err != nil {
			pterm.Error.Printfln("Error loading keys: %v", err)
			return
		}

		if len(keys) == 0 {
			pterm.Warning.Println("No valid keys found to process.")
			return
		}

		runr.Run(keys)
	},
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
	validateCmd.Flags().StringVarP(&outputFile, "out", "o", "", "Output file for valid keys/results (can be .txt, .csv, or .json)")
	validateCmd.Flags().StringVarP(&provider, "provider", "p", "", "Force a specific provider (e.g. 'stripe', 'openai') to bypass regex auto-detection")
	validateCmd.Flags().StringVarP(&category, "category", "c", "", "Limit regex auto-detection to a specific category (e.g. 'llm', 'payments')")
	validateCmd.Flags().IntVarP(&threads, "threads", "t", 10, "Number of concurrent validation workers (1-100)")
	validateCmd.Flags().StringVar(&proxy, "proxy", "", "Proxy string (http://... or socks5://...) or path to proxy file")
	validateCmd.Flags().IntVarP(&retries, "retries", "r", 3, "Number of retries for failures or 429 Too Many Requests (0-10)")
	validateCmd.Flags().IntVar(&timeout, "timeout", 15, "Timeout in seconds per validation request (5-120)")
	validateCmd.Flags().BoolVar(&resume, "resume", false, "Resume from previous checkpoint file")
	validateCmd.Flags().BoolVarP(&list, "list", "l", false, "List all supported providers")
}
