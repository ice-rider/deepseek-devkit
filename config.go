package devkit

import (
	"log"
	"time"

	"github.com/ice-rider/deepseek-devkit/internal/httpx"
)

// Config is the top-level configuration for a Client.
type Config struct {
	// BaseURL overrides the default DeepSeek endpoint.
	BaseURL string

	// Network controls the HTTP layer: logging, retries, timeouts.
	Network NetworkConfig
}

// NetworkConfig controls the optional network-observability layer.
//
// When Enabled is false the client behaves like a plain http.Client:
// no request/response logging, no body capture, no retries.
type NetworkConfig struct {
	// Enabled is the master switch. Set to true to turn on the
	// logging/retry layer.
	Enabled bool

	// LogRequests logs method, URL, headers and (optionally) body.
	LogRequests bool

	// LogResponses logs status, duration and (optionally) body.
	LogResponses bool

	// LogBodies includes request/response bodies in the log.
	// Bodies are truncated to MaxBodyBytes.
	LogBodies bool

	// LogStreaming logs individual SSE chunks as they arrive.
	// Very verbose; off by default.
	LogStreaming bool

	// MaxBodyBytes caps how many bytes of a body are kept for the log.
	MaxBodyBytes int

	// Logger receives log lines. If nil, a default logger is created.
	// NOTE: the logger is captured when the HTTP client is first used;
	// changing it later via SetNetworkConfig has no effect.
	Logger *log.Logger

	// Timeout is the http.Client.Timeout. 0 means no client-level
	// timeout; callers should pass a context deadline instead.
	// Keep it 0 for long-running streaming requests.
	Timeout time.Duration

	// MaxRetries is the number of additional attempts after a
	// transient failure (429 / 502 / 503 / 504 / network error).
	// 0 disables retries.
	MaxRetries int

	// RetryBackoff is the base exponential backoff. Default 500ms.
	RetryBackoff time.Duration

	// MaxRetryBackoff caps the backoff. Default 10s.
	MaxRetryBackoff time.Duration
}

// DefaultConfig returns a Config with the default DeepSeek endpoint
// and the network layer disabled (production-safe).
func DefaultConfig() Config {
	return Config{
		BaseURL: defaultBaseURL,
		Network: DefaultNetworkConfig(),
	}
}

// DefaultNetworkConfig returns a NetworkConfig with logging and retries off.
func DefaultNetworkConfig() NetworkConfig {
	return NetworkConfig{
		Enabled:         false,
		LogRequests:     true,
		LogResponses:    true,
		LogBodies:       true,
		LogStreaming:    false,
		MaxBodyBytes:    4096,
		Timeout:         0, // rely on caller context; safe for SSE
		MaxRetries:      2,
		RetryBackoff:    500 * time.Millisecond,
		MaxRetryBackoff: 10 * time.Second,
	}
}

// DebugNetworkConfig returns a NetworkConfig with full logging enabled.
// Useful when you want to see exactly what goes over the wire.
func DebugNetworkConfig() NetworkConfig {
	c := DefaultNetworkConfig()
	c.Enabled = true
	c.LogStreaming = true
	return c
}

func (n NetworkConfig) toHTTPX() httpx.Config {
	return httpx.Config{
		Enabled:         n.Enabled,
		LogRequests:     n.LogRequests,
		LogResponses:    n.LogResponses,
		LogBodies:       n.LogBodies,
		LogStreaming:    n.LogStreaming,
		MaxBodyBytes:    n.MaxBodyBytes,
		Timeout:         n.Timeout,
		MaxRetries:      n.MaxRetries,
		RetryBackoff:    n.RetryBackoff,
		MaxRetryBackoff: n.MaxRetryBackoff,
	}
}