package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"jarvis/internal/types"
)

type Model interface {
	Send(messages []types.Message) (string, error)
}

type Planner struct {
	model Model
}

func NewPlanner(model Model) *Planner {
	return &Planner{model: model}
}

func (p *Planner) Plan(ctx context.Context, userMessage string) (Decision, error) {
	if p == nil || p.model == nil {
		return Decision{}, fmt.Errorf("operation planner model is nil")
	}

	system := `Ты Operation Planner внутри Windows AI computer core.

Твоя задача — НЕ управлять приложениями напрямую.
Ты переводишь обычный запрос пользователя в универсальную операцию высокого уровня.

Возвращай СТРОГО JSON.

Типы:
- chat_reply — если действие на компьютере не нужно
- operation — если нужно действие

Операции:
- insert_text — вставить/напечатать текст в целевом окне
- replace_text — заменить весь текст в целевом текстовом поле/редакторе
- new_line — перейти на новую строку
- select_all — выделить весь текст
- save — сохранить
- copy — скопировать
- paste — вставить
- scroll — прокрутить
- open_app — открыть приложение

Формат operation:

{
  "type": "operation",
  "operation": {
    "kind": "replace_text",
    "target_hint": "блокнот",
    "app_query": "notepad",
    "text": "новый текст"
  },
  "reason": "пользователь просит заменить текст в блокноте"
}

Правила:
- target_hint — как пользователь назвал нужное окно/приложение: "блокнот", "телега", "браузер", "vscode".
- app_query — короткий Windows-запрос запуска приложения, если его нужно открыть: "notepad", "telegram", "chrome", "code".
- Если пользователь говорит "в блокноте" — target_hint="блокнот", app_query="notepad".
- Если пользователь просит "открой X" — open_app.
- "напечатай/введи/добавь текст" → insert_text.
- "замени текст на ..." → replace_text.
- "перейди на новую строку" → new_line.
- "выдели весь текст" → select_all.
- "сохрани" → save.
- "скопируй" → copy.
- "вставь" → paste.
- "прокрути вниз/вверх" → scroll.
- Не возвращай низкоуровневые hotkey/press_key/type_text.
- Не используй координаты.
- Не пиши markdown.
- Не оборачивай JSON в code fences.

Если пользователь просто общается — верни:
{
  "type": "chat_reply",
  "reply": "ответ",
  "reason": "действие на ПК не нужно"
}`

	raw, err := p.model.Send([]types.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: userMessage},
	})
	if err != nil {
		return Decision{}, err
	}

	decision, err := parseDecision(raw)
	if err != nil {
		return Decision{}, err
	}

	decision = normalizeDecision(decision)

	if decision.Type == "" {
		decision.Type = DecisionChatReply
	}

	return decision, nil
}

func parseDecision(raw string) (Decision, error) {
	clean := extractJSON(raw)

	var decision Decision
	if err := json.Unmarshal([]byte(clean), &decision); err != nil {
		return Decision{}, fmt.Errorf("cannot parse operation decision: %w. raw=%s", err, raw)
	}

	return decision, nil
}

func normalizeDecision(decision Decision) Decision {
	if decision.Type == "" {
		if decision.Operation.Kind != "" {
			decision.Type = DecisionOperation
		} else {
			decision.Type = DecisionChatReply
		}
	}

	decision.Operation.TargetHint = strings.TrimSpace(decision.Operation.TargetHint)
	decision.Operation.AppQuery = strings.TrimSpace(decision.Operation.AppQuery)
	decision.Operation.Text = strings.TrimSpace(decision.Operation.Text)
	decision.Operation.Direction = strings.TrimSpace(decision.Operation.Direction)

	return decision
}

func extractJSON(raw string) string {
	clean := strings.TrimSpace(raw)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)

	start := strings.Index(clean, "{")
	end := strings.LastIndex(clean, "}")

	if start >= 0 && end > start {
		return clean[start : end+1]
	}

	return clean
}
