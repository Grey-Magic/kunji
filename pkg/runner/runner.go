package runner

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Grey-Magic/kunji/pkg/audit"
	"github.com/Grey-Magic/kunji/pkg/client"
	"github.com/Grey-Magic/kunji/pkg/history"
	"github.com/Grey-Magic/kunji/pkg/models"
	"github.com/Grey-Magic/kunji/pkg/sink"
	"github.com/Grey-Magic/kunji/pkg/source"
	"github.com/Grey-Magic/kunji/pkg/utils"
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
	OnlyValid        bool
	MinBalance       float64
	DeepScan         bool
	NoCache          bool
	SkipMetadata     bool
	CanaryCheck      bool
	Password         string
	Bench            bool
	Quiet            bool
	Format           string
	Factory          *validators.ValidatorFactory
	Detector         *validators.Detector
	ProxyRotator     *client.ProxyRotator
	sharedHTTPClient *http.Client
	negativeCache    *utils.BloomFilter
	negCachePath     string
	metadataJobs     chan *models.ValidationResult
	metadataWg       sync.WaitGroup
	Sinks            []sink.Sink

	// FilterExprs is the parsed --filter argument. Empty means "no filter".
	FilterExprs []FilterExpr
	// Audit is the audit logger. nil disables audit logging.
	Audit *audit.Logger
	// History is the run history logger. nil disables history persistence.
	History *history.Logger
	// RunID is the unique identifier stamped on every history record.
	RunID string
	// Thresholds holds parsed --fail-if conditions. Empty disables CI mode.
	Thresholds []Threshold
	// Stats tracks aggregate run counters. Populated during runInternal;
	// intended for the CI threshold evaluator.
	Stats RunStats
	// EmitDedupe suppresses duplicate (provider, key, is_valid) emits in the
	// JSONL stream. The first emission wins; subsequent identical results
	// during the same run are dropped silently.
	EmitDedupe bool
	// seenEmits is the dedupe key set. Lazily initialized when EmitDedupe is true.
	seenEmits map[string]struct{}
	seenMu    sync.Mutex
	// Sharded enables per-provider worker pools. When true, detection happens
	// once at the feeder and each provider gets its own small worker pool,
	// reducing contention on the proxy rotator and per-provider rate
	// limiter. Off by default.
	Sharded bool
}

func NewRunner(threads int, proxy string, retries int, timeout int, out string, manualProv string, manualCat string, resume bool, onlyValid bool, minBalance float64, skipMetadata bool, canaryCheck bool) (*Runner, error) {
	return NewRunnerWithOptions(threads, proxy, retries, timeout, out, manualProv, manualCat, resume, onlyValid, minBalance, skipMetadata, canaryCheck, validators.FactoryOptions{})
}

// NewRunnerWithOptions is the configurable constructor. factoryOpts controls
// the positive-cache layer (persistent on/off) and whether caching is
// disabled entirely (--no-cache).
func NewRunnerWithOptions(threads int, proxy string, retries int, timeout int, out string, manualProv string, manualCat string, resume bool, onlyValid bool, minBalance float64, skipMetadata bool, canaryCheck bool, factoryOpts validators.FactoryOptions) (*Runner, error) {
	factory, configs, rotator, err := validators.NewValidatorFactoryWithOptions(proxy, timeout, factoryOpts)
	if err != nil {
		return nil, err
	}

	domains := validators.GetCommonDomains()
	go func() {
		client.WarmDNS(domains)
		if factory.SharedClient() != nil {
			client.WarmConnections(factory.SharedClient(), domains)
		}
	}()

	negCachePath := ".kunji_neg_cache"
	negCache, err := utils.LoadBloomFilter(negCachePath)
	if err != nil {
		negCache = utils.NewBloomFilter(1000000, 0.01)
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
		OnlyValid:      onlyValid,
		MinBalance:     minBalance,
		SkipMetadata:   skipMetadata,
		CanaryCheck:    canaryCheck,
		Factory:        factory,
		Detector:       validators.NewDetectorFromConfigs(configs),
		ProxyRotator:   rotator,
		negativeCache:  negCache,
		negCachePath:   negCachePath,
	}, nil
}

