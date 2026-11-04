package devkit

import (
    "context"
    "fmt"
    "net/http"
    "time"
    "sync"

    "github.com/ice-rider/deepseek-devkit/internal/httpx"
)

const defaultBaseURL = "https://chat.deepseek.com"

// devkit.go
type Client struct {
	token     string
	baseURL   string
	http      *http.Client
	hx        *httpx.Client
	lastMsgID map[string]string
	msgMu     sync.Mutex
}

func NewClient(token string) *Client {
	return &Client{
		token:     token,
		baseURL:   defaultBaseURL,
		http:      &http.Client{Timeout: 60 * time.Second},
		lastMsgID: make(map[string]string),
	}
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

// Authorize проверяет токен и инициализирует внутренний HTTP-клиент.
// Если токен пуст, возвращается ошибка.
func (c *Client) Authorize(ctx context.Context) (*User, error) {
    if c.token == "" {
        return nil, fmt.Errorf("devkit: token is empty")
    }
    c.hx = httpx.New(c.token, c.baseURL)

    var raw struct {
        Data struct {
            BizData struct {
                User User `json:"user"`
            } `json:"biz_data"`
        } `json:"data"`
    }

    if err := c.hx.GetJSON(ctx, "/api/v0/users/current", &raw); err != nil {
        return nil, fmt.Errorf("authorize: %w", err)
    }
    raw.Data.BizData.User.Token = c.token
    return &raw.Data.BizData.User, nil
}

// GetCurrentUser — псевдоним для Authorize, возвращает информацию о пользователе.
func (c *Client) GetCurrentUser(ctx context.Context) (*User, error) {
    return c.Authorize(ctx)
}