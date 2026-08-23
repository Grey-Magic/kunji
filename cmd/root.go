package cmd

import (
	"fmt"
	"os"

	"github.com/Grey-Magic/kunji/pkg/config"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// profileFlag holds the value of --profile from the command line. It is
// applied to flag defaults BEFORE parsing so the right profile takes effect
// even for subcommand flags like --threads.
var profileFlag string

var rootCmd = &cobra.Command{
	Use:     "kunji",
	Version: "1.2.0",
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

	rootCmd.PersistentFlags().StringVar(&profileFlag, "profile", "",
		"Configuration profile to apply (overrides default_profile / KUNJI_PROFILE)")
	rootCmd.PersistentFlags().String("config", "",
		"Path to config file (overrides ~/.kunji/config.yaml and KUNJI_CONFIG)")
}

func Execute() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(1)
	}

	// --config flag takes priority over KUNJI_CONFIG / default path. If the
	// user passed --config explicitly, reload from that path so the values
	// they specified are what we apply.
	if cfgPath, _ := rootCmd.PersistentFlags().GetString("config"); cfgPath != "" {
		if c, err := config.LoadFrom(cfgPath); err == nil {
			cfg = c
		} else {
			fmt.Fprintln(os.Stderr, "config error:", err)
			os.Exit(1)
		}
	}

	cfg.Apply(rootCmd, profileFlag)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