func (r *Runner) GetKeyStream(singleKey, keyFile string) (io.ReadCloser, int, error) {
	if singleKey != "" {
		return io.NopCloser(strings.NewReader(singleKey)), 1, nil
	}

	if keyFile != "" {
		file, err := os.Open(keyFile)
		if err != nil {
			return nil, 0, err
		}

		count := 0
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			if strings.TrimSpace(scanner.Text()) != "" {
				count++
			}
		}
		file.Seek(0, 0)
		return file, count, nil
	}

	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		tmpFile, err := os.CreateTemp("", "kunji_stdin_*.txt")
		if err != nil {
			return nil, 0, fmt.Errorf("failed to create temp file for stdin: %w", err)
		}

		count := 0
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" {
				tmpFile.WriteString(line + "\n")
				count++
			}
		}
		tmpFile.Seek(0, 0)
		return tmpFile, count, nil
	}

	return nil, 0, fmt.Errorf("no input provided")
}

func (r *Runner) PreflightProxyCheck() {
	if r.Quiet || r.Format == "json" {
		return
	}
	if r.ProxyRotator == nil {
		return
	}

	spinner, _ := pterm.DefaultSpinner.Start("Checking proxy health...")
	deadCount := r.ProxyRotator.FilterDeadProxies(r.Timeout)

	if deadCount > 0 {
		spinner.Warning(fmt.Sprintf("Discarded %d dead proxies. Continuing with remaining proxies.", deadCount))
	} else {
		spinner.Success("All proxies are healthy.")
	}
}

func (r *Runner) Run(keyReader io.Reader, totalKeys int) {
	r.runInternal(keyReader, totalKeys)
}

// RunSource pulls keys from a Source plugin and feeds them into the validation
// pipeline as if they came from a single reader. The Source's Total() is used
// when non-zero so the progress bar reflects the real key count; otherwise a
// streaming run is launched (totalKeys=0).
func (r *Runner) RunSource(ctx context.Context, src source.Source) {
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		for {
			k, err := src.Next(ctx)
			if err == io.EOF {
				return
			}
			if err != nil {
				_ = pw.CloseWithError(err)
				return
			}
			if _, err := io.WriteString(pw, k+"\n"); err != nil {
				_ = pw.CloseWithError(err)
				return
			}
		}
	}()
	total := src.Total()
	if total <= 0 {
		// The pipeline requires totalKeys >= 1 to start the progress UI; use 1
		// so the bar shows "validating" and the ticker drives updates.
		total = 1
	}
	r.runInternal(pr, total)
}

