package runner

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Grey-Magic/kunji/pkg/models"
	"github.com/Grey-Magic/kunji/pkg/validators"
	"github.com/pterm/pterm"
)

type Runner struct {
	Threads          int
	Proxy            string
	Retries          int
	Timeout          int
	OutFile          string
	ManualProvider   string
	ManualCategory   string
	Resume           bool
	MinKeyLength     int
	Validators       map[string]validators.Validator
	Detector         *validators.Detector
	sharedHTTPClient interface{}
}

func NewRunner(threads int, proxy string, retries int, timeout int, out string, manualProv string, manualCat string, resume bool) (*Runner, error) {
	v, configs, err := validators.InitValidatorsWithConfigs(proxy, timeout)
	if err != nil {
		return nil, err
	}

	return &Runner{
		Threads:        threads,
		Proxy:          proxy,
		Retries:        retries,
		Timeout:        timeout,
		OutFile:        out,
		ManualProvider: strings.ToLower(manualProv),
		ManualCategory: strings.ToLower(manualCat),
		Resume:         resume,
		MinKeyLength:   4,
		Validators:     v,
		Detector:       validators.NewDetectorFromConfigs(configs),
	}, nil
}

func (r *Runner) LoadAndFilterKeys(singleKey, keyFile string) ([]string, error) {
	rawKeys := []string{}

	if singleKey != "" {
		rawKeys = append(rawKeys, singleKey)
	}

	if keyFile != "" {
		file, err := os.Open(keyFile)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			rawKeys = append(rawKeys, scanner.Text())
		}
	}

	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" {
				rawKeys = append(rawKeys, line)
			}
		}
		if err := scanner.Err(); err != nil {
			pterm.Warning.Printfln("Error reading from stdin: %v", err)
		}
	}

	alreadyProcessed := make(map[string]bool)
	if r.Resume {
		alreadyProcessed = r.loadExistingKeys()
		if len(alreadyProcessed) > 0 {
			pterm.Info.Printfln("Resume Checkpoint: Found %d already processed keys in '%s'. Skipping them...", len(alreadyProcessed), r.OutFile)
		}
	}

	uniqueKeys := make(map[string]bool)
	var finalKeys []string

	for _, k := range rawKeys {
		k = strings.TrimSpace(k)

		if len(k) >= r.MinKeyLength && !strings.Contains(k, " ") {

			if alreadyProcessed[k] {
				continue
			}

			if !uniqueKeys[k] {
				uniqueKeys[k] = true
				finalKeys = append(finalKeys, k)
			}
		}
	}

	return finalKeys, nil
}

