package httpx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// Config controls the network-observability layer.
type Config struct {
	Enabled         bool
	LogRequests     bool
	LogResponses    bool
	LogBodies       bool
	LogStreaming    bool
	MaxBodyBytes    int
	Timeout         time.Duration
	MaxRetries      int
	RetryBackoff    time.Duration
	MaxRetryBackoff time.Duration
}

func (c Config) withDefaults() Config {
	if c.MaxBodyBytes <= 0 {
		c.MaxBodyBytes = 4096
	}
	if c.RetryBackoff <= 0 {
		c.RetryBackoff = 500 * time.Millisecond
	}
	if c.MaxRetryBackoff <= 0 {
		c.MaxRetryBackoff = 10 * time.Second
	}
	return c
}

// Transport is an http.RoundTripper that optionally logs and retries.
// When Config.Enabled is false it is a transparent pass-through.
type Transport struct {
	base http.RoundTripper
	log  *log.Logger
	cfg  atomic.Value // Config
}

func NewTransport(base http.RoundTripper, cfg Config, logger *log.Logger) *Transport {
	if base == nil {
		base = http.DefaultTransport
	}
	if logger == nil {
		logger = log.New(log.Writer(), "[devkit/http] ", log.LstdFlags|log.Lmicroseconds)
	}
	t := &Transport{base: base, log: logger}
	t.cfg.Store(cfg.withDefaults())
	return t
}

func (t *Transport) SetConfig(cfg Config) { t.cfg.Store(cfg.withDefaults()) }
func (t *Transport) Config() Config       { return t.cfg.Load().(Config) }

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	cfg := t.Config()
	if !cfg.Enabled {
		return t.base.RoundTrip(req)
	}

	// Snapshot the body so we can log it and replay it on retries.
	needBody := cfg.MaxRetries > 0 || (cfg.LogRequests && cfg.LogBodies)
	var reqBody []byte
	if needBody && req.Body != nil && req.Body != http.NoBody {
		b, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("httpx: read request body: %w", err)
		}
		reqBody = b
	}

	for attempt := 1; ; attempt++ {
		attemptReq := req.Clone(req.Context())
		if reqBody != nil {
			attemptReq.Body = io.NopCloser(bytes.NewReader(reqBody))
		}

		if cfg.LogRequests {
			t.logRequest(cfg, attemptReq, reqBody, attempt)
		}

		start := time.Now()
		resp, err := t.base.RoundTrip(attemptReq)
		elapsed := time.Since(start)

		if err != nil {
			if cfg.LogResponses {
				t.log.Printf("← ERROR %s %s (attempt %d, %s): %v",
					req.Method, req.URL.Redacted(), attempt, elapsed, err)
			}
			if t.shouldRetry(cfg, attempt, nil, err, req.Context()) {
				t.waitBackoff(cfg, attempt, req.Context())
				continue
			}
			return nil, err
		}

		if t.shouldRetry(cfg, attempt, resp, nil, req.Context()) {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
			_ = resp.Body.Close()
			if cfg.LogResponses {
				t.log.Printf("← %d %s %s (attempt %d, %s) — retrying",
					resp.StatusCode, req.Method, req.URL.Redacted(), attempt, elapsed)
			}
			t.waitBackoff(cfg, attempt, req.Context())
			continue
		}

		if cfg.LogResponses {
			t.log.Printf("← %d %s %s (attempt %d, %s)",
				resp.StatusCode, req.Method, req.URL.Redacted(), attempt, elapsed)
			resp.Body = &captureBody{
				rc:      resp.Body,
				cfg:     cfg,
				log:     t.log,
				start:   start,
				max:     cfg.MaxBodyBytes,
				reqDesc: fmt.Sprintf("%s %s", req.Method, req.URL.Redacted()),
			}
		}
		return resp, nil
	}
}

func (t *Transport) logRequest(cfg Config, req *http.Request, body []byte, attempt int) {
	var b strings.Builder
	fmt.Fprintf(&b, "→ %s %s (attempt %d)", req.Method, req.URL.Redacted(), attempt)
	if hdr := redactHeaders(req.Header); len(hdr) > 0 {
		fmt.Fprintf(&b, "\n  headers: %v", hdr)
	}
	if cfg.LogBodies && len(body) > 0 {
		fmt.Fprintf(&b, "\n  body: %s", truncate(body, cfg.MaxBodyBytes))
	}
	t.log.Print(b.String())
}

func redactHeaders(h http.Header) http.Header {
	out := make(http.Header, len(h))
	for k, v := range h {
		switch strings.ToLower(k) {
		case "authorization", "x-ds-pow-response", "cookie":
			out[k] = []string{"<redacted>"}
		default:
			out[k] = v
		}
	}
	return out
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + fmt.Sprintf("… (%d bytes total)", len(b))
}

func (t *Transport) shouldRetry(
	cfg Config, attempt int, resp *http.Response, err error, ctx context.Context,
) bool {
	if cfg.MaxRetries <= 0 || attempt > cfg.MaxRetries {
		return false
	}
	if ctx.Err() != nil {
		return false
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false
		}
		return true
	}
	if resp == nil {
		return false
	}
	switch resp.StatusCode {
	case http.StatusTooManyRequests,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}

func (t *Transport) waitBackoff(cfg Config, attempt int, ctx context.Context) {
	backoff := cfg.RetryBackoff << (attempt - 1)
	if backoff > cfg.MaxRetryBackoff || backoff <= 0 {
		backoff = cfg.MaxRetryBackoff
	}
	t.log.Printf("  retry backoff %s (attempt %d/%d)", backoff, attempt, cfg.MaxRetries)
	timer := time.NewTimer(backoff)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}

// captureBody keeps up to max bytes for logging and reports the total
// byte count + duration when the body is consumed or closed.
type captureBody struct {
	rc      io.ReadCloser
	cfg     Config
	log     *log.Logger
	start   time.Time
	max     int
	reqDesc string
	buf     bytes.Buffer
	total   int64
	done    atomic.Bool
}

func (b *captureBody) Read(p []byte) (int, error) {
	n, err := b.rc.Read(p)
	if n > 0 {
		b.total += int64(n)
		if b.cfg.LogBodies && b.buf.Len() < b.max {
			rem := b.max - b.buf.Len()
			if rem > n {
				rem = n
			}
			b.buf.Write(p[:rem])
		}
		if b.cfg.LogStreaming && b.cfg.LogBodies {
			chunk := p[:n]
			if len(chunk) > 256 {
				chunk = chunk[:256]
			}
			b.log.Printf("  %s chunk (%d): %q", b.reqDesc, n, chunk)
		}
	}
	if err == io.EOF {
		b.finish()
	}
	return n, err
}

func (b *captureBody) Close() error {
	b.finish()
	return b.rc.Close()
}

func (b *captureBody) finish() {
	if !b.done.CompareAndSwap(false, true) {
		return
	}
	if b.cfg.LogBodies && b.buf.Len() > 0 {
		suffix := ""
		if b.total > int64(b.buf.Len()) {
			suffix = " (truncated)"
		}
		b.log.Printf("  %s body%s: %s", b.reqDesc, suffix, b.buf.String())
	}
	b.log.Printf("  %s body complete: %d bytes in %s",
		b.reqDesc, b.total, time.Since(b.start))
}
