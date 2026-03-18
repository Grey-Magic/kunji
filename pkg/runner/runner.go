package runner

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Grey-Magic/kunji/pkg/models"
	"github.com/Grey-Magic/kunji/pkg/utils"
	"github.com/Grey-Magic/kunji/pkg/validators"
	"github.com/pterm/pterm"
)

type Runner struct {
	Threads          int
	adaptiveThreads  int
	threadMux        sync.RWMutex
	Proxy            string
	Retries          int
	Timeout          int
	OutFile          string
	ManualProvider   string
	ManualCategory   string
	Resume           bool
	MinKeyLength     int
	OnlyValid        bool
	MinBalance       float64
	DeepScan         bool
	Password         string
	Bench            bool
	Validators       map[string]validators.Validator
	Detector         *validators.Detector
	sharedHTTPClient interface{}
}

func NewRunner(threads int, proxy string, retries int, timeout int, out string, manualProv string, manualCat string, resume bool, onlyValid bool, minBalance float64) (*Runner, error) {
	v, configs, err := validators.InitValidatorsWithConfigs(proxy, timeout)
	if err != nil {
		return nil, err
	}

	return &Runner{
		Threads:         threads,
		adaptiveThreads: threads,
		Proxy:           proxy,
		Retries:         retries,
		Timeout:         timeout,
		OutFile:         out,
		ManualProvider:  strings.ToLower(manualProv),
		ManualCategory:  strings.ToLower(manualCat),
		Resume:          resume,
		MinKeyLength:    4,
		OnlyValid:       onlyValid,
		MinBalance:      minBalance,
		Validators:      v,
		Detector:        validators.NewDetectorFromConfigs(configs),
	}, nil
}

func (r *Runner) LoadAndFilterKeys(singleKey, keyFile string) ([]string, error) {
	spinner, _ := pterm.DefaultSpinner.Start("Loading and filtering keys...")
	rawKeys := make([]string, 0)

	if singleKey != "" {
		rawKeys = append(rawKeys, singleKey)
	}

	if keyFile != "" {
		file, err := os.Open(keyFile)
		if err != nil {
			spinner.Fail("Failed to open keys file")
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

		if len(k) >= r.MinKeyLength {
			h := r.hashKey(k)
			if alreadyProcessed[h] {
				continue
			}

			if !uniqueKeys[k] {
				uniqueKeys[k] = true
				finalKeys = append(finalKeys, k)
			}
		}
	}

	spinner.Success(fmt.Sprintf("Loaded %d unique keys.", len(finalKeys)))
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
		if err := csvWriter.Write([]string{"Key", "Provider", "IsValid", "StatusCode", "ResponseTime", "Balance", "AccountName", "Email", "Error"}); err != nil {
			pterm.Warning.Printfln("Failed to write CSV header: %v", err)
		}
		csvWriter.Flush()
	}

	var allResults []models.ValidationResult
	collectAllResults := r.OutFile == "" || strings.HasSuffix(strings.ToLower(r.OutFile), ".json")
	if collectAllResults {
		allResults = make([]models.ValidationResult, 0, len(keys))
	}

	updateCounter := 0
	updateCounterMutex := sync.Mutex{}
	var progressTicker *time.Ticker
	if p != nil {
		progressTicker = time.NewTicker(100 * time.Millisecond)
		defer progressTicker.Stop()
	}

	area, _ := pterm.DefaultArea.Start()
	defer area.Stop()

	lastResults := make([]*models.ValidationResult, 0, 5)

	for {
		select {
		case <-progressTicker.C:
			updateCounterMutex.Lock()
			if p != nil && updateCounter > 0 {
				current := p.Current
				if current < len(keys) {
					p.Increment()
				}
				updateCounter = 0
			}

			var areaText string
			for i := len(lastResults) - 1; i >= 0; i-- {
				res := lastResults[i]
				status := pterm.Red("✗ Invalid")
				if res.IsValid {
					status = pterm.Green("✓ Valid")
				}
				keyMasked := maskKey(res.Key)
				areaText += fmt.Sprintf("  %s %-15s %s %s\n", pterm.Gray("»"), pterm.LightCyan(res.Provider), status, pterm.Gray(keyMasked))
			}
			area.Update(areaText)
			updateCounterMutex.Unlock()

		case res, ok := <-results:
			if !ok {
				updateCounterMutex.Lock()
				if p != nil && updateCounter > 0 {
					p.Add(updateCounter)
				}
				updateCounterMutex.Unlock()
				goto done
			}
			if res.IsValid {
				validCount++
			}

			updateCounterMutex.Lock()
			updateCounter++

			lastResults = append(lastResults, res)
			if len(lastResults) > 5 {
				lastResults = lastResults[1:]
			}
			updateCounterMutex.Unlock()

			shouldKeep := true
			if r.OnlyValid && (!res.IsValid || res.Balance < r.MinBalance) {
				shouldKeep = false
			}

			if shouldKeep {
				if r.OutFile != "" {
					// If password is set, we must collect all results and encrypt at the end
					// instead of streaming to file.
					if collectAllResults || r.Password != "" {
						allResults = append(allResults, *res)
					}
					if !strings.HasSuffix(strings.ToLower(r.OutFile), ".json") && r.Password == "" {
						r.writeResult(resultFile, csvWriter, res)
					}
				} else {
					allResults = append(allResults, *res)
				}
			}
		}
	}

done:
	if p != nil {
		p.Stop()
	}
	area.Stop()

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

	validColor := pterm.FgGreen
	if validPercent < 50 {
		validColor = pterm.FgYellow
	}

	stats := pterm.TableData{
		{pterm.LightCyan("Metric"), pterm.LightCyan("Value")},
		{"Total Keys", fmt.Sprintf("%d", len(keys))},
		{"Valid Keys", pterm.NewStyle(validColor).Sprint(validCount)},
		{"Invalid Keys", pterm.Red(len(keys) - validCount)},
		{"Throughput", fmt.Sprintf("%.2f keys/s", keysPerSec)},
		{"Total Time", duration.Round(time.Millisecond).String()},
	}

	pterm.DefaultSection.Println("Validation Summary")
	summaryTable, _ := pterm.DefaultTable.WithHasHeader().WithData(stats).Srender()
	pterm.DefaultBox.WithTitle("Results").Println(summaryTable)
	pterm.Println()
}

