package devkit

import (
    "bufio"
    "context"
    "encoding/json"
    "fmt"
    "strings"
)

// NewMessage отправляет сообщение в чат.
// Если stream == false, метод блокируется до полного ответа и возвращает NewMessageResult.
// Если stream == true, возвращает канал, в который по мере поступления пишутся куски текста.
func (c *Client) NewMessage(
	ctx context.Context,
	chatID, prompt string,
	stream bool,
) (*NewMessageResult, <-chan string, error) {

	powHeader, err := c.preparePoW(ctx, "/api/v0/chat/completion")
	if err != nil {
		return nil, nil, err
	}

	body := map[string]interface{}{
		"chat_session_id":   chatID,
		"parent_message_id": nil,
		"model_type":        "default",
		"prompt":            prompt,
		"ref_file_ids":      []string{},
		"thinking_enabled":  false,
		"search_enabled":    false,
	}

	if stream {
		ch := make(chan string, 64)
		go func() {
			defer close(ch)
			if err := c.streamCompletion(ctx, body, powHeader, ch); err != nil {
				// ошибку можно передать через отдельный канал или лог
			}
		}()
		return nil, ch, nil
	}

	// Передаём prompt в syncCompletion.
	res, err := c.syncCompletion(ctx, body, powHeader, prompt)
	if err != nil {
		return nil, nil, err
	}
	return res, nil, nil
}

// syncCompletion собирает весь SSE-поток и возвращает готовые сообщения.
func (c *Client) syncCompletion(
	ctx context.Context,
	body map[string]interface{},
	powHeader string,
	prompt string, // <-- добавлен параметр
) (*NewMessageResult, error) {

	resp, err := c.hx.PostJSONStream(ctx, "/api/v0/chat/completion", body, powHeader)
	if err != nil {
		return nil, fmt.Errorf("completion: %w", err)
	}
	defer resp.Body.Close()

	var assistant strings.Builder
	var userMsg, assistMsg *Message

	// ... остальной код без изменений ...
	// В конце:
	userMsg = &Message{
		Role:    "user",
		Content: prompt, // теперь переменная определена
	}
	assistMsg = &Message{
		Role:    "assistant",
		Content: assistant.String(),
	}

	return &NewMessageResult{
		UserMessage:      userMsg,
		AssistantMessage: assistMsg,
	}, nil
}

// streamCompletion читает SSE и отправляет текстовые дельты в канал.
func (c *Client) streamCompletion(
    ctx context.Context,
    body map[string]interface{},
    powHeader string,
    out chan<- string,
) error {

    resp, err := c.hx.PostJSONStream(ctx, "/api/v0/chat/completion", body, powHeader)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    scanner := bufio.NewScanner(resp.Body)
    for scanner.Scan() {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
        }

        line := scanner.Text()
        if !strings.HasPrefix(line, "data:") {
            continue
        }
        data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
        if data == "" || data == "[DONE]" {
            continue
        }

        var evt map[string]interface{}
        if err := json.Unmarshal([]byte(data), &evt); err != nil {
            continue
        }

        if delta, ok := evt["delta"].(map[string]interface{}); ok {
            if content, ok := delta["content"].(string); ok && content != "" {
                out <- content
            }
        }
    }
    return scanner.Err()
}

// StopGeneration останавливает генерацию для указанной сессии.
func (c *Client) StopGeneration(ctx context.Context, chatID string) error {
    // Эндпоинт требует message_id, но для простоты можно опустить или доработать.
    // В реальном API: POST /api/v0/chat/stop_stream { chat_session_id, message_id }.
    return fmt.Errorf("not implemented")
}