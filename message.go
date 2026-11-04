package devkit

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// NewMessage отправляет сообщение в чат.
func (c *Client) NewMessage(
	ctx context.Context,
	chatID, prompt string,
	stream bool,
) (*NewMessageResult, <-chan string, error) {

	powHeader, err := c.preparePoW(ctx, "/api/v0/chat/completion")
	if err != nil {
		return nil, nil, err
	}

	var parentID interface{}
	if id := c.getLastMsgID(chatID); id != "" {
		parentID = id
	}

	body := map[string]interface{}{
		"chat_session_id":   chatID,
		"parent_message_id": parentID,
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
			_ = c.streamCompletion(ctx, chatID, body, powHeader, ch)
		}()
		return nil, ch, nil
	}

	res, err := c.syncCompletion(ctx, chatID, body, powHeader, prompt)
	if err != nil {
		return nil, nil, err
	}
	return res, nil, nil
}

// syncCompletion читает SSE и собирает полный ответ.
func (c *Client) syncCompletion(
	ctx context.Context,
	chatID string,
	body map[string]interface{},
	powHeader string,
	prompt string,
) (*NewMessageResult, error) {

	resp, err := c.hx.PostJSONStream(ctx, "/api/v0/chat/completion", body, powHeader)
	if err != nil {
		return nil, fmt.Errorf("completion: %w", err)
	}
	defer resp.Body.Close()

	var assistant strings.Builder
	var userMsgID, assistMsgID string

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
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

		if id, ok := evt["request_message_id"].(string); ok && id != "" {
			userMsgID = id
		}
		if id, ok := evt["response_message_id"].(string); ok && id != "" {
			assistMsgID = id
		}

		if txt, ok := extractContent(evt); ok {
			assistant.WriteString(txt)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if assistMsgID != "" {
		c.setLastMsgID(chatID, assistMsgID)
	}

	return &NewMessageResult{
		UserMessage:      &Message{ID: userMsgID, Role: "user", Content: prompt},
		AssistantMessage: &Message{ID: assistMsgID, Role: "assistant", Content: assistant.String()},
	}, nil
}

// streamCompletion читает SSE и отправляет текстовые дельты в канал.
func (c *Client) streamCompletion(
	ctx context.Context,
	chatID string,
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
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
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

		if id, ok := evt["response_message_id"].(string); ok && id != "" {
			c.setLastMsgID(chatID, id)
		}

		if txt, ok := extractContent(evt); ok {
			out <- txt
		}
	}
	return scanner.Err()
}

// extractContent достаёт текстовый кусок из события SSE.
//
// DeepSeek-веб присылает куски в поле "v". Поле "p" (path) говорит, к чему
// относится "v". Контентом считаем:
//   - p отсутствует/пуст        -> обычный чанк ответа
//   - p заканчивается на /content -> явный чанк ответа
// Пропускаем пути вида "response/thinking_content", "response/status" и т.п.
func extractContent(evt map[string]interface{}) (string, bool) {
	if p, ok := evt["p"].(string); ok && p != "" && !strings.HasSuffix(p, "/content") {
		return "", false
	}
	v, ok := evt["v"].(string)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

// StopGeneration останавливает генерацию ответа в указанной сессии.
//
// DeepSeek требует два идентификатора: chat_session_id и message_id,
// причём message_id — это идентификатор ответа ассистента
// (response_message_id из SSE), который ещё генерируется. Он хранится
// в Client.lastMsgID и обновляется на каждом событии стрима/синка.
//
// Если в сессии ещё не было ответов — вернёт ошибку: останавливать нечего.
// Метод не блокирует вызывающего: если нужно прервать текущий
// syncCompletion/streamCompletion, вызывайте из отдельной горутины.
func (c *Client) StopGeneration(ctx context.Context, chatID string) error {
	if chatID == "" {
		return fmt.Errorf("devkit: chatID is empty")
	}

	msgID := c.getLastMsgID(chatID)
	if msgID == "" {
		return fmt.Errorf("devkit: no active message in chat %q", chatID)
	}

	body := map[string]string{
		"chat_session_id": chatID,
		"message_id":      msgID,
	}

	if err := c.hx.PostJSON(ctx, "/api/v0/chat/stop_stream", body, nil); err != nil {
		return fmt.Errorf("stop generation: %w", err)
	}
	return nil
}