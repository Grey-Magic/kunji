package client

import (
	"bufio"
	"crypto/rand"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
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

	if _, err := os.Stat(proxyInput); err == nil {
		file, err := os.Open(proxyInput)
		if err != nil {
			return nil, err
		}
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
	} else {
		line := proxyInput
		if !strings.HasPrefix(line, "http") && !strings.HasPrefix(line, "socks5") {
			line = "http://" + line
		}
		u, err := url.Parse(line)
		if err == nil {
			urls = append(urls, u)
		}
	}

	return &ProxyRotator{proxies: urls}, nil
}

func (pr *ProxyRotator) GetProxy(req *http.Request) (*url.URL, error) {
	if len(pr.proxies) == 0 {
		return nil, nil
	}
	pr.mux.Lock()
	defer pr.mux.Unlock()
	p := pr.proxies[pr.index]
	pr.index = (pr.index + 1) % len(pr.proxies)
	return p, nil
}

func NewHTTPClient(proxyStr string, timeoutSecs int) (*http.Client, error) {
	rotator, err := NewProxyRotator(proxyStr)
	if err != nil {
		return nil, err
	}

	dialTimeout := time.Duration(timeoutSecs) * time.Second
	if dialTimeout < 10*time.Second {
		dialTimeout = 10 * time.Second
	}

	transport := &http.Transport{
		Proxy: rotator.GetProxy,
		DialContext: (&net.Dialer{
			Timeout:       dialTimeout,
			KeepAlive:     30 * time.Second,
			FallbackDelay: -1,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          2000,
		MaxIdleConnsPerHost:   200,
		MaxConnsPerHost:       200,
		IdleConnTimeout:       120 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: dialTimeout,
		DisableKeepAlives:     false,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   time.Duration(timeoutSecs) * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return client, nil
}
