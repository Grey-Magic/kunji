package client

import (
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRandomUserAgent(t *testing.T) {
	userAgents := []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Mobile/15E148 Safari/604.1",
	}

	result := GetRandomUserAgent()
	assert.Contains(t, userAgents, result, "should return a valid user agent")

	for i := 0; i < 10; i++ {
		ua := GetRandomUserAgent()
		assert.NotEmpty(t, ua, "should return non-empty user agent")
	}
}

func TestNewProxyRotator_EmptyInput(t *testing.T) {
	rotator, err := NewProxyRotator("")

	require.NoError(t, err)
	assert.NotNil(t, rotator)
	assert.Nil(t, rotator.proxies)
	assert.Equal(t, 0, len(rotator.proxies))
}

func TestNewProxyRotator_SingleProxy(t *testing.T) {
	rotator, err := NewProxyRotator("http://proxy.example.com:8080")

	require.NoError(t, err)
	assert.NotNil(t, rotator)
	assert.Len(t, rotator.proxies, 1)
}

func TestNewProxyRotator_ProxyWithCredentials(t *testing.T) {
	rotator, err := NewProxyRotator("http://user:pass@proxy.example.com:8080")

	require.NoError(t, err)
	assert.NotNil(t, rotator)
	assert.Len(t, rotator.proxies, 1)
	assert.Equal(t, "proxy.example.com:8080", rotator.proxies[0].Host)
}

func TestNewProxyRotator_SOCKS5(t *testing.T) {
	rotator, err := NewProxyRotator("socks5://proxy.example.com:1080")

	require.NoError(t, err)
	assert.NotNil(t, rotator)
	assert.Len(t, rotator.proxies, 1)
}

func TestNewProxyRotator_NoPrefix(t *testing.T) {
	rotator, err := NewProxyRotator("proxy.example.com:8080")

	require.NoError(t, err)
	assert.NotNil(t, rotator)
	assert.Len(t, rotator.proxies, 1)
	assert.Equal(t, "http", rotator.proxies[0].Scheme)
}

func TestNewProxyRotator_MultipleProxies(t *testing.T) {

	content := "http://proxy1.com:8080\nhttp:
	tmpFile, err := os.CreateTemp("", "proxies*.txt")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	tmpFile.Close()

	rotator, err := NewProxyRotator(tmpFile.Name())

	require.NoError(t, err)
	assert.NotNil(t, rotator)
	assert.Len(t, rotator.proxies, 3)
}

func TestProxyRotator_GetProxy(t *testing.T) {
	proxy1, _ := url.Parse("http://proxy1.com:8080")
	proxy2, _ := url.Parse("http://proxy2.com:8080")
	proxy3, _ := url.Parse("http://proxy3.com:8080")

	rotator := &ProxyRotator{
		proxies: []*url.URL{proxy1, proxy2, proxy3},
	}

	p1, _ := rotator.GetProxy(nil)
	assert.Equal(t, "proxy1.com:8080", p1.Host)

	p2, _ := rotator.GetProxy(nil)
	assert.Equal(t, "proxy2.com:8080", p2.Host)

	p3, _ := rotator.GetProxy(nil)
	assert.Equal(t, "proxy3.com:8080", p3.Host)

	p4, _ := rotator.GetProxy(nil)
	assert.Equal(t, "proxy1.com:8080", p4.Host, "should cycle back to first proxy")
}

func TestProxyRotator_GetProxy_Empty(t *testing.T) {
	rotator := &ProxyRotator{}

	proxy, err := rotator.GetProxy(nil)
	assert.NoError(t, err)
	assert.Nil(t, proxy)
}

func TestNewHTTPClient(t *testing.T) {
	client, rotator, err := NewHTTPClient("", 30)

	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.NotNil(t, rotator)
	assert.NotNil(t, client.Transport)
	assert.Equal(t, 30*time.Second, client.Timeout)
}

func TestNewHTTPClient_WithProxy(t *testing.T) {
	client, rotator, err := NewHTTPClient("http://proxy.com:8080", 15)

	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.NotNil(t, rotator)
}
