package cmd

import (
	"fmt"
	"os"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var diffA string
var diffB string

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Compare two validation result files and report changes",
	Long: `Reads two JSONL result files produced by 'kunji validate --format jsonl' and
prints three groups: keys whose validity changed, keys only in A, and keys
only in B. Comparison is done by SHA-256 hash of the key, so secrets are not
printed. Use --quiet to suppress the per-group listings and only see counts.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if diffA == "" || diffB == "" {
			fmt.Fprintln(os.Stderr, "diff: both --a and --b are required")
			os.Exit(1)
		}

		a, _, err := loadValidationResults(diffA)
		if err != nil {
			fmt.Fprintln(os.Stderr, "diff: read A:", err)
			os.Exit(1)
		}
		b, _, err := loadValidationResults(diffB)
		if err != nil {
			fmt.Fprintln(os.Stderr, "diff: read B:", err)
			os.Exit(1)
		}

		changed, onlyA, onlyB := diffResults(a, b)

		fmt.Fprintf(os.Stderr,
			"diff: %d changed, %d only-in-A, %d only-in-B (A size=%d, B size=%d)\n",
			len(changed), len(onlyA), len(onlyB), len(a), len(b))

		if quiet {
			return
		}

		if len(changed) > 0 {
			pterm.DefaultSection.Println("Changed (key_hash: A -> B)")
			for _, id := range changed {
				fmt.Printf("  %s : %s -> %s\n", id, a[id], b[id])
			}
		}
		if len(onlyA) > 0 {
			pterm.DefaultSection.Println("Only in A")
			for _, id := range onlyA {
				fmt.Println("  " + id)
			}
		}
		if len(onlyB) > 0 {
			pterm.DefaultSection.Println("Only in B")
			for _, id := range onlyB {
				fmt.Println("  " + id)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(diffCmd)
	diffCmd.Flags().StringVar(&diffA, "a", "", "First result file (JSONL). Use '-' for stdin.")
	diffCmd.Flags().StringVar(&diffB, "b", "", "Second result file (JSONL). Use '-' for stdin.")
	diffCmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Print only counts, no per-key listings")
}