func (r *Runner) runInternal(keyReader io.Reader, totalKeys int) {
	if !r.Quiet && r.Format != "json" {
		pterm.Success.Printfln("Streaming %d deduplicated and well-formatted keys...", totalKeys)
	}

	if r.Format == "jsonl" {
		if out, err := models.NewStartEvent(&models.StartEvent{
			Total:   totalKeys,
			Threads: r.Threads,
			Proxy:   r.Proxy,
		}).Marshal(); err == nil {
			os.Stdout.Write(out)
		}
	}

	updateCounter := 0
	updateCounterMutex := sync.Mutex{}

	bufferSize := 1000
	if totalKeys < bufferSize {
		bufferSize = totalKeys
	}
	if bufferSize == 0 {
		bufferSize = 1
	}

	jobs := make(chan string, bufferSize)
	results := make(chan *models.ValidationResult, totalKeys+1)
	r.metadataJobs = make(chan *models.ValidationResult, totalKeys+1)
	var wg sync.WaitGroup

	numWorkers := r.Threads
	if totalKeys > 0 && totalKeys < numWorkers {
		numWorkers = totalKeys
	}

	if r.Sharded {
		// Sharded mode: skip the global worker pool and the jobs channel;
		// detection moves to the feeder (runSharded) and results flow
		// directly into `results`. We track completion in the same wg
		// so the close-results goroutine fires when sharded drains too.
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.runSharded(keyReader, results)
		}()
	} else {
		for w := 1; w <= numWorkers; w++ {
			wg.Add(1)
			go r.worker(jobs, results, &wg)
		}
	}

	metadataThreadCount := numWorkers / 2
	if metadataThreadCount < 1 {
		metadataThreadCount = 1
	}
	for w := 1; w <= metadataThreadCount; w++ {
		go r.metadataWorker(results)
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
		defer closeJobs()

		// In sharded mode, runSharded owns the keyReader. Skip this feeder.
		if r.Sharded {
			return
		}

		alreadyProcessed := utils.NewBloomFilter(10000000, 0.001)
		if r.Resume {
			alreadyProcessed = r.loadExistingKeys()
		}

		uniqueKeys := utils.NewBloomFilter(10000000, 0.001)

		scanner := bufio.NewScanner(keyReader)
		for scanner.Scan() {
			key := strings.TrimSpace(scanner.Text())
			if len(key) < r.MinKeyLength {
				updateCounterMutex.Lock()
				updateCounter++
				updateCounterMutex.Unlock()
				continue
			}

			if uniqueKeys.Test(key) {
				updateCounterMutex.Lock()
				updateCounter++
				updateCounterMutex.Unlock()
				continue
			}
			uniqueKeys.Add(key)

			if !r.NoCache && r.negativeCache != nil && r.negativeCache.Test(key) {
				updateCounterMutex.Lock()
				updateCounter++
				updateCounterMutex.Unlock()
				continue
			}

			if r.Resume && alreadyProcessed.Test(key) {
				updateCounterMutex.Lock()
				updateCounter++
				updateCounterMutex.Unlock()
				continue
			}

			select {
			case jobs <- key:
			case <-sigCh:
				return
			}
		}
	}()

	startTime := time.Now()

	go func() {
		wg.Wait()
		close(r.metadataJobs)
		r.metadataWg.Wait()
		close(results)
	}()

	validCount := 0

	var p *pterm.ProgressbarPrinter
	showUI := totalKeys > 1 && !r.Quiet
	if showUI {
		pp := pterm.DefaultProgressbar.WithTotal(totalKeys).WithTitle("Validating API Keys").WithRemoveWhenDone(true)
		p, _ = pp.Start()
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
		if err := csvWriter.Write([]string{"Key", "Provider", "Endpoint", "IsValid", "StatusCode", "ResponseTime", "Balance", "AccountName", "Email", "Error"}); err != nil {
			pterm.Warning.Printfln("Failed to write CSV header: %v", err)
		}
		csvWriter.Flush()
	}

	collectAllResults := r.OutFile == "" || strings.HasSuffix(strings.ToLower(r.OutFile), ".json")
	var allResults []models.ValidationResult

	var progressTicker *time.Ticker
	if showUI {
		progressTicker = time.NewTicker(500 * time.Millisecond)
		defer progressTicker.Stop()
	}

	var area *pterm.AreaPrinter
	if showUI {
		area, _ = pterm.DefaultArea.Start()
		defer area.Stop()
	}

	lastResults := make([]*models.ValidationResult, 0, 5)
	providerStats := make(map[string]models.ProviderTotals)
	var statsMu sync.Mutex

	var lastAreaText string
	var lastTitle string
	lastUpdateTitle := time.Time{}

	buildAreaText := func(results []*models.ValidationResult) string {
		var areaText string
		for i := len(results) - 1; i >= 0; i-- {
			res := results[i]
			var status string
			switch {
			case res.IsValid:
				status = pterm.Green("✓ Valid")
			case res.StatusCode == 429:
				status = pterm.Yellow("⚠ Rate Limited")
			case strings.Contains(res.ErrorMessage, "Timeout") || strings.Contains(res.ErrorMessage, "proxy"):
				status = pterm.Magenta("⧗ Network Error")
			case strings.Contains(res.ErrorMessage, "Canary") || strings.Contains(res.ErrorMessage, "detect"):
				status = pterm.Gray("∅ Skipped")
			default:
				status = pterm.Red("✗ Invalid")
			}
			keyMasked := maskKey(res.Key)
			areaText += fmt.Sprintf("  %s %-15s %s %s\n", pterm.Gray("»"), pterm.LightCyan(res.Provider), status, pterm.Gray(keyMasked))
		}
		return areaText
	}

	for {
		var tickerChan <-chan time.Time
		if progressTicker != nil {
			tickerChan = progressTicker.C
		}

		select {
		case <-tickerChan:
			updateCounterMutex.Lock()
			if p != nil && updateCounter > 0 {
				p.Add(updateCounter)
				updateCounter = 0

				current := p.Current
				if current > 0 {
					elapsed := time.Since(startTime)
					speed := float64(current) / elapsed.Seconds()
					remaining := totalKeys - current
					eta := time.Duration(float64(remaining)/speed) * time.Second
					newTitle := fmt.Sprintf("Validating API Keys [%.2f keys/s, ETA: %s]", speed, eta.Round(time.Second))
					// Throttle title rewrites to ~1s to avoid flicker from
					// the progressbar redrawing on every tick.
					if newTitle != lastTitle && time.Since(lastUpdateTitle) > time.Second {
						p.UpdateTitle(newTitle)
						lastTitle = newTitle
						lastUpdateTitle = time.Now()
					}

					if r.Format == "jsonl" {
						progress := &models.ProgressEvent{
							Done:       current,
							Total:      totalKeys,
							Valid:      validCount,
							SpeedKeys:  speed,
							ETASeconds: eta.Seconds(),
						}
						if out, err := models.NewProgressEvent(progress).Marshal(); err == nil {
							os.Stdout.Write(out)
						}
					}
				}
			}

			if area != nil {
				areaText := buildAreaText(lastResults)
				// Only redraw the live region when the content actually
				// changed; this keeps the area stable between result events
				// and eliminates the strobe/flicker against the progressbar.
				if areaText != lastAreaText {
					area.Update(areaText)
					lastAreaText = areaText
				}
			}
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

			// Always update aggregate Stats so CI threshold checks have
			// accurate totals even when a filter drops the result downstream.
			statsMu.Lock()
			r.Stats.Total++
			if res.IsValid {
				r.Stats.Valid++
			} else {
				switch {
				case res.StatusCode == 429:
					r.Stats.RateLimit++
				case strings.Contains(res.ErrorMessage, "Canary") || strings.Contains(res.ErrorMessage, "detect"):
					r.Stats.Skipped++
				case res.StatusCode >= 500 || strings.Contains(res.ErrorMessage, "Timeout") || strings.Contains(res.ErrorMessage, "Network"):
					r.Stats.Errored++
				default:
					r.Stats.Invalid++
				}
			}
			statsMu.Unlock()

			// Audit log: emit every result regardless of filter (audit
			// records what we attempted, not what the user kept).
			if r.Audit != nil {
				_ = r.Audit.Emit(res)
			}
			// History log: same per-result record, tagged with the run ID
			// so downstream `kunji history` queries can correlate keys.
			if r.History != nil {
				_ = r.History.Emit(res)
			}

			// Dedupe: suppress identical (provider, key, is_valid) emits
			// from the JSONL stream and sink list. The first emission wins.
			// We use shouldKeep=false so UI/progress still update, but
			// file/sink/stream emission is skipped on duplicates.
			dup := false
			if r.EmitDedupe {
				r.seenMu.Lock()
				if r.seenEmits == nil {
					r.seenEmits = make(map[string]struct{})
				}
				k := res.Provider + "|" + res.Key + "|" + boolStr(res.IsValid)
				if _, exists := r.seenEmits[k]; exists {
					dup = true
				} else {
					r.seenEmits[k] = struct{}{}
				}
				r.seenMu.Unlock()
			}

			// Filter: when --filter is set, drop results that don't match
			// before any UI / sink / file emission happens.
			if !Match(r.FilterExprs, res) {
				updateCounterMutex.Lock()
				updateCounter++
				updateCounterMutex.Unlock()
				continue
			}

			if res.IsValid {
				validCount++
				statsMu.Lock()
				totals := providerStats[res.Provider]
				totals.Valid++
				providerStats[res.Provider] = totals
				statsMu.Unlock()
			} else if res.ErrorMessage != "Probe cancelled" && !strings.Contains(res.ErrorMessage, "Could not auto-detect") {
				if !r.NoCache && r.negativeCache != nil {
					r.negativeCache.Add(res.Key)
				}
				statsMu.Lock()
				totals := providerStats[res.Provider]
				switch {
				case res.StatusCode == 429:
					totals.RateLimit++
				case strings.Contains(res.ErrorMessage, "Canary") || strings.Contains(res.ErrorMessage, "detect"):
					totals.Skipped++
				default:
					totals.Invalid++
				}
				providerStats[res.Provider] = totals
				statsMu.Unlock()
			}

			updateCounterMutex.Lock()
			updateCounter++

			lastResults = append(lastResults, res)
			if len(lastResults) > 5 {
				lastResults = lastResults[1:]
			}

			// Redraw the live region immediately on new results so feedback
			// is instant, but only when the rendered text actually changes.
			if area != nil {
				areaText := buildAreaText(lastResults)
				if areaText != lastAreaText {
					area.Update(areaText)
					lastAreaText = areaText
				}
			}
			updateCounterMutex.Unlock()

			shouldKeep := true
			if dup {
				shouldKeep = false
			}
			if r.OnlyValid && (!res.IsValid || res.Balance < r.MinBalance) {
				shouldKeep = false
			}

			if shouldKeep {
				for _, sk := range r.Sinks {
					// Sinks run detached so a slow HTTP endpoint never stalls
					// the result loop. Errors are intentionally swallowed here;
					// per-sink Stats() methods expose failure counts.
					go func(s sink.Sink) {
						_ = s.Emit(context.Background(), res)
					}(sk)
				}
				if r.Format == "json" {
					b, _ := json.Marshal(res)
					fmt.Println(string(b))
					if r.OutFile != "" && r.OutFile != "stdout" {
						allResults = append(allResults, *res)
					}
				} else if r.Format == "jsonl" {
					if out, err := models.NewResultEvent(res).Marshal(); err == nil {
						os.Stdout.Write(out)
					}
					allResults = append(allResults, *res)
				} else if r.OutFile != "" {
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
		default:
			// If we have no ticker and no result yet, don't spin too fast
			if progressTicker == nil {
				// We still need to check the results channel, but select default won't wait.
				// However, if we reach here it means results channel was not ready.
				time.Sleep(10 * time.Millisecond)
			}
		}
	}

done:
	for _, sk := range r.Sinks {
		_ = sk.Close()
	}
	if p != nil {
		p.Stop()
	}
	if area != nil {
		area.Clear()
		area.Stop()
	}

	if !r.Quiet && r.Format != "json" {
		pterm.Println()
	}

	if csvWriter != nil {
		csvWriter.Flush()
	}

	if r.OutFile != "" && r.Format != "json" {
		r.exportResults(allResults)
	} else if r.Format == "json" && r.OutFile != "" && r.OutFile != "stdout" {
		r.exportResults(allResults)
	} else if len(allResults) > 0 && !r.Quiet && r.Format != "json" {
		r.displayResultsTable(allResults)
	}

	// Save negative cache unless caching is disabled
	if !r.NoCache && r.negativeCache != nil {
		r.negativeCache.Save(r.negCachePath)
	}

	duration := time.Since(startTime)
	keysPerSec := float64(totalKeys) / duration.Seconds()

	validPercent := 0
	if totalKeys > 0 {
		validPercent = (validCount * 100) / totalKeys
	}

	validColor := pterm.FgGreen
	if validPercent < 50 {
		validColor = pterm.FgYellow
	}

	stats := pterm.TableData{
		{pterm.LightCyan("Metric"), pterm.LightCyan("Value")},
		{"Total Keys", fmt.Sprintf("%d", totalKeys)},
		{"Valid Keys", pterm.NewStyle(validColor).Sprint(validCount)},
		{"Invalid Keys", pterm.Red(totalKeys - validCount)},
		{"Throughput", fmt.Sprintf("%.2f keys/s", keysPerSec)},
		{"Total Time", duration.Round(time.Millisecond).String()},
	}

	if !r.Quiet && r.Format != "json" && totalKeys > 5 {
		pterm.DefaultSection.Println("Validation Summary")
		summaryTable, _ := pterm.DefaultTable.WithHasHeader().WithData(stats).Srender()
		pterm.DefaultBox.WithTitle("Results").Println(summaryTable)

		r.renderProviderBreakdown(&statsMu, providerStats, totalKeys)
		pterm.Println()
	}

	if r.Format == "jsonl" {
		var invalid, rateLimit, skipped int
		statsMu.Lock()
		byProvider := make(map[string]models.ProviderTotals, len(providerStats))
		for prov, t := range providerStats {
			byProvider[prov] = t
			invalid += t.Invalid
			rateLimit += t.RateLimit
			skipped += t.Skipped
		}
		statsMu.Unlock()
		summary := &models.SummaryEvent{
			Total:      totalKeys,
			Valid:      validCount,
			Invalid:    invalid,
			RateLimit:  rateLimit,
			Skipped:    skipped,
			DurationMs: time.Since(startTime).Milliseconds(),
			ByProvider: byProvider,
		}
		if out, err := models.NewSummaryEvent(summary).Marshal(); err == nil {
			os.Stdout.Write(out)
		}
	}
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

		if len(providersToTry) == 1 {
			val, exists := r.Factory.GetValidator(providersToTry[0])
			if exists {
				val.SetSkipMetadata(true)
				val.SetCanaryCheck(r.CanaryCheck)
				res := r.validateWithRetries(val, key, providersToTry[0])

				if res.IsValid && !r.SkipMetadata {
					r.metadataWg.Add(1)
					select {
					case r.metadataJobs <- res:
					default:
						r.metadataWg.Done()
						results <- res
					}
				} else {
					results <- res
				}
			}
			continue
		}

		var probeWg sync.WaitGroup
		probeResults := make(chan *models.ValidationResult, len(providersToTry))
		ctx, cancel := context.WithCancel(context.Background())

		for _, pName := range providersToTry {
			probeWg.Add(1)
			go func(name string) {
				defer probeWg.Done()
				val, exists := r.Factory.GetValidator(name)
				if !exists {
					return
				}
				val.SetSkipMetadata(true)
				val.SetCanaryCheck(r.CanaryCheck)

				res := r.validateWithRetriesWithContext(ctx, val, key, name)
				if res.IsValid {
					cancel()
				}
				probeResults <- res
			}(pName)
		}

		go func() {
			probeWg.Wait()
			close(probeResults)
			cancel()
		}()

		var bestResult *models.ValidationResult
		for res := range probeResults {
			if bestResult == nil || res.IsValid {
				bestResult = res
			}
			if res.IsValid {
				go func() {
					for range probeResults {
					}
				}()
				break
			}
		}

		if bestResult.IsValid && !r.SkipMetadata {
			r.metadataWg.Add(1)
			select {
			case r.metadataJobs <- bestResult:
			default:
				r.metadataWg.Done()
				results <- bestResult
			}
		} else {
			results <- bestResult
		}
	}
}

