package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"jarvis/internal/actions"
	"jarvis/internal/scene"
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

func (p *Planner) Plan(ctx context.Context, userGoal string, currentScene scene.Scene) (Decision, error) {
	return p.PlanWithHistory(ctx, userGoal, currentScene, nil)
}

func (p *Planner) PlanWithHistory(
	ctx context.Context,
	userGoal string,
	currentScene scene.Scene,
	history []string,
) (Decision, error) {
	if p == nil || p.model == nil {
		return Decision{}, fmt.Errorf("planner model is nil")
	}

	system := `Ты planner внутри Windows AI computer core.

Ты НЕ управляешь конкретными приложениями через сценарии.
Ты работаешь универсально:
goal → scene → next action → verifier → next scene.

Твоя задача:
- получить цель пользователя
- получить текущую Scene
- получить историю уже выполненных шагов
- решить следующий шаг
- вернуть СТРОГО валидный JSON

Разрешённые type:
- "action" — нужно выполнить действие на ПК
- "done" — цель уже достигнута
- "chat_reply" — действие не нужно или нужна уточняющая информация

Разрешённые actions.kind:
- focus_window
- click_entity
- type_text
- press_key
- hotkey
- scroll
- wait

Формат action:

{
  "type": "action",
  "actions": [
    {
      "kind": "type_text",
      "text": "текст"
    }
  ],
  "reason": "почему это следующий шаг"
}

Формат done:

{
  "type": "done",
  "reply": "Готово",
  "reason": "цель выполнена"
}

Формат chat_reply:

{
  "type": "chat_reply",
  "reply": "уточняющий вопрос или обычный ответ",
  "reason": "почему действие не выполняется"
}

Правила:
- Если цель уже выполнена по текущей сцене или истории — верни done.
- Если пользователь просит напечатать/ввести/вставить текст — type_text.
- Если пользователь просит перейти на новую строку — press_key enter.
- Если пользователь просит выделить весь текст — hotkey ["ctrl", "a"].
- Если пользователь просит заменить весь текст на новый — actions:
  1) hotkey ["ctrl", "a"]
  2) type_text с новым текстом.
- Если пользователь просит сохранить — hotkey ["ctrl", "s"].
- Если пользователь просит скопировать — hotkey ["ctrl", "c"].
- Если пользователь просит вставить — hotkey ["ctrl", "v"].
- Если пользователь просит прокрутить — scroll.
- Если пользователь просит кликнуть по видимому объекту — выбери entity_id.
- Не отправляй сообщения людям / формы / заказы / удаления без подтверждения.
- Не используй координаты напрямую.
- Не выдумывай элементы, которых нет в Scene.
- Возвращай только JSON. Без markdown. Без code fences.`

	user := fmt.Sprintf(
		"USER_GOAL:\n%s\n\nHISTORY:\n%s\n\nCURRENT_SCENE:\n%s",
		userGoal,
		compactHistory(history),
		compactSceneForPlanner(currentScene),
	)

	raw, err := p.model.Send([]types.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
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

	if decision.Type == DecisionAction {
		if len(decision.Actions) == 0 {
			return Decision{}, fmt.Errorf("planner returned action decision without actions")
		}

		for _, req := range decision.Actions {
			if err := validateAction(req); err != nil {
				return Decision{}, err
			}
		}
	}

	return decision, nil
}

func compactHistory(history []string) string {
	if len(history) == 0 {
		return "no previous steps"
	}

	limit := len(history)
	if limit > 8 {
		limit = 8
	}

	var b strings.Builder
	start := len(history) - limit
	if start < 0 {
		start = 0
	}

	for i := start; i < len(history); i++ {
		b.WriteString(fmt.Sprintf("- %s\n", strings.TrimSpace(history[i])))
	}

	return b.String()
}

func compactSceneForPlanner(s scene.Scene) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("window_title: %s\n", s.Window.Title))
	b.WriteString(fmt.Sprintf("process_name: %s\n", s.Window.ProcessName))
	b.WriteString(fmt.Sprintf("window_rect: %d,%d %dx%d\n", s.Window.X, s.Window.Y, s.Window.Width, s.Window.Height))
	b.WriteString(fmt.Sprintf("ui_elements_count: %d\n", len(s.UIElements)))
	b.WriteString(fmt.Sprintf("ocr_lines_count: %d\n", len(s.OCRLines)))
	b.WriteString(fmt.Sprintf("entities_count: %d\n\n", len(s.Entities)))

	if s.Vision != nil {
		b.WriteString("vision_summary:\n")
		b.WriteString(strings.TrimSpace(s.Vision.Summary))
		b.WriteString("\n\n")
	}

	if len(s.OCRLines) > 0 {
		b.WriteString("visible_text_lines:\n")

		limit := len(s.OCRLines)
		if limit > 30 {
			limit = 30
		}

		for i := 0; i < limit; i++ {
			text := strings.TrimSpace(s.OCRLines[i].Text)
			if text == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(text)
			b.WriteString("\n")
		}

		b.WriteString("\n")
	}

	b.WriteString("top_entities:\n")

	limit := len(s.Entities)
	if limit > 80 {
		limit = 80
	}

	for i := 0; i < limit; i++ {
		e := s.Entities[i]

		label := strings.TrimSpace(e.Label)
		if label == "" {
			label = "—"
		}

		description := strings.TrimSpace(e.Description)
		if description != "" {
			description = " desc=" + description
		}

		b.WriteString(fmt.Sprintf(
			"- id=%s kind=%s label=%q source=%s rect=%.0f,%.0f %.0fx%.0f conf=%.2f%s\n",
			e.ID,
			e.Kind,
			label,
			strings.Join(e.Source, "+"),
			e.X,
			e.Y,
			e.Width,
			e.Height,
			e.Confidence,
			description,
		))
	}

	if len(s.Entities) > limit {
		b.WriteString(fmt.Sprintf("- ...and %d more entities\n", len(s.Entities)-limit))
	}

	return b.String()
}

