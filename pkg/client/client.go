package client

import (
	"bufio"
	"context"
	"crypto/rand"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pterm/pterm"
	"golang.org/x/time/rate"
)

var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 17_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Mobile/15E148 Safari/604.1",
}

var userAgentsLen = big.NewInt(int64(len(userAgents)))

func GetRandomUserAgent() string {
	n, _ := rand.Int(rand.Reader, userAgentsLen)
	return userAgents[n.Int64()]
}

type ProxyRotator struct {
	proxies []*url.URL
	index   int
	mux     sync.Mutex
}

func NewProxyRotator(proxyInput string) (*ProxyRotator, error) {
	if proxyInput == "" {
		return &ProxyRotator{proxies: nil}, nil
	}

	urls := []*url.URL{}

	file, err := os.Open(proxyInput)
	if err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" {
				if !strings.HasPrefix(line, "http") && !strings.HasPrefix(line, "socks5") {
					line = "http://" + line
				}
				u, err := url.Parse(line)
				if err == nil {
					urls = append(urls, u)
				}
			}
		}
		if len(urls) > 0 {
			return &ProxyRotator{proxies: urls}, nil
		}
	}

	line := proxyInput
	if !strings.HasPrefix(line, "http") && !strings.HasPrefix(line, "socks5") {
		line = "http://" + line
	}
	u, err := url.Parse(line)
	if err == nil {
		urls = append(urls, u)
	}

	return &ProxyRotator{proxies: urls}, nil
}

func (pr *ProxyRotator) GetProxy(req *http.Request) (*url.URL, error) {
	pr.mux.Lock()
	defer pr.mux.Unlock()
	if len(pr.proxies) == 0 {
		return nil, nil
	}
	p := pr.proxies[pr.index]
	pr.index = (pr.index + 1) % len(pr.proxies)
	return p, nil
}

func (pr *ProxyRotator) ReportFailure(pxy *url.URL) {
	pr.mux.Lock()
	defer pr.mux.Unlock()

	newProxies := []*url.URL{}
	found := false
	for _, p := range pr.proxies {
		if p.String() == pxy.String() {
			found = true
			continue
		}
		newProxies = append(newProxies, p)
	}

	if found {
		pr.proxies = newProxies
		if len(pr.proxies) > 0 {
			pr.index = pr.index % len(pr.proxies)
		}
	}
}

func (pr *ProxyRotator) FilterDeadProxies(timeoutSecs int) int {
	if len(pr.proxies) == 0 {
		return 0
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	validProxies := []*url.URL{}

	testURL := "https://api.ipify.org?format=json"

	for _, p := range pr.proxies {
		wg.Add(1)
		go func(pxy *url.URL) {
			defer wg.Done()

			transport := &http.Transport{
				Proxy: http.ProxyURL(pxy),
				DialContext: (&net.Dialer{
					Timeout: time.Duration(timeoutSecs) * time.Second,
				}).DialContext,
			}
			client := &http.Client{
				Transport: transport,
				Timeout:   time.Duration(timeoutSecs) * time.Second,
			}

			req, _ := http.NewRequest("GET", testURL, nil)
			req.Header.Set("User-Agent", GetRandomUserAgent())

			resp, err := client.Do(req)
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == 200 {
					mu.Lock()
					validProxies = append(validProxies, pxy)
					mu.Unlock()
				}
			}
		}(p)
	}

	wg.Wait()

	deadCount := len(pr.proxies) - len(validProxies)
	pr.mux.Lock()
	pr.proxies = validProxies
	pr.index = 0
	pr.mux.Unlock()

	return deadCount
}

type providerState struct {
	limiter        *rate.Limiter
	consecutive429 int
	currentR       rate.Limit
	lastSuccess    time.Time
	last429        time.Time
	totalRequests  int64
	total429s      int64
	backoffUntil   time.Time
}

type RateLimiterManager struct {
	states   map[string]*providerState
	mux      sync.RWMutex
	defaultR rate.Limit
	defaultB int
}

func NewRateLimiterManager(r rate.Limit, b int) *RateLimiterManager {
	return &RateLimiterManager{
		states:   make(map[string]*providerState),
		defaultR: r,
		defaultB: b,
	}
}

func (rm *RateLimiterManager) Wait(ctx context.Context, provider string) error {
	rm.mux.RLock()
	state, exists := rm.states[provider]
	rm.mux.RUnlock()

	if !exists {
		rm.mux.Lock()
		state, exists = rm.states[provider]
		if !exists {
			state = &providerState{
				limiter:  rate.NewLimiter(rm.defaultR, rm.defaultB),
				currentR: rm.defaultR,
			}
			rm.states[provider] = state
		}
		rm.mux.Unlock()
	}

	if !state.backoffUntil.IsZero() {
		waitDur := time.Until(state.backoffUntil)
		if waitDur > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(waitDur):
			}
		}
	}

	return state.limiter.Wait(ctx)
}

