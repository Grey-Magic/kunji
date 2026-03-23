package cmd

import (
	"bufio"
	"os"
	"regexp"
	"strings"

	"github.com/Grey-Magic/kunji/pkg/runner"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var interactiveCmd = &cobra.Command{
	Use:   "interactive",
	Short: "Interactive paste mode",
	Long:  `Paste a block of text containing API keys. The tool will automatically extract and validate them. Press Ctrl+D when finished.`,
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		PrintBanner()

		if threads < 1 || threads > 100 {
			pterm.Error.Printfln("Error: threads must be between 1 and 100 (got %d)", threads)
			os.Exit(1)
		}

		if timeout < 5 || timeout > 120 {
			pterm.Error.Printfln("Error: timeout must be between 5 and 120 seconds (got %d)", timeout)
			os.Exit(1)
		}

		pterm.Info.Println("Paste your text containing API keys below. Press Ctrl+D (or Ctrl+Z on Windows) on a new line when done:")

		scanner := bufio.NewScanner(os.Stdin)
		var sb strings.Builder
		for scanner.Scan() {
			sb.WriteString(scanner.Text() + "\n")
		}

		if err := scanner.Err(); err != nil {
			pterm.Error.Printfln("Error reading input: %v", err)
			return
		}

		text := sb.String()

		re := regexp.MustCompile(`([a-zA-Z0-9_-]{15,})`)
		matches := re.FindAllString(text, -1)

		if len(matches) == 0 {
			pterm.Warning.Println("No potential API keys found in the provided text.")
			return
		}

		if threads < 1 {
			threads = 10
		}
		if timeout < 5 {
			timeout = 15
		}

		runr, err := runner.NewRunner(threads, proxy, 0, 15, outputFile, provider, category, false, onlyValid, minBalance, skipMetadata, canaryCheck)
		if err != nil {
			pterm.Error.Printfln("Error initializing runner: %v", err)
			return
		}

		pterm.Info.Printfln("Found %d potential keys. Starting validation...", len(matches))

		uniqueKeys := make(map[string]bool)
		var finalKeys []string
		for _, k := range matches {
			if !uniqueKeys[k] {
				uniqueKeys[k] = true
				finalKeys = append(finalKeys, k)
			}
		}

		runr.Password = password
		runr.PreflightProxyCheck()
		runr.Run(strings.NewReader(strings.Join(finalKeys, "\n")), len(finalKeys))
		},
		}

		func init() {
		rootCmd.AddCommand(interactiveCmd)

		interactiveCmd.Flags().StringVarP(&outputFile, "out", "o", "", "Output file for valid keys/results")
		interactiveCmd.Flags().StringVarP(&provider, "provider", "p", "", "Force a specific provider")
		interactiveCmd.Flags().StringVarP(&category, "category", "c", "", "Limit regex auto-detection to a specific category")
		interactiveCmd.Flags().IntVarP(&threads, "threads", "t", 10, "Number of concurrent validation workers")
		interactiveCmd.Flags().StringVar(&proxy, "proxy", "", "Proxy string")
		interactiveCmd.Flags().BoolVar(&onlyValid, "only-valid", false, "Only output valid keys to file/console")
		interactiveCmd.Flags().BoolVar(&skipMetadata, "skip-metadata", false, "Skip fetching account metadata for speed")
		interactiveCmd.Flags().BoolVar(&canaryCheck, "no-canary-check", true, "Disable automated canary/honeypot token detection")
		interactiveCmd.Flags().Float64Var(&minBalance, "min-balance", 0.0, "Minimum balance required")

		interactiveCmd.Flags().StringVar(&password, "password", "", "Password to encrypt output files")
		}

