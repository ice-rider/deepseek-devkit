package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
)

type Client struct {
	token     string
	baseURL   string
	http      *http.Client
	transport *Transport
}

func New(token, baseURL string) *Client {
	return NewWithConfig(token, baseURL, Config{})
}

func NewWithConfig(token, baseURL string, cfg Config) *Client {
	return NewWithLogger(token, baseURL, cfg, nil)
}

func NewWithLogger(token, baseURL string, cfg Config, logger *log.Logger) *Client {
	cfg = cfg.withDefaults()
	t := NewTransport(nil, cfg, logger)
	return &Client{
		token:     token,
		baseURL:   baseURL,
		transport: t,
		http: &http.Client{
			Transport: t,
			Timeout:   cfg.Timeout,
		},
	}
}

// SetNetworkLogging toggles the logging/retry layer at runtime.
func (c *Client) SetNetworkLogging(enabled bool) {
	cfg := c.transport.Config()
	cfg.Enabled = enabled
	c.transport.SetConfig(cfg)
}

// SetConfig replaces the network configuration at runtime.
// Note: Timeout changes require a new Client.
func (c *Client) SetConfig(cfg Config) {
	c.transport.SetConfig(cfg)
}

func (c *Client) Config() Config { return c.transport.Config() }

func (c *Client) do(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "deepseek-devkit/0.1")
	return c.http.Do(req)
}

// GetJSON, GetJSONWithQuery, PostJSON, PostJSONStream — unchanged bodies,
// they keep calling c.do(req) as before.
// (Copy them verbatim from the current file.)

func (c *Client) GetJSON(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("http %d: %s", resp.StatusCode, body)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) GetJSONWithQuery(
	ctx context.Context,
	path string,
	query map[string]string,
	out interface{},
) error {
	u, err := url.Parse(c.baseURL + path)
	if err != nil {
		return err
	}
	q := u.Query()
	for k, v := range query {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("http %d: %s", resp.StatusCode, body)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) PostJSON(
	ctx context.Context,
	path string,
	body interface{},
	out interface{},
) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("http %d: %s", resp.StatusCode, raw)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// PostJSONStream отправляет запрос с PoW-заголовком и возвращает потоковый ответ.
func (c *Client) PostJSONStream(
	ctx context.Context,
	path string,
	body interface{},
	powHeader string,
) (*http.Response, error) {	
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-ds-pow-response", powHeader)
	// Таймаут для стриминга должен быть больше или отсутствовать;
	// здесь используется клиент с общим таймаутом.
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, raw)
	}
	return resp, nil
}
