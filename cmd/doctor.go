package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Grey-Magic/kunji/pkg/audit"
	"github.com/Grey-Magic/kunji/pkg/client"
	"github.com/Grey-Magic/kunji/pkg/history"
	"github.com/Grey-Magic/kunji/pkg/models"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose the local Kunji environment",
	Long: `Run a battery of diagnostics so you can see whether Kunji is set up
correctly before running a real validate. Checks include:

  * Provider config load (counts embedded YAML files)
  * DNS resolution against known-good provider hosts
  * Write permissions for ~/.kunji/, the audit log, and the history log
  * Persistent cache file round-trip (write + read)
  * Proxy health (when --proxy is set)
  * Synthetic key probe (one unauthenticated request to api.openai.com)

Exit code is 0 when every check passes, 1 when any check fails.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		results := runDoctor()
		printDoctorReport(results)

		for _, r := range results {
			if r.Status == doctorFail {
				os.Exit(1)
			}
		}
	},
}

type doctorStatus string

const (
	doctorPass doctorStatus = "PASS"
	doctorWarn doctorStatus = "WARN"
	doctorFail doctorStatus = "FAIL"
)

type doctorCheck struct {
	Name    string
	Status  doctorStatus
	Message string
}

func runDoctor() []doctorCheck {
	results := []doctorCheck{}
	results = append(results, checkProviderConfigs())
	results = append(results, checkDNS())
	results = append(results, checkKunjiDir())
	results = append(results, checkDirForLog(audit.DefaultPath(), "Audit log directory"))
	results = append(results, checkDirForLog(history.DefaultPath(), "History log directory"))
	results = append(results, checkPersistentCache())
	if proxy != "" {
		results = append(results, checkProxy(proxy))
	}
	results = append(results, checkSyntheticProbe())
	return results
}

func checkProviderConfigs() doctorCheck {
	configs, err := os.ReadDir(filepath.Join("pkg", "validators", "providers"))
	if err != nil {
		// The cwd may not be the project root (e.g. installed via go install).
		// Don't fail on this; just warn.
		return doctorCheck{Name: "Provider configs", Status: doctorWarn, Message: "could not enumerate providers dir"}
	}
	yamlCount := 0
	for _, e := range configs {
		if strings.HasSuffix(e.Name(), ".yaml") {
			yamlCount++
		}
	}
	return doctorCheck{
		Name:    "Provider configs",
		Status:  doctorPass,
		Message: fmt.Sprintf("%d embedded YAML file(s) found", yamlCount),
	}
}

func checkDNS() doctorCheck {
	domains := []string{
		"api.openai.com",
		"api.github.com",
		"api.anthropic.com",
		"api.deepseek.com",
		"generativelanguage.googleapis.com",
	}
	resolver := &net.Resolver{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var failed []string
	for _, d := range domains {
		if _, err := resolver.LookupHost(ctx, d); err != nil {
			failed = append(failed, d)
		}
	}

	if len(failed) == 0 {
		return doctorCheck{
			Name:    "DNS resolution",
			Status:  doctorPass,
			Message: fmt.Sprintf("resolved %d/%d provider hosts", len(domains), len(domains)),
		}
	}
	return doctorCheck{
		Name:    "DNS resolution",
		Status:  doctorFail,
		Message: fmt.Sprintf("failed to resolve: %s", strings.Join(failed, ", ")),
	}
}

func checkKunjiDir() doctorCheck {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return doctorCheck{Name: "Kunji directory", Status: doctorWarn, Message: "no home directory available"}
	}
	return checkDirForLog(filepath.Join(home, ".kunji", "doctor-probe.tmp"), "Kunji directory")
}

func checkDirForLog(path, name string) doctorCheck {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return doctorCheck{
			Name:    name,
			Status:  doctorFail,
			Message: fmt.Sprintf("cannot create %s: %v", dir, err),
		}
	}
	probe := filepath.Join(dir, ".doctor_probe")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return doctorCheck{
			Name:    name,
			Status:  doctorFail,
			Message: fmt.Sprintf("cannot write to %s: %v", dir, err),
		}
	}
	_ = os.Remove(probe)
	return doctorCheck{
		Name:    name,
		Status:  doctorPass,
		Message: fmt.Sprintf("%s writable", dir),
	}
}

func checkPersistentCache() doctorCheck {
	tmp, err := os.CreateTemp("", "kunji-doctor-cache-*.jsonl")
	if err != nil {
		return doctorCheck{Name: "Persistent cache", Status: doctorFail, Message: fmt.Sprintf("temp file: %v", err)}
	}
	path := tmp.Name()
	tmp.Close()
	defer os.Remove(path)

	const provider = "doctor"
	key := fmt.Sprintf("probe-%d", time.Now().UnixNano())

	pc, err := client.NewPersistentCache(path, time.Hour)
	if err != nil {
		return doctorCheck{Name: "Persistent cache", Status: doctorFail, Message: fmt.Sprintf("open: %v", err)}
	}

	probe := &models.ValidationResult{
		Provider:   provider,
		Key:        key,
		IsValid:    true,
		StatusCode: 200,
	}

	if _, ok := pc.Get(provider, key); ok {
		return doctorCheck{Name: "Persistent cache", Status: doctorFail, Message: "Get returned a hit before Set"}
	}

	pc.SetWithTTL(provider, key, probe, 5*time.Minute)
	if _, ok := pc.Get(provider, key); !ok {
		return doctorCheck{Name: "Persistent cache", Status: doctorFail, Message: "Get missed immediately after Set"}
	}

	pc2, err := client.NewPersistentCache(path, time.Hour)
	if err != nil {
		return doctorCheck{Name: "Persistent cache", Status: doctorFail, Message: fmt.Sprintf("reopen: %v", err)}
	}
	if _, ok := pc2.Get(provider, key); !ok {
		return doctorCheck{Name: "Persistent cache", Status: doctorFail, Message: "round-trip across instances failed"}
	}

	return doctorCheck{
		Name:    "Persistent cache",
		Status:  doctorPass,
		Message: fmt.Sprintf("round-trip OK at %s", path),
	}
}

func checkProxy(proxyInput string) doctorCheck {
	rotator, err := client.NewProxyRotator(proxyInput)
	if err != nil {
		return doctorCheck{
			Name:    "Proxy health",
			Status:  doctorFail,
			Message: fmt.Sprintf("cannot parse proxy: %v", err),
		}
	}
	// FilterDeadProxies requires a working HTTP client to probe each proxy.
	// Construct one and surface a degraded answer if we can't reach the
	// probe target.
	count := countProxies(rotator)
	if count == 0 {
		return doctorCheck{
			Name:    "Proxy health",
			Status:  doctorWarn,
			Message: "no proxies configured",
		}
	}

	// Try to reach the probe target through one of the proxies.
	pxy, _ := rotator.GetProxy(nil)
	if pxy == nil {
		return doctorCheck{
			Name:    "Proxy health",
			Status:  doctorWarn,
			Message: fmt.Sprintf("%d proxy/ies parsed, but rotator returned no proxy", count),
		}
	}

	tr := &http.Transport{
		Proxy: http.ProxyURL(pxy),
	}
	c := &http.Client{Transport: tr, Timeout: 5 * time.Second}
	resp, err := c.Get("https://api.ipify.org?format=json")
	if err != nil {
		return doctorCheck{
			Name:    "Proxy health",
			Status:  doctorFail,
			Message: fmt.Sprintf("proxy %s unreachable: %v", pxy.String(), err),
		}
	}
	resp.Body.Close()
	return doctorCheck{
		Name:    "Proxy health",
		Status:  doctorPass,
		Message: fmt.Sprintf("%s responded", pxy.String()),
	}
}

// countProxies parses the proxy input string and counts entries without
// depending on ProxyRotator's unexported proxies field. Best-effort.
func countProxies(_ *client.ProxyRotator) int {
	if proxy == "" {
		return 0
	}
	if strings.Contains(proxy, "\n") {
		return strings.Count(proxy, "\n") + 1
	}
	return 1
}

func checkSyntheticProbe() doctorCheck {
	c := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("GET", "https://api.openai.com/v1/models", nil)
	if err != nil {
		return doctorCheck{Name: "Synthetic probe", Status: doctorFail, Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer sk-doctor-probe-do-not-use")
	resp, err := c.Do(req)
	if err != nil {
		return doctorCheck{
			Name:    "Synthetic probe",
			Status:  doctorWarn,
			Message: fmt.Sprintf("network unreachable: %v", err),
		}
	}
	defer resp.Body.Close()

	// We expect 401 (unauthorized) since the key is bogus. Anything else is
	// suspicious and worth surfacing.
	switch resp.StatusCode {
	case 401, 403:
		return doctorCheck{
			Name:    "Synthetic probe",
			Status:  doctorPass,
			Message: fmt.Sprintf("api.openai.com responded %d (expected for invalid key)", resp.StatusCode),
		}
	default:
		return doctorCheck{
			Name:    "Synthetic probe",
			Status:  doctorWarn,
			Message: fmt.Sprintf("api.openai.com responded %d (unexpected for invalid key)", resp.StatusCode),
		}
	}
}

func printDoctorReport(results []doctorCheck) {
	pterm.DefaultSection.Println("Kunji doctor")
	table := pterm.TableData{
		{pterm.LightCyan("Status"), pterm.LightCyan("Check"), pterm.LightCyan("Detail")},
	}
	for _, r := range results {
		cell := string(r.Status)
		switch r.Status {
		case doctorPass:
			cell = pterm.Green(cell)
		case doctorWarn:
			cell = pterm.Yellow(cell)
		case doctorFail:
			cell = pterm.Red(cell)
		}
		table = append(table, []string{cell, r.Name, r.Message})
	}
	pterm.DefaultTable.WithHasHeader().WithData(table).Render()

	failed, warned := 0, 0
	for _, r := range results {
		switch r.Status {
		case doctorFail:
			failed++
		case doctorWarn:
			warned++
		}
	}
	pterm.Println()
	switch {
	case failed > 0:
		pterm.Error.Printfln("%d check(s) failed, %d warned", failed, warned)
	case warned > 0:
		pterm.Warning.Printfln("All checks passed; %d warning(s)", warned)
	default:
		pterm.Success.Println("All checks passed.")
	}
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
