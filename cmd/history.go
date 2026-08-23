package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Grey-Magic/kunji/pkg/history"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var (
	historyFile    string
	historyKey     string
	historyLimit   int
	historyOnlyFli bool
)

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Inspect cross-run validation history",
	Long: `Read the cross-run history JSONL and display per-key records.

By default prints a summary for every distinct key seen in the history file,
ordered by latest observation. Pass --key <sha256-prefix> to drill into a
single key's full timeline.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		path := historyFile
		if path == "" {
			path = history.DefaultPath()
		}

		records, err := history.ReadAll(path)
		if err != nil {
			pterm.Error.Printfln("history: %v", err)
			return
		}
		if len(records) == 0 {
			pterm.Info.Println("No history records found at " + path)
			return
		}

		if historyKey != "" {
			showKeyTimeline(records, historyKey)
			return
		}

		showSummary(records)
	},
}

func showSummary(records []history.Record) {
	sums := history.Summarize(records)
	if historyLimit > 0 && len(sums) > historyLimit {
		sums = sums[:historyLimit]
	}

	pterm.DefaultSection.Printfln("Validation history (%d distinct keys)", len(sums))
	table := pterm.TableData{
		{pterm.LightCyan("Provider"), pterm.LightCyan("Key SHA256"), pterm.LightCyan("Latest"), pterm.LightCyan("Valid"), pterm.LightCyan("Streak"), pterm.LightCyan("Runs")},
	}
	for _, s := range sums {
		valid := pterm.Red("invalid")
		if s.Latest.IsValid {
			valid = pterm.Green("valid")
		}
		streak := fmt.Sprintf("%d", s.ValidStreak)
		if s.ValidStreak == s.TotalRuns {
			streak = pterm.Green(streak)
		}
		table = append(table, []string{
			s.Provider,
			truncateHash(s.KeySHA256),
			s.Latest.Timestamp,
			valid,
			streak,
			fmt.Sprintf("%d/%d", s.ValidRuns, s.TotalRuns),
		})
	}
	pterm.DefaultTable.WithHasHeader().WithData(table).Render()
}

func showKeyTimeline(records []history.Record, keyPrefix string) {
	sums := history.Summarize(records)
	needle := strings.ToLower(keyPrefix)

	matched := 0
	for _, s := range sums {
		if strings.HasPrefix(strings.ToLower(s.KeySHA256), needle) {
			matched++
			pterm.DefaultSection.Printfln("Key: %s (provider: %s)", s.KeySHA256, s.Provider)
			table := pterm.TableData{
				{pterm.LightCyan("Timestamp"), pterm.LightCyan("Run ID"), pterm.LightCyan("Status"), pterm.LightCyan("HTTP"), pterm.LightCyan("Latency")},
			}
			for _, r := range s.History {
				status := pterm.Green("valid")
				if !r.IsValid {
					status = pterm.Red("invalid")
				}
				latency := "-"
				if r.DurationMs > 0 {
					latency = strconv.FormatInt(r.DurationMs, 10) + "ms"
				}
				table = append(table, []string{
					r.Timestamp,
					r.RunID,
					status,
					strconv.Itoa(r.StatusCode),
					latency,
				})
			}
			pterm.DefaultTable.WithHasHeader().WithData(table).Render()
			pterm.Info.Printfln("Streak: %d valid run(s), %d total",
				s.ValidStreak, s.TotalRuns)
		}
	}
	if matched == 0 {
		pterm.Warning.Printfln("No history records match key prefix %q", keyPrefix)
	}
}

func truncateHash(h string) string {
	if len(h) <= 12 {
		return h
	}
	return h[:8] + "…" + h[len(h)-4:]
}

func init() {
	rootCmd.AddCommand(historyCmd)
	historyCmd.Flags().StringVar(&historyFile, "file", "",
		"History file (default ~/.kunji/history.jsonl)")
	historyCmd.Flags().StringVar(&historyKey, "key", "",
		"Show full timeline for one key (SHA-256 prefix match)")
	historyCmd.Flags().IntVar(&historyLimit, "limit", 50,
		"Limit summary rows to N keys (0 = no limit)")
	historyCmd.Flags().BoolVar(&historyOnlyFli, "flipped", false,
		"Show only keys whose latest result flipped from their previous run")
}