func (r *Runner) Run(keys []string) {
	pterm.Success.Printfln("Loaded %d deduplicated and well-formatted keys.", len(keys))

	jobs := make(chan string, len(keys))
	results := make(chan *models.ValidationResult, len(keys))
	var wg sync.WaitGroup

	for w := 1; w <= r.Threads; w++ {
		wg.Add(1)
		go r.worker(jobs, results, &wg)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	var closeOnce sync.Once
	closeJobs := func() { closeOnce.Do(func() { close(jobs) }) }

	go func() {
		<-sigCh
		pterm.Warning.Println("\n[!] graceful shutdown initiated. Saving partial results... (Press Ctrl+C again to force exit)")
		closeJobs()
		signal.Stop(sigCh)
	}()

	go func() {
		for _, key := range keys {
			jobs <- key
		}
		closeJobs()
	}()

	startTime := time.Now()

	go func() {
		wg.Wait()
		close(results)
	}()

	validCount := 0

	var p *pterm.ProgressbarPrinter
	pp := pterm.DefaultProgressbar.WithTotal(len(keys)).WithTitle("Validating API Keys")
	p, err := pp.Start()
	if err != nil {
		pterm.Warning.Println("Failed to start progress bar, continuing without it...")
	}

	resultFile, err := r.openResultFile()
	if err != nil {
		pterm.Error.Printfln("Error opening result file: %v", err)
		return
	}
	if resultFile != nil {
		defer resultFile.Close()
	}

	csvWriter := r.getCSVWriter(resultFile)
	shouldWriteHeader := r.shouldWriteHeader()

	if shouldWriteHeader && csvWriter != nil {
		csvWriter.Write([]string{"Key", "Provider", "IsValid", "StatusCode", "ResponseTime", "Balance", "AccountName", "Email", "Error"})
		csvWriter.Flush()
	}

	var allResults []models.ValidationResult
	collectAllResults := r.OutFile == "" || strings.HasSuffix(strings.ToLower(r.OutFile), ".json")
	if collectAllResults {
		allResults = make([]models.ValidationResult, 0, len(keys))
	}

	updateCounter := 0
	for res := range results {
		if res.IsValid {
			validCount++
		}

		updateCounter++
		if p != nil {
			p.Increment()
		}

		if r.OutFile != "" {
			if collectAllResults {
				allResults = append(allResults, *res)
			}
			if !strings.HasSuffix(strings.ToLower(r.OutFile), ".json") {
				r.writeResult(resultFile, csvWriter, res)
			}
		} else {
			allResults = append(allResults, *res)
		}
	}

	if p != nil {
		p.Stop()
	}

	pterm.Println()

	if csvWriter != nil {
		csvWriter.Flush()
	}

	if r.OutFile != "" {
		r.exportResults(allResults)
	} else if len(allResults) > 0 {
		r.displayResultsTable(allResults)
	}

	duration := time.Since(startTime)
	keysPerSec := float64(len(keys)) / duration.Seconds()

	validPercent := 0
	if len(keys) > 0 {
		validPercent = (validCount * 100) / len(keys)
	}

	validColor := pterm.Green
	if validPercent < 50 {
		validColor = pterm.Yellow
	}

	pterm.Println()
	pterm.Println(pterm.Cyan("┌────────────────────────────────────────┐"))
	pterm.Printf(pterm.Cyan("│")+" %-11s %s "+pterm.Cyan("│\n"),
		pterm.LightCyan("Total:"), pterm.Bold.Sprint(len(keys)))
	pterm.Printf(pterm.Cyan("│")+" %-11s %s "+pterm.Cyan("│\n"),
		pterm.LightCyan("Valid:"), validColor(pterm.Bold.Sprint(validCount)))
	pterm.Printf(pterm.Cyan("│")+" %-11s %s "+pterm.Cyan("│\n"),
		pterm.LightCyan("Invalid:"), pterm.Red(pterm.Bold.Sprint(len(keys)-validCount)))
	pterm.Printf(pterm.Cyan("│")+" %-11s %s "+pterm.Cyan("│\n"),
		pterm.LightCyan("Speed:"), pterm.Bold.Sprintf("%.2f/s", keysPerSec))
	pterm.Printf(pterm.Cyan("│")+" %-11s %s "+pterm.Cyan("│\n"),
		pterm.LightCyan("Time:"), pterm.Bold.Sprint(duration.Round(time.Millisecond)))
	pterm.Println(pterm.Cyan("└────────────────────────────────────────┘"))
}

func (r *Runner) worker(jobs <-chan string, results chan<- *models.ValidationResult, wg *sync.WaitGroup) {
	defer wg.Done()

	for key := range jobs {
		var providerName string

		if r.ManualProvider != "" {
			providerName = r.ManualProvider
		} else {
			providerName = r.Detector.DetectProvider(key, r.ManualCategory)
		}

		val, exists := r.Validators[providerName]
		if !exists {
			results <- &models.ValidationResult{
				Key:          key,
				Provider:     providerName,
				IsValid:      false,
				ErrorMessage: "Unknown provider or unsupported key format",
				ResponseTime: 0,
			}
			continue
		}

		var finalRes *models.ValidationResult
		for attempt := 0; attempt <= r.Retries; attempt++ {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(r.Timeout)*time.Second)
			defer cancel()

			res, err := val.Validate(ctx, key)

			if err != nil {
				errStr := err.Error()

				if strings.Contains(errStr, "context deadline exceeded") || strings.Contains(errStr, "timeout") {
					errStr = "Request Timeout"
				} else if strings.Contains(errStr, "no such host") {
					errStr = "DNS Resolution Error"
				} else if strings.Contains(errStr, "connection refused") {
					errStr = "Connection Refused"
				}

				if attempt == r.Retries {
					finalRes = &models.ValidationResult{
						Key:          key,
						Provider:     providerName,
						IsValid:      false,
						ErrorMessage: errStr,
					}
					break
				}
				backoff := time.Duration(1<<attempt) * time.Second
				if backoff > 10*time.Second {
					backoff = 10 * time.Second
				}
				backoff = r.addJitter(backoff)
				time.Sleep(backoff)
				continue
			}

			if res.StatusCode == 429 && attempt < r.Retries {
				backoffSecs := 2
				if res.RetryAfter > 0 {
					backoffSecs = res.RetryAfter
					if backoffSecs > 30 {
						backoffSecs = 30
					}
				}
				backoff := time.Duration(backoffSecs) * time.Second
				backoff = r.addJitter(backoff)

				pterm.Warning.Printfln("Rate limited by %s, backing off for %s...", providerName, backoff.Round(time.Second))
				time.Sleep(backoff)
				attempt++
				continue
			}

			finalRes = res
			break
		}
		results <- finalRes
	}
}

