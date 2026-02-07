package pool

import (
	"net"
	"net/http"
	"time"
)

// HTTPClientPool provides an optimized HTTP client with connection pooling
type HTTPClientPool struct {
	client *http.Client
}

// NewHTTPClientPool creates a new HTTP client with optimized settings
func NewHTTPClientPool() *HTTPClientPool {
	return &HTTPClientPool{
		client: &http.Client{
			Transport: &http.Transport{
				// Connection pooling settings
				MaxIdleConns:        100,              // Maximum idle connections across all hosts
				MaxIdleConnsPerHost: 10,               // Maximum idle connections per host
				MaxConnsPerHost:     50,               // Maximum connections per host (0 = unlimited)
				IdleConnTimeout:     90 * time.Second, // How long idle connections stay alive

				// TCP settings for better performance
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second, // Connection timeout
					KeepAlive: 30 * time.Second, // Keep-alive probe interval
					DualStack: true,             // Use IPv4 and IPv6
				}).DialContext,

				// TLS handshake timeout
				TLSHandshakeTimeout: 10 * time.Second,

				// Enable HTTP/2 (automatically enabled by default in Go 1.6+)
				ForceAttemptHTTP2: true,

				// Timeout for reading response headers
				ResponseHeaderTimeout: 30 * time.Second,

				// Expect-Continue timeout (for large uploads)
				ExpectContinueTimeout: 1 * time.Second,

				// Don't disable compression
				DisableCompression: false,
			},

			// Overall request timeout (can be overridden per request)
			Timeout: 5 * time.Minute,
		},
	}
}

// Client returns the underlying HTTP client
func (p *HTTPClientPool) Client() *http.Client {
	return p.client
}

// Close closes all idle connections
func (p *HTTPClientPool) Close() {
	p.client.CloseIdleConnections()
}

// Stats returns connection pool statistics
func (p *HTTPClientPool) Stats() map[string]interface{} {
	// Note: http.Transport doesn't expose internal stats directly
	// This is a placeholder for future implementation
	return map[string]interface{}{
		"max_idle_conns":          100,
		"max_idle_conns_per_host": 10,
		"max_conns_per_host":      50,
		"idle_conn_timeout":       "90s",
	}
}

// DefaultHTTPClient is a singleton instance of the optimized HTTP client
var DefaultHTTPClient = NewHTTPClientPool()
