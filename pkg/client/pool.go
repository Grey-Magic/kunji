package client

import (
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

type HostPoolStats struct {
	Host            string
	ActiveConns     int64
	IdleConns       int64
	TotalRequests   int64
	TotalFailures   int64
	H2Multiplexed   int64
	AvgResponseTime time.Duration
}

type HostPoolConfig struct {
	MaxConnsPerHost     int
	MaxIdleConnsPerHost int
	IdleTimeout         time.Duration
}

var DefaultHostPoolConfig = HostPoolConfig{
	MaxConnsPerHost:     100,
	MaxIdleConnsPerHost: 20,
	IdleTimeout:         120 * time.Second,
}

type hostMetrics struct {
	requests     atomic.Int64
	failures     atomic.Int64
	h2Multiplex  atomic.Int64
	totalLatency atomic.Int64
	latencyCount atomic.Int64
}

type ConnectionPoolManager struct {
	config  HostPoolConfig
	metrics map[string]*hostMetrics
	mu      sync.RWMutex
}

func NewConnectionPoolManager(config HostPoolConfig) *ConnectionPoolManager {
	return &ConnectionPoolManager{
		config:  config,
		metrics: make(map[string]*hostMetrics),
	}
}

func (cpm *ConnectionPoolManager) getMetrics(host string) *hostMetrics {
	cpm.mu.RLock()
	m, ok := cpm.metrics[host]
	cpm.mu.RUnlock()
	if ok {
		return m
	}

	cpm.mu.Lock()
	defer cpm.mu.Unlock()
	if m, ok = cpm.metrics[host]; ok {
		return m
	}
	m = &hostMetrics{}
	cpm.metrics[host] = m
	return m
}

func (cpm *ConnectionPoolManager) RecordRequest(host string, latency time.Duration, statusCode int) {
	m := cpm.getMetrics(host)
	m.requests.Add(1)
	m.latencyCount.Add(1)
	m.totalLatency.Add(int64(latency))
	if statusCode >= 500 || statusCode == 0 {
		m.failures.Add(1)
	}
}

func (cpm *ConnectionPoolManager) RecordH2Multiplex(host string) {
	m := cpm.getMetrics(host)
	m.h2Multiplex.Add(1)
}

func (cpm *ConnectionPoolManager) Stats() []HostPoolStats {
	cpm.mu.RLock()
	defer cpm.mu.RUnlock()

	stats := make([]HostPoolStats, 0, len(cpm.metrics))
	for host, m := range cpm.metrics {
		reqs := m.requests.Load()
		failures := m.failures.Load()
		latCount := m.latencyCount.Load()
		avgLat := time.Duration(0)
		if latCount > 0 {
			avgLat = time.Duration(m.totalLatency.Load() / latCount)
		}
		stats = append(stats, HostPoolStats{
			Host:            host,
			TotalRequests:   reqs,
			TotalFailures:   failures,
			H2Multiplexed:   m.h2Multiplex.Load(),
			AvgResponseTime: avgLat,
		})
	}
	return stats
}

func (cpm *ConnectionPoolManager) BuildTransport(dialer *net.Dialer, proxyFunc func(*http.Request) (*url.URL, error)) *http.Transport {
	transport := &http.Transport{
		Proxy:                 proxyFunc,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          cpm.config.MaxConnsPerHost * 10,
		MaxIdleConnsPerHost:   cpm.config.MaxIdleConnsPerHost,
		MaxConnsPerHost:       cpm.config.MaxConnsPerHost,
		IdleConnTimeout:       cpm.config.IdleTimeout,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableKeepAlives:     false,
		WriteBufferSize:       4 << 10,
		ReadBufferSize:        4 << 10,
	}

	if dialer != nil {
		transport.DialContext = dialer.DialContext
	}

	return transport
}
