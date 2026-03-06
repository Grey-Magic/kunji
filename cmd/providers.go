package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Grey-Magic/kunji/pkg/validators"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var providersCmd = &cobra.Command{
	Use:   "providers",
	Short: "List and explore providers",
	Long:  `List all supported providers, categories, or services for a specific provider.`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) > 0 {
			showProviderServices(args[0])
		} else {
			listAllProviders()
		}
	},
}

var listCategoriesCmd = &cobra.Command{
	Use:   "categories",
	Short: "List all provider categories",
	Run: func(cmd *cobra.Command, args []string) {
		listCategories()
	},
}

var listLLMCmd = &cobra.Command{
	Use:   "llm",
	Short: "List all LLM providers",
	Run: func(cmd *cobra.Command, args []string) {
		listProvidersByCategory("llm")
	},
}

var listCommonCmd = &cobra.Command{
	Use:   "common",
	Short: "List all common service providers",
	Run: func(cmd *cobra.Command, args []string) {
		listProvidersByCategory("common")
	},
}

var listSecurityCmd = &cobra.Command{
	Use:   "security",
	Short: "List all security providers",
	Run: func(cmd *cobra.Command, args []string) {
		listProvidersByCategory("security")
	},
}

var providerName string

var showProviderCmd = &cobra.Command{
	Use:   "google",
	Short: "Show all services for a provider (e.g., google, github, mapbox)",
	Run: func(cmd *cobra.Command, args []string) {
		showProviderServices(providerName)
	},
}

func init() {
	rootCmd.AddCommand(providersCmd)
	providersCmd.AddCommand(listCategoriesCmd)
	providersCmd.AddCommand(listLLMCmd)
	providersCmd.AddCommand(listCommonCmd)
	providersCmd.AddCommand(listSecurityCmd)
}

func listAllProviders() {
	providers, err := validators.GetAllProviders()
	if err != nil {
		pterm.Error.Printfln("Error loading providers: %v", err)
		return
	}

	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Name < providers[j].Name
	})

	pterm.DefaultSection.Println("Supported Providers")
	pterm.Println(pterm.Gray(fmt.Sprintf("Total: %d providers\n", len(providers))))

	for _, p := range providers {
		prefixes := ""
		if len(p.KeyPrefixes) > 0 {
			prefixes = p.KeyPrefixes[0]
			if len(p.KeyPrefixes) > 1 {
				prefixes += fmt.Sprintf(" (+%d)", len(p.KeyPrefixes)-1)
			}
		}
		pterm.Printf("  %s %s %s\n", pterm.LightCyan(p.Name), pterm.Gray("["+p.Category+"]"), pterm.Gray(prefixes))
	}
}

func listCategories() {
	categories, err := validators.GetCategories()
	if err != nil {
		pterm.Error.Printfln("Error loading categories: %v", err)
		return
	}

	pterm.DefaultSection.Println("Provider Categories")
	pterm.Println()

	for _, cat := range categories {
		count := 0
		providers, _ := validators.GetProvidersByCategory(cat)
		count = len(providers)

		pterm.Printf("  %s %s (%d providers)\n", pterm.Cyan("•"), pterm.Bold.Sprint(cat), count)
	}
	pterm.Println()
	pterm.Info.Println("Use: kunji providers <category> to list providers in a category")
	pterm.Info.Println("Example: kunji providers llm")
}

func listProvidersByCategory(category string) {
	providers, err := validators.GetProvidersByCategory(category)
	if err != nil {
		pterm.Error.Printfln("Error loading providers: %v", err)
		return
	}

	if len(providers) == 0 {
		pterm.Warning.Printfln("No providers found for category: %s", category)
		return
	}

	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Name < providers[j].Name
	})

	pterm.DefaultSection.Println("Category: " + strings.ToUpper(category))
	pterm.Println(pterm.Gray(fmt.Sprintf("Total: %d providers\n", len(providers))))

	for _, p := range providers {
		prefixes := ""
		if len(p.KeyPrefixes) > 0 {
			prefixes = p.KeyPrefixes[0]
			if len(p.KeyPrefixes) > 1 {
				prefixes += fmt.Sprintf(" (+%d)", len(p.KeyPrefixes)-1)
			}
		}
		pterm.Printf("  %s %s\n", pterm.LightCyan(p.Name), pterm.Gray(prefixes))
	}
}

func showProviderServices(name string) {

	providers, err := validators.FindProviderByName(name)
	if err != nil {
		pterm.Error.Printfln("Error loading providers: %v", err)
		return
	}

	if len(providers) == 0 {
		pterm.Warning.Printfln("No services found for: %s", name)
		pterm.Info.Println("Try: google, github, mapbox, newrelic, etc.")
		return
	}

	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Name < providers[j].Name
	})

	pterm.DefaultSection.Println("Services for: " + strings.ToUpper(name))
	pterm.Println(pterm.Gray(fmt.Sprintf("Total: %d services\n", len(providers))))

	for _, p := range providers {
		pterm.Printf("  %s\n", pterm.LightCyan(p.Name))
		pterm.Printf("    %s\n", pterm.Gray("Category: "+p.Category))

		if len(p.KeyPrefixes) > 0 {
			pterm.Printf("    %s\n", pterm.Gray("Prefix: "+strings.Join(p.KeyPrefixes, ", ")))
		}
		pterm.Println()
	}

	pterm.Info.Printfln("Validate with: kunji validate -k \"YOUR_KEY\" -p %s", providers[0].Name)
}