func (r *Runner) metadataWorker(results chan<- *models.ValidationResult) {
	for res := range r.metadataJobs {
		val, exists := r.Factory.GetValidator(res.Provider)
		if exists {
			val.FetchMetadata(context.Background(), res.Key, res)
		}
		results <- res
		r.metadataWg.Done()
	}
}

func (r *Runner) validateWithRetries(val validators.Validator, key, providerName string) *models.ValidationResult {
	return r.validateWithRetriesWithContext(context.Background(), val, key, providerName)
}

// retryBudget derives the effective retry behavior for this provider. A
// non-nil RetryPolicy in the provider config overrides the global defaults.
type retryBudget struct {
	maxAttempts    int
	onStatus       map[int]bool
	initialBackoff time.Duration
	maxBackoff     time.Duration
}

func (r *Runner) retryBudget(val validators.Validator) retryBudget {
	b := retryBudget{
		maxAttempts:    r.Retries + 1,
		onStatus:       defaultRetryOnStatus(),
		initialBackoff: 1 * time.Second,
		maxBackoff:     8 * time.Second,
	}
	if gv, ok := val.(*validators.GenericValidator); ok && gv.Config().RetryPolicy != nil {
		rp := gv.Config().RetryPolicy
		if rp.MaxAttempts >= 1 {
			b.maxAttempts = rp.MaxAttempts
		}
		if len(rp.OnStatus) > 0 {
			b.onStatus = make(map[int]bool, len(rp.OnStatus))
			for _, s := range rp.OnStatus {
				b.onStatus[s] = true
			}
		}
		if rp.InitialBackoffMs > 0 {
			b.initialBackoff = time.Duration(rp.InitialBackoffMs) * time.Millisecond
		}
		if rp.MaxBackoffMs > 0 {
			b.maxBackoff = time.Duration(rp.MaxBackoffMs) * time.Millisecond
		}
	}
	return b
}

