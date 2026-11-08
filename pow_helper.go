package devkit

import (
	"context"
	"fmt"

	"github.com/ice-rider/ds2api/pow"
)

// powChallengeResponse — ответ эндпоинта /api/v0/chat/create_pow_challenge.
type powChallengeResponse struct {
	Data struct {
		BizCode int `json:"biz_code"`
		BizData struct {
			Challenge pow.Challenge `json:"challenge"` // <-- вложенный объект
		} `json:"biz_data"`
	} `json:"data"`
}

// preparePoW получает челлендж для targetPath и возвращает готовый заголовок
// x-ds-pow-response.
func (c *Client) preparePoW(ctx context.Context, targetPath string) (string, error) {
	body := map[string]string{"target_path": targetPath}

	var ch powChallengeResponse
	if err := c.httpClient().PostJSON(ctx, "/api/v0/chat/create_pow_challenge", body, &ch); err != nil {
		return "", fmt.Errorf("pow challenge: %w", err)
	}
	if ch.Data.BizCode != 0 {
		return "", fmt.Errorf("pow challenge: biz_code=%d", ch.Data.BizCode)
	}

	// Теперь передаём контекст и указатель на Challenge — как требует ds2api/pow.
	header, err := pow.SolveAndBuildHeader(ctx, &ch.Data.BizData.Challenge)
	if err != nil {
		return "", fmt.Errorf("pow solve: %w", err)
	}
	return header, nil
}