func (r *Runner) openResultFile() (*os.File, error) {
	if r.OutFile == "" {
		return nil, nil
	}

	flag := os.O_APPEND | os.O_CREATE | os.O_WRONLY

	return os.OpenFile(r.OutFile, flag, 0644)
}

func (r *Runner) shouldWriteHeader() bool {
	if r.OutFile == "" {
		return false
	}

	ext := strings.ToLower(r.OutFile)
	if strings.HasSuffix(ext, ".csv") {
		info, err := os.Stat(r.OutFile)
		return err != nil || info.Size() == 0
	}

	return false
}

func (r *Runner) getCSVWriter(f *os.File) *csv.Writer {
	if f == nil {
		return nil
	}

	ext := strings.ToLower(r.OutFile)
	if strings.HasSuffix(ext, ".csv") {
		return csv.NewWriter(f)
	}

	return nil
}

func (r *Runner) writeResult(f *os.File, cw *csv.Writer, res *models.ValidationResult) {
	if f == nil {
		return
	}

	ext := strings.ToLower(r.OutFile)

	if strings.HasSuffix(ext, ".csv") && cw != nil {
		cw.Write([]string{
			res.Key,
			res.Provider,
			fmt.Sprintf("%t", res.IsValid),
			fmt.Sprintf("%d", res.StatusCode),
			fmt.Sprintf("%.2f", res.ResponseTime),
			fmt.Sprintf("%.5f", res.Balance),
			res.AccountName,
			res.Email,
			res.ErrorMessage,
		})
		return
	}

	status := "INVALID"
	if res.IsValid {
		status = "VALID"
		if res.StatusNote != "" {
			status = "VALID (" + res.StatusNote + ")"
		}
	}

	line := fmt.Sprintf("%s | %s | %s", res.Key, res.Provider, status)

	if res.Balance > 0 {
		line += fmt.Sprintf(" | Balance: $%.2f", res.Balance)
	}
	if res.AccountName != "" {
		line += " | Name: " + res.AccountName
	}
	if res.Email != "" {
		line += " | Email: " + res.Email
	}
	if !res.IsValid && res.ErrorMessage != "" {
		errMsg := res.ErrorMessage
		if len(errMsg) > 80 {
			errMsg = errMsg[:77] + "..."
		}
		line += " | Error: " + errMsg
	}

	f.WriteString(line + "\n")
}

