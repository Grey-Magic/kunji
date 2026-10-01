package client

import (
	"net"
	"net/http"
	"net/url"
	"time"
)

type HostPoolConfig struct {
	MaxConnsPerHost     int
	MaxIdleConnsPerHost int
	IdleTimeout         time.Duration
}

var DefaultHostPoolConfig = HostPoolConfig{
	MaxConnsPerHost:     100,
	MaxIdleConnsPerHost: 100,
	IdleTimeout:         120 * time.Second,
}

type ConnectionPoolManager struct {
	config HostPoolConfig
}

func NewConnectionPoolManager(config HostPoolConfig) *ConnectionPoolManager {
	return &ConnectionPoolManager{
		config: config,
	}
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
