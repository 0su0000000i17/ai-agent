package core

import (
	"context"
	"strings"

	"jarvis/internal/types"
)

func (a *Agent) Chat(ctx context.Context, message string) (string, error) {
	system := `Ты JARVIS. Отвечай как обычный помощник.
Не утверждай, что ты посмотрел экран или выполнил действие, если этого не было.`

	raw, err := a.model.Send([]types.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: message},
	})
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(raw), nil
}