func (r *Runner) worker(jobs <-chan string, results chan<- *models.ValidationResult, wg *sync.WaitGroup) {
	defer wg.Done()

	for key := range jobs {

		var providersToTry []string
		var suggestions []string

		if r.ManualProvider != "" {
			providersToTry = []string{r.ManualProvider}
		} else {
			detectionResult := r.Detector.DetectProviderWithSuggestion(key, r.ManualCategory)
			if detectionResult.Provider != "unknown" {
				providersToTry = []string{detectionResult.Provider}
			}
			if r.DeepScan && len(detectionResult.Suggestions) > 0 {
				for _, s := range detectionResult.Suggestions {
					if detectionResult.Provider != s {
						providersToTry = append(providersToTry, s)
					}
				}
			}
			if len(providersToTry) == 0 && detectionResult.Provider == "unknown" {
				suggestions = detectionResult.Suggestions
			}
		}

		if len(providersToTry) == 0 {
			errMsg := "Could not auto-detect provider. Use -p flag to specify manually."
			if len(suggestions) > 0 {
				errMsg = fmt.Sprintf("Could not auto-detect provider. Did you mean: %s?", strings.Join(suggestions, ", "))
			}
			results <- &models.ValidationResult{
				Key:          key,
				Provider:     "unknown",
				IsValid:      false,
				ErrorMessage: errMsg,
			}
			continue
		}

		var lastResult *models.ValidationResult
		for _, providerName := range providersToTry {
			val, exists := r.Validators[providerName]
			if !exists {
				continue
			}

			finalRes := r.validateWithRetries(val, key, providerName)
			lastResult = finalRes
			if finalRes.IsValid {
				break
			}
		}
		results <- lastResult
	}
}

