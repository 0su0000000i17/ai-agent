package ops

import (
	"fmt"
	"strings"

	"jarvis/internal/actions"
)

func Compile(operation Operation) ([]actions.Request, error) {
	switch operation.Kind {
	case OperationInsertText:
		if strings.TrimSpace(operation.Text) == "" {
			return nil, fmt.Errorf("insert_text requires text")
		}

		return []actions.Request{
			{
				Kind: actions.ActionTypeText,
				Text: operation.Text,
			},
		}, nil

	case OperationReplaceText:
		if strings.TrimSpace(operation.Text) == "" {
			return nil, fmt.Errorf("replace_text requires text")
		}

		return []actions.Request{
			{
				Kind: actions.ActionHotkey,
				Keys: []string{"ctrl", "a"},
			},
			{
				Kind:            actions.ActionTypeText,
				Text:            operation.Text,
				SkipTargetClick: true,
			},
		}, nil

	case OperationNewLine:
		return []actions.Request{
			{
				Kind: actions.ActionPressKey,
				Key:  "enter",
			},
		}, nil

	case OperationSelectAll:
		return []actions.Request{
			{
				Kind: actions.ActionHotkey,
				Keys: []string{"ctrl", "a"},
			},
		}, nil

	case OperationSave:
		return []actions.Request{
			{
				Kind: actions.ActionHotkey,
				Keys: []string{"ctrl", "s"},
			},
		}, nil

	case OperationCopy:
		return []actions.Request{
			{
				Kind: actions.ActionHotkey,
				Keys: []string{"ctrl", "c"},
			},
		}, nil

	case OperationPaste:
		return []actions.Request{
			{
				Kind: actions.ActionHotkey,
				Keys: []string{"ctrl", "v"},
			},
		}, nil

	case OperationScroll:
		direction := strings.TrimSpace(operation.Direction)
		if direction == "" {
			direction = "down"
		}

		return []actions.Request{
			{
				Kind:      actions.ActionScroll,
				Direction: direction,
				Amount:    4,
			},
		}, nil

	case OperationOpenApp:
		return []actions.Request{
			{
				Kind:   actions.ActionWait,
				WaitMS: 300,
			},
		}, nil

	default:
		return nil, fmt.Errorf("unsupported operation kind: %s", operation.Kind)
	}
}
