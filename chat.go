package devkit

import (
    "context"
    "fmt"
)

// NewChat создаёт новую чат-сессию.
func (c *Client) NewChat(ctx context.Context) (*Session, error) {
    var raw struct {
        Data struct {
            BizData struct {
                ChatSession Session `json:"chat_session"`
            } `json:"biz_data"`
        } `json:"data"`
    }

    if err := c.hx.PostJSON(ctx, "/api/v0/chat_session/create", map[string]interface{}{}, &raw); err != nil {
        return nil, fmt.Errorf("new chat: %w", err)
    }
    return &raw.Data.BizData.ChatSession, nil
}

// GetListOfChats возвращает список чатов (первую страницу).
func (c *Client) GetListOfChats(ctx context.Context) ([]*Session, error) {
    var raw struct {
        Data struct {
            BizData struct {
                ChatSessions []*Session `json:"chat_sessions"`
            } `json:"biz_data"`
        } `json:"data"`
    }

    query := map[string]string{
        "count": "50",
        "mode":  "lte",
    }
    if err := c.hx.GetJSONWithQuery(ctx, "/api/v0/chat_session/fetch_page", query, &raw); err != nil {
        return nil, fmt.Errorf("list chats: %w", err)
    }
    return raw.Data.BizData.ChatSessions, nil
}

// GetChatById возвращает историю сообщений указанной сессии.
func (c *Client) GetChatById(ctx context.Context, chatID string) ([]*Message, error) {
    var raw struct {
        Data struct {
            BizData struct {
                ChatMessages []*Message `json:"chat_messages"`
            } `json:"biz_data"`
        } `json:"data"`
    }

    query := map[string]string{"chat_session_id": chatID}
    if err := c.hx.GetJSONWithQuery(ctx, "/api/v0/chat/history_messages", query, &raw); err != nil {
        return nil, fmt.Errorf("get chat: %w", err)
    }
    return raw.Data.BizData.ChatMessages, nil
}

// RenameChat переименовывает чат.
func (c *Client) RenameChat(ctx context.Context, chatID, newTitle string) error {
    body := map[string]string{
        "chat_session_id": chatID,
        "title":           newTitle,
    }
    return c.hx.PostJSON(ctx, "/api/v0/chat_session/update_title", body, nil)
}

// DeleteChat удаляет чат.
func (c *Client) DeleteChat(ctx context.Context, chatID string) error {
    body := map[string][]string{
        "chat_session_ids": {chatID},
    }
    return c.hx.PostJSON(ctx, "/api/v0/chat_session/delete", body, nil)
}