func parseDecision(raw string) (Decision, error) {
	clean := extractJSON(raw)

	var decision Decision
	if err := json.Unmarshal([]byte(clean), &decision); err != nil {
		return Decision{}, fmt.Errorf("cannot parse planner decision: %w. raw=%s", err, raw)
	}

	return decision, nil
}

func normalizeDecision(decision Decision) Decision {
	if decision.Type == "" {
		if len(decision.Actions) > 0 || decision.Action.Kind != "" {
			decision.Type = DecisionAction
		} else {
			decision.Type = DecisionChatReply
		}
	}

	if decision.Type != DecisionAction {
		return decision
	}

	if len(decision.Actions) == 0 && decision.Action.Kind != "" {
		decision.Actions = []actions.Request{decision.Action}
	}

	normalized := make([]actions.Request, 0, len(decision.Actions))

	for _, req := range decision.Actions {
		normalized = append(normalized, normalizeAction(req)...)
	}

	decision.Actions = normalized

	if len(decision.Actions) > 0 {
		decision.Action = decision.Actions[0]
	}

	return decision
}

func normalizeAction(req actions.Request) []actions.Request {
	switch req.Kind {
	case actions.ActionPressKey:
		key := strings.TrimSpace(req.Key)

		if looksLikeHotkey(key) {
			return []actions.Request{
				{
					Kind: actions.ActionHotkey,
					Keys: splitHotkey(key),
				},
			}
		}

		return []actions.Request{req}

	case actions.ActionHotkey:
		if len(req.Keys) == 0 && strings.TrimSpace(req.Key) != "" {
			req.Keys = splitHotkey(req.Key)
		}

		return []actions.Request{req}

	case actions.ActionScroll:
		if strings.TrimSpace(req.Direction) == "" {
			req.Direction = "down"
		}

		return []actions.Request{req}

	default:
		return []actions.Request{req}
	}
}

func looksLikeHotkey(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))

	return strings.Contains(v, "+") ||
		strings.Contains(v, "ctrl ") ||
		strings.Contains(v, "control ") ||
		strings.Contains(v, "shift ") ||
		strings.Contains(v, "alt ")
}

func splitHotkey(raw string) []string {
	raw = strings.ReplaceAll(raw, "+", " ")
	raw = strings.ReplaceAll(raw, ",", " ")

	parts := strings.Fields(raw)
	result := make([]string, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		result = append(result, part)
	}

	return result
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

func validateAction(req actions.Request) error {
	switch req.Kind {
	case actions.ActionFocusWindow:
		return nil

	case actions.ActionClickEntity:
		if strings.TrimSpace(req.EntityID) == "" {
			return fmt.Errorf("click_entity requires entity_id")
		}
		return nil

	case actions.ActionTypeText:
		if strings.TrimSpace(req.Text) == "" {
			return fmt.Errorf("type_text requires text")
		}
		return nil

	case actions.ActionPressKey:
		if strings.TrimSpace(req.Key) == "" {
			return fmt.Errorf("press_key requires key")
		}

		if looksLikeHotkey(req.Key) {
			return fmt.Errorf("press_key cannot be hotkey: %s", req.Key)
		}

		return nil

	case actions.ActionHotkey:
		if len(req.Keys) == 0 {
			return fmt.Errorf("hotkey requires keys")
		}
		return nil

	case actions.ActionScroll:
		return nil

	case actions.ActionWait:
		return nil

	default:
		return fmt.Errorf("unsupported action kind: %s", req.Kind)
	}
}
