package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var dedupeOut string
var dedupeIn string

var dedupeCmd = &cobra.Command{
	Use:   "dedupe",
	Short: "Remove duplicate keys from a key file",
	Long: `Reads a newline-separated key file (or stdin) and emits the de-duplicated
list to stdout or --out. Prints a summary of how many duplicates were removed
to stderr.

Blank lines and lines starting with '#' are treated as comments and skipped.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		keys, err := loadKeys(dedupeIn)
		if err != nil {
			fmt.Fprintln(os.Stderr, "dedupe: read input:", err)
			os.Exit(1)
		}
		unique, dups := dedupeKeys(keys)

		w := os.Stdout
		if dedupeOut != "" && dedupeOut != "-" {
			f, err := os.Create(dedupeOut)
			if err != nil {
				fmt.Fprintln(os.Stderr, "dedupe: create output:", err)
				os.Exit(1)
			}
			defer f.Close()
			w = f
		}
		for _, k := range unique {
			fmt.Fprintln(w, k)
		}

		fmt.Fprintf(os.Stderr, "dedupe: %d unique, %d duplicate(s) removed, %d total input\n",
			len(unique), dups, len(keys))
	},
}

func init() {
	rootCmd.AddCommand(dedupeCmd)
	dedupeCmd.Flags().StringVarP(&dedupeIn, "in", "i", "-", "Input key file ('-' for stdin)")
	dedupeCmd.Flags().StringVarP(&dedupeOut, "out", "o", "-", "Output file for unique keys ('-' for stdout)")
}