func (r *Runner) validateWithRetries(val validators.Validator, key, providerName string) *models.ValidationResult {
	var finalRes *models.ValidationResult
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(r.Timeout)*time.Second*time.Duration(r.Retries+1))
	defer cancel()

	for attempt := 0; attempt <= r.Retries; attempt++ {
		res, err := val.Validate(ctx, key)

		if err != nil {
			errStr := err.Error()
			if strings.Contains(errStr, "timeout") {
				errStr = "Request Timeout"
			}

			if attempt == r.Retries {
				finalRes = &models.ValidationResult{
					Key: key, Provider: providerName, IsValid: false, ErrorMessage: errStr,
				}
				break
			}
			time.Sleep(r.addJitter(time.Duration(1<<attempt) * time.Second))
			continue
		}

		if res.StatusCode == 429 && attempt < r.Retries {
			r.adjustThreads(false)
			backoff := 2
			if res.RetryAfter > 0 {
				backoff = res.RetryAfter
			}
			time.Sleep(r.addJitter(time.Duration(backoff) * time.Second))
			continue
		}

		if res.IsValid {
			r.adjustThreads(true)
			if r.Bench {
				var totalTime float64 = res.ResponseTime
				count := 1
				for i := 0; i < 2; i++ {
					time.Sleep(200 * time.Millisecond) // Small delay
					benchRes, err := val.Validate(ctx, key)
					if err == nil {
						totalTime += benchRes.ResponseTime
						count++
					}
				}
				res.ResponseTime = totalTime / float64(count)
				res.StatusNote = fmt.Sprintf("Benchmarked (%d samples)", count)
			}
		}

		finalRes = res
		break
	}
	return finalRes
}

func (r *Runner) adjustThreads(increase bool) {
	r.threadMux.Lock()
	defer r.threadMux.Unlock()
	if increase {
		if r.adaptiveThreads < r.Threads {
			r.adaptiveThreads++
		}
	} else {
		r.adaptiveThreads = r.adaptiveThreads / 2
		if r.adaptiveThreads < 1 {
			r.adaptiveThreads = 1
		}
	}
}

