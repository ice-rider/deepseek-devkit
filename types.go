package devkit

// User — информация о текущем пользователе.
type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Token string `json:"-"`
}

// Session — чат-сессия.
type Session struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	ModelType string  `json:"model_type"`
	UpdatedAt float64 `json:"updated_at"`
	Pinned    bool    `json:"pinned"`
}

// Message — сообщение в чате.
type Message struct {
	ID        string `json:"id"`
	Role      string `json:"role"` // "user" или "assistant"
	Content   string `json:"content"`
	CreatedAt int64  `json:"created_at"`
}

// NewMessageResult — результат отправки сообщения в синхронном режиме.
type NewMessageResult struct {
	UserMessage      *Message
	AssistantMessage *Message
}