func (rm *RateLimiterManager) ReportResult(provider string, statusCode int) {
	rm.mux.Lock()
	defer rm.mux.Unlock()

	state, exists := rm.states[provider]
	if !exists {
		return
	}

	state.totalRequests++

	if statusCode == 429 {
		state.consecutive429++
		state.total429s++
		state.last429 = time.Now()

		state.currentR = state.currentR / 2
		if state.currentR < 0.1 {
			state.currentR = 0.1
		}
		state.limiter.SetLimit(state.currentR)

		switch {
		case state.consecutive429 >= 4:
			state.backoffUntil = time.Now().Add(30 * time.Second)
		case state.consecutive429 >= 2:
			state.backoffUntil = time.Now().Add(10 * time.Second)
		default:
			state.backoffUntil = time.Now().Add(3 * time.Second)
		}

		pterm.Debug.Printfln("Throttling %s: %d consecutive 429s, limit=%.2f req/s, backoff=%.0fs",
			provider, state.consecutive429, float64(state.currentR), time.Until(state.backoffUntil).Seconds())
	} else if statusCode >= 200 && statusCode < 300 {
		state.consecutive429 = 0
		state.lastSuccess = time.Now()
		state.backoffUntil = time.Time{}

		if state.currentR < rm.defaultR {
			state.currentR = state.currentR * 1.5
			if state.currentR > rm.defaultR {
				state.currentR = rm.defaultR
			}
			state.limiter.SetLimit(state.currentR)
			pterm.Debug.Printfln("Recovering %s: limit=%.2f req/s", provider, float64(state.currentR))
		}
	} else if statusCode >= 500 {
		state.consecutive429 = 0
		state.backoffUntil = time.Time{}

		if state.currentR < rm.defaultR {
			state.currentR = state.currentR * 1.1
			if state.currentR > rm.defaultR {
				state.currentR = rm.defaultR
			}
			state.limiter.SetLimit(state.currentR)
		}
	}
}

func (rm *RateLimiterManager) SetLimit(provider string, r rate.Limit, b int) {
	rm.mux.Lock()
	defer rm.mux.Unlock()
	rm.states[provider] = &providerState{
		limiter:  rate.NewLimiter(r, b),
		currentR: r,
	}
}

func (rm *RateLimiterManager) Stats() map[string]struct {
	Requests int64
	Rate429s int64
	Limit    float64
} {
	rm.mux.RLock()
	defer rm.mux.RUnlock()

	stats := make(map[string]struct {
		Requests int64
		Rate429s int64
		Limit    float64
	}, len(rm.states))
	for name, s := range rm.states {
		stats[name] = struct {
			Requests int64
			Rate429s int64
			Limit    float64
		}{
			Requests: s.totalRequests,
			Rate429s: s.total429s,
			Limit:    float64(s.currentR),
		}
	}
	return stats
}

var (
	dnsCache    = make(map[string][]net.IP)
	dnsCacheMux sync.RWMutex
)

func WarmDNS(domains []string) {
	var wg sync.WaitGroup
	for _, domain := range domains {
		wg.Add(1)
		go func(d string) {
			defer wg.Done()
			ips, err := net.LookupIP(d)
			if err == nil && len(ips) > 0 {
				dnsCacheMux.Lock()
				dnsCache[d] = ips
				dnsCacheMux.Unlock()
			}
		}(domain)
	}
	wg.Wait()
}

func NewHTTPClient(proxyStr string, timeoutSecs int) (*http.Client, *ProxyRotator, error) {
	rotator, err := NewProxyRotator(proxyStr)
	if err != nil {
		return nil, nil, err
	}

	dialTimeout := time.Duration(timeoutSecs) * time.Second
	if dialTimeout < 10*time.Second {
		dialTimeout = 10 * time.Second
	}

	dialer := &net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: 30 * time.Second,
	}

	poolMgr := NewConnectionPoolManager(HostPoolConfig{
		MaxConnsPerHost:     100,
		MaxIdleConnsPerHost: 20,
		IdleTimeout:         120 * time.Second,
	})

	transport := poolMgr.BuildTransport(dialer, rotator.GetProxy)

	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, _ := net.SplitHostPort(addr)
		dnsCacheMux.RLock()
		ips, found := dnsCache[host]
		dnsCacheMux.RUnlock()

		if found && len(ips) > 0 {
			for _, ip := range ips {
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return conn, nil
				}
			}
		}
		return dialer.DialContext(ctx, network, addr)
	}

	transport.ResponseHeaderTimeout = dialTimeout

	client := &http.Client{
		Transport: transport,
		Timeout:   time.Duration(timeoutSecs) * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return client, rotator, nil
}
