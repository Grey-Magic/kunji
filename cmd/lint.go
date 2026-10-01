package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Grey-Magic/kunji/pkg/validators"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var (
	lintPaths []string
	lintJSON  bool
)

var lintProvidersCmd = &cobra.Command{
	Use:   "lint-providers",
	Short: "Validate custom provider YAML files against the schema",
	Long: `Lint custom provider YAML files before using them with --templates.

Catches the most common mistakes that would otherwise produce silent runtime
failures:

  * missing or invalid key_prefixes / key_patterns
  * invalid regex syntax in key_patterns, canary_patterns, regex_extract
  * unknown HTTP methods, unknown auth schemes, unsupported syntax_check
  * missing validation.url / validation.endpoints
  * malformed or non-http(s) URLs
  * misconfigured detection.min_score, cache_ttl_seconds, retry_policy

Exit code 0 means no errors (warnings may still be present).
Exit code 1 means at least one error was found.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if len(lintPaths) == 0 {
			pterm.Error.Println("Please provide at least one path with --path.")
			os.Exit(1)
		}

		var configs []validators.ProviderConfig
		var loadErrors []string

		for _, p := range lintPaths {
			info, err := os.Stat(p)
			if err != nil {
				loadErrors = append(loadErrors, fmt.Sprintf("%s: %v", p, err))
				continue
			}

			if info.IsDir() {
				matches, err := filepath.Glob(filepath.Join(p, "*.yaml"))
				if err != nil {
					loadErrors = append(loadErrors, fmt.Sprintf("glob %s: %v", p, err))
					continue
				}
				if len(matches) == 0 {
					loadErrors = append(loadErrors, fmt.Sprintf("no .yaml files found in %s", p))
					continue
				}
				for _, m := range matches {
					loaded, err := validators.LoadProviderFile(m)
					if err != nil {
						loadErrors = append(loadErrors, err.Error())
						continue
					}
					configs = append(configs, loaded...)
				}
				continue
			}

			loaded, err := validators.LoadProviderFile(p)
			if err != nil {
				loadErrors = append(loadErrors, err.Error())
				continue
			}
			configs = append(configs, loaded...)
		}

		if len(loadErrors) > 0 {
			for _, e := range loadErrors {
				pterm.Error.Println(e)
			}
			if len(configs) == 0 {
				os.Exit(1)
			}
		}

		issues, ok := validators.LintAll(configs)

		if lintJSON {
			out, _ := json.MarshalIndent(struct {
				OK     bool                   `json:"ok"`
				Issues []validators.LintIssue `json:"issues"`
			}{OK: ok, Issues: issues}, "", "  ")
			fmt.Println(string(out))
			if !ok {
				os.Exit(1)
			}
			return
		}

		if len(issues) == 0 {
			pterm.Success.Printf("Linted %d providers, no issues found.\n", len(configs))
			return
		}

		tableData := pterm.TableData{
			{pterm.LightCyan("Severity"), pterm.LightCyan("Provider"), pterm.LightCyan("Field"), pterm.LightCyan("Message")},
		}
		errCount := 0
		for _, i := range issues {
			sev := string(i.Severity)
			if i.Severity == validators.SeverityError {
				sev = pterm.Red(sev)
				errCount++
			} else {
				sev = pterm.Yellow(sev)
			}
			tableData = append(tableData, []string{
				sev,
				i.Provider,
				i.Field,
				i.Message,
			})
		}

		pterm.DefaultSection.Println("Lint results")
		pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
		pterm.Println()
		pterm.Info.Printf("Linted %d providers, %d issue(s) (%d error(s), %d warning(s))\n",
			len(configs), len(issues), errCount, len(issues)-errCount)

		if !ok {
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(lintProvidersCmd)
	lintProvidersCmd.Flags().StringSliceVar(&lintPaths, "path", nil,
		"Path to a provider YAML file or directory of YAML files (repeatable)")
	lintProvidersCmd.Flags().BoolVar(&lintJSON, "json", false,
		"Emit machine-readable JSON output")
}
