package cmd

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Grey-Magic/kunji/pkg/client"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var proxyFile string
var proxyTimeout int

var checkProxiesCmd = &cobra.Command{
	Use:   "check-proxies",
	Short: "Check the health of a list of proxies",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		PrintBanner()

		if proxyTimeout < 1 || proxyTimeout > 60 {
			pterm.Error.Printfln("Error: timeout must be between 1 and 60 seconds (got %d)", proxyTimeout)
			os.Exit(1)
		}

		if proxyFile == "" {
			pterm.Error.Println("Please provide a proxy file using --proxy")
			return
		}

		var proxies []string
		file, err := os.Open(proxyFile)
		if err == nil {
			defer file.Close()
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line != "" {
					proxies = append(proxies, line)
				}
			}
		} else {
			proxies = append(proxies, proxyFile)
		}

		if len(proxies) == 0 {
			pterm.Error.Println("No proxies loaded")
			return
		}

		pterm.Info.Printfln("Testing %d proxies...", len(proxies))

		var wg sync.WaitGroup
		results := make(chan string, len(proxies))

		validCount := 0

		p, _ := pterm.DefaultProgressbar.WithTotal(len(proxies)).WithTitle("Checking Proxies").Start()

		for _, pxy := range proxies {
			wg.Add(1)
			go func(pxyStr string) {
				defer wg.Done()

				_, err := url.Parse(pxyStr)
				if err != nil {
					if !strings.HasPrefix(pxyStr, "http") && !strings.HasPrefix(pxyStr, "socks5") {
						_, err = url.Parse("http://" + pxyStr)
					}
				}

				if err != nil {
					results <- fmt.Sprintf("✗ %s (Invalid URL)", pxyStr)
					pterm.DefaultProgressbar.Increment()
					return
				}

				httpClient, _, _ := client.NewHTTPClient(pxyStr, proxyTimeout)

				ctx, cancel := context.WithTimeout(context.Background(), time.Duration(proxyTimeout)*time.Second)
				defer cancel()

				req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.ipify.org?format=json", nil)
				req.Header.Set("User-Agent", client.GetRandomUserAgent())

				start := time.Now()
				resp, err := httpClient.Do(req)
				duration := time.Since(start)

				if err == nil && resp.StatusCode == 200 {
					results <- fmt.Sprintf("✓ %s (%.2fs)", pxyStr, duration.Seconds())
				} else {
					errMsg := "Failed"
					if err != nil {
						errMsg = err.Error()
					} else {
						errMsg = fmt.Sprintf("Status %d", resp.StatusCode)
					}

					if len(errMsg) > 30 {
						errMsg = errMsg[:27] + "..."
					}
					results <- fmt.Sprintf("✗ %s (%s)", pxyStr, errMsg)
				}
				p.Increment()
			}(pxy)
		}

		go func() {
			wg.Wait()
			close(results)
		}()

		var validProxies []string
		for res := range results {
			if strings.HasPrefix(res, "✓") {
				validCount++
				validProxies = append(validProxies, res)
			}
		}

		pterm.Println()
		pterm.DefaultSection.Println("Proxy Check Results")
		pterm.Info.Printfln("Valid: %d / %d", validCount, len(proxies))

		if len(validProxies) > 0 {
			pterm.Println()
			pterm.Success.Println("Valid Proxies:")
			for _, v := range validProxies {
				pterm.Println(v)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(checkProxiesCmd)
	checkProxiesCmd.Flags().StringVar(&proxyFile, "proxy", "", "Proxy string or path to proxy file")
	checkProxiesCmd.Flags().IntVar(&proxyTimeout, "timeout", 10, "Timeout in seconds per test")
}