func (r *Runner) displayResultsTable(results []models.ValidationResult) {
	pterm.Println()
	pterm.DefaultSection.Println("Validation Results")

	validResults := []models.ValidationResult{}
	invalidResults := []models.ValidationResult{}

	for _, res := range results {
		if res.IsValid {
			validResults = append(validResults, res)
		} else {
			invalidResults = append(invalidResults, res)
		}
	}

	if len(validResults) > 0 {
		pterm.Println(pterm.Green("✅ Valid Keys (" + fmt.Sprintf("%d", len(validResults)) + "):"))
		r.displayValidKeysTable(validResults)
	}

	if len(invalidResults) > 0 {
		pterm.Println(pterm.Red("❌ Invalid Keys (" + fmt.Sprintf("%d", len(invalidResults)) + "):"))
		r.displayInvalidKeysTable(invalidResults)
	}
}

func (r *Runner) displayValidKeysTable(results []models.ValidationResult) {
	tableData := pterm.TableData{
		{"Provider", "Key", "Status", "Response", "Balance", "Account / Email"},
	}

	for _, res := range results {
		balanceStr := "-"
		if res.Balance > 0 {
			balanceStr = pterm.Green(fmt.Sprintf("$%.4f", res.Balance))
		}

		accountStr := "-"
		parts := []string{}
		if res.AccountName != "" {
			parts = append(parts, res.AccountName)
		}
		if res.Email != "" {
			parts = append(parts, res.Email)
		}

		if res.Extra != nil {
			if vip := res.GetExtraString("vip_level"); vip != "" {
				parts = append(parts, "VIP: "+vip)
			}
			if quota := res.GetExtraString("quota"); quota != "" {
				parts = append(parts, "Quota: "+quota)
			}
			if credits := res.GetExtraString("credits"); credits != "" {
				parts = append(parts, "Credits: "+credits)
			}
			if team := res.GetExtraString("team_name"); team != "" {
				parts = append(parts, "Team: "+team)
			}
			if username := res.GetExtraString("username"); username != "" {
				parts = append(parts, "User: "+username)
			}
		}

		if len(parts) > 0 {
			accountStr = strings.Join(parts, "\n")
		}

		keyMasked := res.Key
		if len(keyMasked) > 20 {
			keyMasked = keyMasked[:8] + "..." + keyMasked[len(keyMasked)-8:]
		}

		responseStr := fmt.Sprintf("%.2fs", res.ResponseTime)

		statusStr := "✓ Valid"
		if res.StatusNote != "" {
			statusStr = "⚠ " + res.StatusNote
		}

		tableData = append(tableData, []string{
			pterm.Cyan(res.Provider),
			keyMasked,
			statusStr,
			responseStr,
			balanceStr,
			accountStr,
		})
	}

	pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()

	for _, res := range results {
		hasExtra := false
		if res.Extra != nil {
			extraKeys := []string{}
			for k := range res.Extra {
				if k != "vip_level" && k != "quota" && k != "credits" && k != "team_name" && k != "username" {
					extraKeys = append(extraKeys, k)
				}
			}
			if len(extraKeys) > 0 {
				hasExtra = true
				pterm.Info.Printfln("%s %s:", res.Provider, pterm.Green("Additional Info:"))
				for _, k := range extraKeys {
					pterm.Println("  " + pterm.Yellow(k+":") + " " + res.GetExtraString(k))
				}
			}
		}

		if len(res.ModelAccess) > 0 {
			if !hasExtra {
				pterm.Info.Printfln("%s %s:", res.Provider, pterm.Green("Available Models:"))
			}
			maxShow := 5
			if len(res.ModelAccess) < 5 {
				maxShow = len(res.ModelAccess)
			}
			pterm.Println("  " + pterm.LightCyan(strings.Join(res.ModelAccess[:maxShow], ", ")))
			if len(res.ModelAccess) > 5 {
				pterm.Println("  " + pterm.Gray(fmt.Sprintf("(+%d more)", len(res.ModelAccess)-5)))
			}
		}
	}
}

