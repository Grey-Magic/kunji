package cmd

import (
	"fmt"
	"os"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "kunji",
	Version: "1.0.0",
	Short:   "A fast, concurrent CLI tool for validating API keys.",
	Long: `Kunji is a high-performance command-line utility written in Go.
It rapidly tests API keys from various services and providers
by utilizing concurrent worker pools, proxy rotation, and smart auto-detection.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func PrintBanner() {
	pterm.Println(pterm.LightMagenta("  ██   ██ ██    ██ ███    ██      ██ ██"))
	pterm.Println(pterm.Magenta("  ██  ██  ██    ██ ████   ██      ██ ██"))
	pterm.Println(pterm.Cyan("  █████   ██    ██ ██ ██  ██      ██ ██"))
	pterm.Println(pterm.LightCyan("  ██  ██  ██    ██ ██  ██ ██      ██ ██"))
	pterm.Println(pterm.Blue("  ██   ██ ██████  ██   ████  █████  ██"))

	pterm.DefaultCenter.Println(pterm.LightCyan("Universal API Key Validation Engine"))
	pterm.Println()
}

func init() {
	originalHelp := rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		PrintBanner()
		originalHelp(cmd, args)
	})
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
