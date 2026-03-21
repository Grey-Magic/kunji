package cmd

import (
	"fmt"
	"os"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "kunji",
	Version: "1.0.6",
	Short:   "A fast, concurrent CLI tool for validating API keys.",
	Long: `Kunji is a high-performance command-line utility written in Go.
It rapidly tests API keys from various services and providers
by utilizing concurrent worker pools, proxy rotation, and smart auto-detection.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func PrintBanner() {
	lines := []string{
		"  ██   ██ ██    ██ ███    ██      ██ ██",
		"  ██  ██  ██    ██ ████   ██      ██ ██",
		"  █████   ██    ██ ██ ██  ██      ██ ██",
		"  ██  ██  ██    ██ ██  ██ ██ ██   ██ ██",
		"  ██   ██  ██████  ██   ████  █████  ██",
	}

	colors := []pterm.RGB{
		{R: 95, G: 0, B: 135},
		{R: 135, G: 45, B: 175},
		{R: 155, G: 70, B: 195},
		{R: 175, G: 95, B: 215},
		{R: 195, G: 120, B: 235},
	}

	for i, line := range lines {
		pterm.RGB{R: colors[i].R, G: colors[i].G, B: colors[i].B}.Println(line)
	}

	pterm.DefaultCenter.Println(pterm.LightMagenta("Universal API Key Validation Engine"))
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