func (r *Runner) displayInvalidKeysTable(results []models.ValidationResult) {
	tableData := pterm.TableData{
		{"Provider", "Key", "Status", "Error"},
	}

	for _, res := range results {
		keyMasked := res.Key
		if len(keyMasked) > 25 {
			keyMasked = keyMasked[:10] + "..." + keyMasked[len(keyMasked)-10:]
		}

		errMsg := res.ErrorMessage
		if len(errMsg) > 50 {
			errMsg = errMsg[:47] + "..."
		}

		statusStr := "✗ Invalid"
		if res.StatusCode > 0 {
			statusStr = fmt.Sprintf("✗ %d", res.StatusCode)
		}

		tableData = append(tableData, []string{
			pterm.Cyan(res.Provider),
			keyMasked,
			statusStr,
			pterm.Gray(errMsg),
		})
	}

	pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
}

func (r *Runner) exportResults(results []models.ValidationResult) {
	if r.OutFile == "" || len(results) == 0 {
		return
	}

	ext := strings.ToLower(r.OutFile)

	if strings.HasSuffix(ext, ".json") {
		r.exportJSON(results)
		return
	}

	if strings.HasSuffix(ext, ".csv") {
		pterm.Success.Printfln("Results exported to %s (%d entries)", r.OutFile, len(results))
		return
	}

	pterm.Success.Printfln("Results exported to %s (%d entries)", r.OutFile, len(results))
}

func (r *Runner) exportJSON(results []models.ValidationResult) {
	var existing []models.ValidationResult

	info, err := os.Stat(r.OutFile)
	if err == nil && info.Size() > 0 {
		b, err := os.ReadFile(r.OutFile)
		if err == nil {
			if err := json.Unmarshal(b, &existing); err != nil {
				pterm.Warning.Printfln("Error parsing existing JSON: %v", err)
				existing = nil
			}
		}
	}

	existing = append(existing, results...)
	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		pterm.Error.Printfln("Error marshaling JSON: %v", err)
		return
	}

	if err := os.WriteFile(r.OutFile, data, 0644); err != nil {
		pterm.Error.Printfln("Error writing %s: %v", r.OutFile, err)
		return
	}

	pterm.Success.Printfln("Results exported to %s (Total: %d)", r.OutFile, len(existing))
}

func maskKey(k string) string {
	if len(k) < 12 {
		return "******"
	}
	return k[:6] + "....." + k[len(k)-4:]
}

func tolower(s string) string {
	return strings.ToLower(s)
}

func (r *Runner) loadExistingKeys() map[string]bool {
	existing := make(map[string]bool)
	if r.OutFile == "" {
		return existing
	}

	file, err := os.Open(r.OutFile)
	if err != nil {
		return existing
	}
	defer file.Close()

	ext := tolower(r.OutFile)

	if strings.HasSuffix(ext, ".json") {
		var results []models.ValidationResult
		if err := json.NewDecoder(file).Decode(&results); err != nil {
			pterm.Warning.Printfln("Error parsing resume file: %v", err)
		} else {
			for _, res := range results {
				existing[res.Key] = true
			}
		}
		return existing
	}

	if strings.HasSuffix(ext, ".csv") {
		reader := csv.NewReader(file)

		_, err := reader.Read()
		if err != nil {
			return existing
		}

		records, err := reader.ReadAll()
		if err != nil {
			pterm.Warning.Printfln("Error reading CSV: %v", err)
			return existing
		}

		for _, row := range records {
			if len(row) > 0 {
				existing[row[0]] = true
			}
		}
		return existing
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			existing[line] = true
		}
	}

	if err := scanner.Err(); err != nil {
		pterm.Warning.Printfln("Error reading resume file: %v", err)
	}

	return existing
}

func (r *Runner) addJitter(d time.Duration) time.Duration {
	maxJitterMs := d.Milliseconds() / 4
	if maxJitterMs < 100 {
		maxJitterMs = 100
	}
	jitter, _ := rand.Int(rand.Reader, big.NewInt(maxJitterMs))
	return d + time.Duration(jitter.Int64())*time.Millisecond
}