func (r *Runner) openResultFile() (*os.File, error) {
	if r.OutFile == "" {
		return nil, nil
	}

	flag := os.O_APPEND | os.O_CREATE | os.O_WRONLY

	return os.OpenFile(r.OutFile, flag, 0600)
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

	if strings.HasSuffix(ext, ".jsonl") {
		data, err := json.Marshal(res)
		if err == nil {
			f.Write(data)
			f.WriteString("\n")
		}
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

	sort.Slice(results, func(i, j int) bool {
		if results[i].Balance != results[j].Balance {
			return results[i].Balance > results[j].Balance
		}

		qi := r.getNumericQuota(results[i])
		qj := r.getNumericQuota(results[j])
		if qi != qj {
			return qi > qj
		}

		return results[i].Provider < results[j].Provider
	})

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

func (r *Runner) getNumericQuota(res models.ValidationResult) float64 {
	if res.Extra == nil {
		return 0
	}

	for _, key := range []string{"quota", "credits"} {
		if val, ok := res.Extra[key]; ok {
			switch v := val.(type) {
			case float64:
				return v
			case int:
				return float64(v)
			case string:
				f, _ := strconv.ParseFloat(v, 64)
				return f
			}
		}
	}
	return 0
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

		if res.StatusCode > 0 && res.StatusCode != 200 {
			switch res.StatusCode {
			case 201:
				statusStr = "✓ Created"
			case 204:
				statusStr = "✓ No Content"
			default:
				statusStr = fmt.Sprintf("⚠ %d", res.StatusCode)
			}
		}

		if res.Extra != nil {
			if disabled, ok := res.Extra["disabled"].(bool); ok && disabled {
				statusStr = "⚠ Disabled"
			}
			if blocked, ok := res.Extra["blocked"].(bool); ok && blocked {
				statusStr = "⚠ Blocked"
			}
			if inactive, ok := res.Extra["inactive"].(bool); ok && inactive {
				statusStr = "⚠ Inactive"
			}
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
			switch res.StatusCode {
			case 401:
				statusStr = "✗ Unauthorized"
			case 403:
				statusStr = "✗ Forbidden"
			case 404:
				statusStr = "✗ Not Found"
			case 429:
				statusStr = "✗ Rate Limited"
			case 500:
				statusStr = "✗ Server Error"
			case 502, 503, 504:
				statusStr = fmt.Sprintf("✗ Bad Gateway (%d)", res.StatusCode)
			default:
				statusStr = fmt.Sprintf("✗ %d", res.StatusCode)
			}
		}

		if strings.Contains(strings.ToLower(res.ErrorMessage), "disabled") {
			statusStr = "✗ Disabled"
		} else if strings.Contains(strings.ToLower(res.ErrorMessage), "blocked") {
			statusStr = "✗ Blocked"
		} else if strings.Contains(strings.ToLower(res.ErrorMessage), "expired") {
			statusStr = "✗ Expired"
		} else if strings.Contains(strings.ToLower(res.ErrorMessage), "revok") {
			statusStr = "✗ Revoked"
		} else if strings.Contains(strings.ToLower(res.ErrorMessage), "invalid") {
			statusStr = "✗ Invalid Key"
		} else if strings.Contains(strings.ToLower(res.ErrorMessage), "quota") {
			statusStr = "✗ Quota Exceeded"
		} else if strings.Contains(strings.ToLower(res.ErrorMessage), "insufficient") {
			statusStr = "✗ Insufficient"
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

	if r.Password != "" {
		var buf strings.Builder
		if strings.HasSuffix(ext, ".csv") {
			cw := csv.NewWriter(&buf)
			cw.Write([]string{"Key", "Provider", "IsValid", "Status", "Response", "Balance", "Account", "Email", "Error"})
			for _, res := range results {
				cw.Write([]string{
					res.Key, res.Provider, fmt.Sprintf("%t", res.IsValid),
					fmt.Sprintf("%d", res.StatusCode), fmt.Sprintf("%.2fs", res.ResponseTime),
					fmt.Sprintf("%.5f", res.Balance), res.AccountName, res.Email, res.ErrorMessage,
				})
			}
			cw.Flush()
		} else if strings.HasSuffix(ext, ".jsonl") {
			for _, res := range results {
				data, _ := json.Marshal(res)
				buf.Write(data)
				buf.WriteString("\n")
			}
		} else {
			for _, res := range results {
				buf.WriteString(fmt.Sprintf("%s | %s | %t | %s\n", res.Key, res.Provider, res.IsValid, res.ErrorMessage))
			}
		}
		r.encryptAndWrite([]byte(buf.String()))
		pterm.Success.Printfln("Results ENCRYPTED and exported to %s (%d entries)", r.OutFile, len(results))
		return
	}

	if strings.HasSuffix(ext, ".csv") {
		pterm.Success.Printfln("Results exported to %s (%d entries)", r.OutFile, len(results))
		return
	}

	if strings.HasSuffix(ext, ".jsonl") {
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

	if r.Password != "" {
		r.encryptAndWrite(data)
		pterm.Success.Printfln("Results ENCRYPTED and exported to %s (Total: %d)", r.OutFile, len(existing))
		return
	}

	if err := os.WriteFile(r.OutFile, data, 0600); err != nil {
		pterm.Error.Printfln("Error writing %s: %v", r.OutFile, err)
		return
	}

	pterm.Success.Printfln("Results exported to %s (Total: %d)", r.OutFile, len(existing))
}

func (r *Runner) encryptAndWrite(data []byte) {
	encrypted, err := utils.Encrypt(data, r.Password)
	if err != nil {
		pterm.Error.Printfln("Encryption failed: %v", err)
		return
	}
	if err := os.WriteFile(r.OutFile, encrypted, 0600); err != nil {
		pterm.Error.Printfln("Error writing encrypted file %s: %v", r.OutFile, err)
	}
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

	data, err := os.ReadFile(r.OutFile)
	if err != nil {
		return existing
	}

	if r.Password != "" {
		decrypted, err := utils.Decrypt(data, r.Password)
		if err == nil {
			data = decrypted
		}
	}

	ext := tolower(r.OutFile)

	if strings.HasSuffix(ext, ".json") {
		var results []models.ValidationResult
		if err := json.Unmarshal(data, &results); err == nil {
			for _, res := range results {
				existing[r.hashKey(res.Key)] = true
			}
		}
		return existing
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	if strings.HasSuffix(ext, ".jsonl") {
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var res models.ValidationResult
			if err := json.Unmarshal([]byte(line), &res); err == nil {
				existing[r.hashKey(res.Key)] = true
			}
		}
		return existing
	}

	if strings.HasSuffix(ext, ".csv") {
		reader := csv.NewReader(strings.NewReader(content))
		_, _ = reader.Read() // Skip header
		for {
			record, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err == nil && len(record) > 0 {
				existing[r.hashKey(record[0])] = true
			}
		}
		return existing
	}

	for _, line := range lines {
		key := strings.TrimSpace(line)
		if key != "" {
			existing[r.hashKey(key)] = true
		}
	}

	return existing
}

func (r *Runner) hashKey(key string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(key)))
}

func (r *Runner) addJitter(d time.Duration) time.Duration {
	maxJitterMs := d.Milliseconds() / 4
	if maxJitterMs < 50 {
		maxJitterMs = 50
	}
	jitter, _ := rand.Int(rand.Reader, big.NewInt(maxJitterMs))
	return d + time.Duration(jitter.Int64())*time.Millisecond
}
