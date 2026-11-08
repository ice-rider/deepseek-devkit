package devkit

import (
	"context"
	"fmt"
	"sync"

	"github.com/ice-rider/deepseek-devkit/internal/httpx"
)

const defaultBaseURL = "https://chat.deepseek.com"

// Client is a DeepSeek web-API client.
type Client struct {
	token   string
	baseURL string

	cfgMu sync.RWMutex
	cfg   Config

	httpOnce sync.Once
	hx       *httpx.Client

	lastMsgID map[string]string
	msgMu     sync.Mutex
}

func NewClient(token string) *Client {
	return NewClientWithConfig(token, DefaultConfig())
}

func NewClientWithConfig(token string, cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	return &Client{
		token:     token,
		baseURL:   cfg.BaseURL,
		cfg:       cfg,
		lastMsgID: make(map[string]string),
	}
}

// SetNetworkLogging turns the detailed network log on or off at runtime.
// Safe to call before Authorize.
func (c *Client) SetNetworkLogging(enabled bool) {
	c.cfgMu.Lock()
	c.cfg.Network.Enabled = enabled
	c.cfgMu.Unlock()
	c.httpClient().SetNetworkLogging(enabled)
}

// SetNetworkConfig replaces the whole NetworkConfig at runtime.
func (c *Client) SetNetworkConfig(nc NetworkConfig) {
	c.cfgMu.Lock()
	c.cfg.Network = nc
	c.cfgMu.Unlock()
	c.httpClient().SetConfig(nc.toHTTPX())
}

// NetworkConfig returns a copy of the current NetworkConfig.
func (c *Client) NetworkConfig() NetworkConfig {
	c.cfgMu.RLock()
	defer c.cfgMu.RUnlock()
	return c.cfg.Network
}

func (c *Client) httpClient() *httpx.Client {
	c.httpOnce.Do(func() {
		c.cfgMu.RLock()
		nc := c.cfg.Network
		c.cfgMu.RUnlock()
		baseURL := c.baseURL
		if baseURL == "" {
			baseURL = defaultBaseURL
		}
		c.hx = httpx.NewWithLogger(c.token, baseURL, nc.toHTTPX(), nc.Logger)
	})
	return c.hx
}

func (c *Client) getLastMsgID(chatID string) string {
	c.msgMu.Lock()
	defer c.msgMu.Unlock()
	return c.lastMsgID[chatID]
}

func (c *Client) setLastMsgID(chatID, id string) {
	if id == "" {
		return
	}
	c.msgMu.Lock()
	defer c.msgMu.Unlock()
	c.lastMsgID[chatID] = id
}

// Authorize validates the token and returns the current user.
func (c *Client) Authorize(ctx context.Context) (*User, error) {
	if c.token == "" {
		return nil, fmt.Errorf("devkit: token is empty")
	}
	var raw struct {
		Data struct {
			BizData struct {
				User User `json:"user"`
			} `json:"biz_data"`
		} `json:"data"`
	}
	if err := c.httpClient().GetJSON(ctx, "/api/v0/users/current", &raw); err != nil {
		return nil, fmt.Errorf("authorize: %w", err)
	}
	raw.Data.BizData.User.Token = c.token
	return &raw.Data.BizData.User, nil
}

func (c *Client) GetCurrentUser(ctx context.Context) (*User, error) {
	return c.Authorize(ctx)
}