// defaultRetryOnStatus returns the conservative retry set: 429 plus any 5xx.
func defaultRetryOnStatus() map[int]bool {
	m := make(map[int]bool, 7)
	m[429] = true
	for c := 500; c < 600; c++ {
		m[c] = true
	}
	return m
}

// backoffDuration returns the wait before retrying after the given attempt.
// Schedule doubles each attempt up to maxBackoff; matches the historical
// 1<<attempt seconds behavior when no per-provider override is set.
func (b retryBudget) backoffDuration(attempt int, retryAfterSec int) time.Duration {
	// Retry-After replaces the schedule-derived duration, but is capped at
	// maxBackoff to prevent a hostile or buggy server from forcing us to
	// sleep for hours.
	if retryAfterSec > 0 {
		d := time.Duration(retryAfterSec) * time.Second
		if d > b.maxBackoff {
			d = b.maxBackoff
		}
		return d
	}
	d := b.initialBackoff << attempt
	if d > b.maxBackoff {
		d = b.maxBackoff
	}
	return d
}

func (r *Runner) validateWithRetriesWithContext(parentCtx context.Context, val validators.Validator, key, providerName string) *models.ValidationResult {
	var finalRes *models.ValidationResult
	budget := r.retryBudget(val)
	ctx, cancel := context.WithTimeout(parentCtx, time.Duration(r.Timeout)*time.Second*time.Duration(budget.maxAttempts))
	defer cancel()

	for attempt := 0; attempt < budget.maxAttempts; attempt++ {
		select {
		case <-parentCtx.Done():
			return &models.ValidationResult{Key: key, Provider: providerName, IsValid: false, ErrorMessage: "Probe cancelled"}
		default:
		}

		res, err := val.Validate(ctx, key)

		if err != nil {
			errStr := err.Error()
			if strings.Contains(errStr, "timeout") {
				errStr = "Request Timeout"
			}

			if attempt == budget.maxAttempts-1 {
				finalRes = &models.ValidationResult{
					Key: key, Provider: providerName, IsValid: false, ErrorMessage: errStr,
				}
				break
			}
			time.Sleep(r.addJitter(budget.backoffDuration(attempt, 0)))
			continue
		}

		if budget.onStatus[res.StatusCode] && attempt < budget.maxAttempts-1 {
			time.Sleep(r.addJitter(budget.backoffDuration(attempt, res.RetryAfter)))
			continue
		}

		if res.IsValid {
			if r.Bench {
				var totalTime float64 = res.ResponseTime
				count := 1
				for i := 0; i < 2; i++ {
					time.Sleep(200 * time.Millisecond)
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
			res.Endpoint,
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

func (r *Runner) renderProviderBreakdown(mu *sync.Mutex, stats map[string]models.ProviderTotals, total int) {
	mu.Lock()
	defer mu.Unlock()
	if len(stats) == 0 {
		return
	}

	type row struct {
		name                   string
		valid, invalid, rl, sk int
		total                  int
		hitRate                float64
	}
	rows := make([]row, 0, len(stats))
	for name, t := range stats {
		tot := t.Valid + t.Invalid + t.RateLimit + t.Skipped
		if tot == 0 {
			continue
		}
		rows = append(rows, row{
			name:    name,
			valid:   t.Valid,
			invalid: t.Invalid,
			rl:      t.RateLimit,
			sk:      t.Skipped,
			total:   tot,
			hitRate: float64(t.Valid) / float64(tot) * 100,
		})
	}
	if len(rows) == 0 {
		return
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].valid != rows[j].valid {
			return rows[i].valid > rows[j].valid
		}
		return rows[i].name < rows[j].name
	})

	if len(rows) > 15 {
		rows = rows[:15]
	}

	pterm.Println()
	pterm.DefaultSection.Println("Per-Provider Breakdown")
	tableData := pterm.TableData{
		{pterm.LightCyan("Provider"), pterm.LightCyan("Valid"), pterm.LightCyan("Invalid"), pterm.LightCyan("Rate Ltd"), pterm.LightCyan("Skip"), pterm.LightCyan("Total"), pterm.LightCyan("Hit %")},
	}
	for _, r := range rows {
		hitColor := pterm.FgGreen
		if r.hitRate < 50 {
			hitColor = pterm.FgYellow
		}
		if r.hitRate < 20 {
			hitColor = pterm.FgRed
		}
		tableData = append(tableData, []string{
			r.name,
			pterm.Green(fmt.Sprintf("%d", r.valid)),
			pterm.Red(fmt.Sprintf("%d", r.invalid)),
			pterm.Yellow(fmt.Sprintf("%d", r.rl)),
			pterm.Gray(fmt.Sprintf("%d", r.sk)),
			fmt.Sprintf("%d", r.total),
			pterm.NewStyle(hitColor).Sprintf("%.1f%%", r.hitRate),
		})
	}
	pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
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
		{"Provider", "Endpoint", "Key", "Status", "Response", "Balance", "Account / Email"},
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
		if res.GraphQLInfo != "" {
			parts = append(parts, pterm.Magenta("GraphQL: ")+res.GraphQLInfo)
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

		statusStr := fmt.Sprintf("✓ Valid (%d)", res.StatusCode)
		if res.StatusNote != "" {
			statusStr = fmt.Sprintf("⚠ %s (%d)", res.StatusNote, res.StatusCode)
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

		endpointDisplay := res.Endpoint
		if len(endpointDisplay) > 30 {
			u, err := url.Parse(res.Endpoint)
			if err == nil {
				endpointDisplay = u.Host + u.Path
				if len(endpointDisplay) > 30 {
					endpointDisplay = endpointDisplay[:27] + "..."
				}
			} else if len(endpointDisplay) > 30 {
				endpointDisplay = endpointDisplay[:27] + "..."
			}
		}

		tableData = append(tableData, []string{
			pterm.Cyan(res.Provider),
			pterm.Gray(endpointDisplay),
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

		if res.ResponseBody != "" || res.ErrorMessage != "" {
			if !hasExtra {
				pterm.Info.Printfln("%s %s:", res.Provider, pterm.Green("Additional Info:"))
				hasExtra = true
			}
			if res.ResponseBody != "" {
				bodyStr := res.ResponseBody
				if len(bodyStr) > 200 {
					bodyStr = bodyStr[:197] + "..."
				}
				pterm.Println("  " + pterm.Yellow("Response Body:") + " " + pterm.Gray(bodyStr))
			}
			if res.ErrorMessage != "" {
				pterm.Println("  " + pterm.Yellow("Error Message:") + " " + pterm.Red(res.ErrorMessage))
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
		{"Provider", "Endpoint", "Key", "Status", "Reason", "Error"},
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

		statusStr := fmt.Sprintf("✗ %s (%d)", res.InvalidReason, res.StatusCode)
		if res.InvalidReason == "" && res.StatusCode > 0 {
			switch res.StatusCode {
			case 401:
				statusStr = fmt.Sprintf("✗ Unauthorized (%d)", res.StatusCode)
			case 403:
				statusStr = fmt.Sprintf("✗ Forbidden (%d)", res.StatusCode)
			case 404:
				statusStr = fmt.Sprintf("✗ Not Found (%d)", res.StatusCode)
			case 429:
				statusStr = fmt.Sprintf("✗ Rate Limited (%d)", res.StatusCode)
			case 500:
				statusStr = fmt.Sprintf("✗ Server Error (%d)", res.StatusCode)
			case 502, 503, 504:
				statusStr = fmt.Sprintf("✗ Bad Gateway (%d)", res.StatusCode)
			default:
				if res.StatusCode > 0 {
					statusStr = fmt.Sprintf("✗ %d", res.StatusCode)
				}
			}
		}

		if res.InvalidReason == "" {
			if strings.Contains(strings.ToLower(res.ErrorMessage), "disabled") {
				statusStr = fmt.Sprintf("✗ Disabled (%d)", res.StatusCode)
			} else if strings.Contains(strings.ToLower(res.ErrorMessage), "blocked") {
				statusStr = fmt.Sprintf("✗ Blocked (%d)", res.StatusCode)
			} else if strings.Contains(strings.ToLower(res.ErrorMessage), "expired") {
				statusStr = fmt.Sprintf("✗ Expired (%d)", res.StatusCode)
			} else if strings.Contains(strings.ToLower(res.ErrorMessage), "revok") {
				statusStr = fmt.Sprintf("✗ Revoked (%d)", res.StatusCode)
			} else if strings.Contains(strings.ToLower(res.ErrorMessage), "invalid") {
				statusStr = fmt.Sprintf("✗ Invalid Key (%d)", res.StatusCode)
			} else if strings.Contains(strings.ToLower(res.ErrorMessage), "quota") {
				statusStr = fmt.Sprintf("✗ Quota Exceeded (%d)", res.StatusCode)
			} else if strings.Contains(strings.ToLower(res.ErrorMessage), "insufficient") {
				statusStr = fmt.Sprintf("✗ Insufficient (%d)", res.StatusCode)
			}
		}

		endpointDisplay := res.Endpoint
		if len(endpointDisplay) > 30 {
			u, err := url.Parse(res.Endpoint)
			if err == nil {
				endpointDisplay = u.Host + u.Path
				if len(endpointDisplay) > 30 {
					endpointDisplay = endpointDisplay[:27] + "..."
				}
			} else if len(endpointDisplay) > 30 {
				endpointDisplay = endpointDisplay[:27] + "..."
			}
		}

		tableData = append(tableData, []string{
			pterm.Cyan(res.Provider),
			pterm.Gray(endpointDisplay),
			keyMasked,
			statusStr,
			res.InvalidReason,
			pterm.Gray(errMsg),
		})
	}

	pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()

	// Print response body for invalid keys as extra info
	for _, res := range results {
		if res.ResponseBody != "" && res.ResponseBody != res.ErrorMessage {
			pterm.Info.Printfln("%s %s:", res.Provider, pterm.Red("Invalid Key Info:"))
			bodyStr := res.ResponseBody
			if len(bodyStr) > 200 {
				bodyStr = bodyStr[:197] + "..."
			}
			pterm.Println("  " + pterm.Yellow("Response Body:") + " " + pterm.Gray(bodyStr))
		}
	}
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

func (r *Runner) loadExistingKeys() *utils.BloomFilter {
	existing := utils.NewBloomFilter(10000000, 0.001)
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
				existing.Add(res.Key)
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
				existing.Add(res.Key)
			}
		}
		return existing
	}

	if strings.HasSuffix(ext, ".csv") {
		reader := csv.NewReader(strings.NewReader(content))
		_, _ = reader.Read()
		for {
			record, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err == nil && len(record) > 0 {
				existing.Add(record[0])
			}
		}
		return existing
	}

	for _, line := range lines {
		key := strings.TrimSpace(line)
		if key != "" {
			existing.Add(key)
		}
	}

	return existing
}

func (r *Runner) addJitter(d time.Duration) time.Duration {
	maxJitterMs := d.Milliseconds() / 4
	if maxJitterMs < 50 {
		maxJitterMs = 50
	}
	jitter, _ := rand.Int(rand.Reader, big.NewInt(maxJitterMs))
	return d + time.Duration(jitter.Int64())*time.Millisecond
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
