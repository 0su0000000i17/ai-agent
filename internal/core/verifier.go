package core

import (
	"fmt"
	"strings"

	"jarvis/internal/actions"
	"jarvis/internal/scene"
)

type VerificationResult struct {
	Changed        bool
	BeforeWindow   string
	AfterWindow    string
	BeforeEntities int
	AfterEntities  int
	BeforeOCRLines int
	AfterOCRLines  int
	Summary        string
}

func VerifyAction(before scene.Scene, after scene.Scene, actionResult actions.Result) VerificationResult {
	result := VerificationResult{
		BeforeWindow:   before.Window.Title,
		AfterWindow:    after.Window.Title,
		BeforeEntities: len(before.Entities),
		AfterEntities:  len(after.Entities),
		BeforeOCRLines: len(before.OCRLines),
		AfterOCRLines:  len(after.OCRLines),
	}

	beforeText := sceneTextFingerprint(before)
	afterText := sceneTextFingerprint(after)

	result.Changed = beforeText != afterText ||
		result.BeforeWindow != result.AfterWindow ||
		result.BeforeEntities != result.AfterEntities ||
		result.BeforeOCRLines != result.AfterOCRLines

	var b strings.Builder

	b.WriteString("Verification\n")
	b.WriteString(fmt.Sprintf("- action_ok: %v\n", actionResult.OK))
	b.WriteString(fmt.Sprintf("- changed: %v\n", result.Changed))
	b.WriteString(fmt.Sprintf("- before_window: %s\n", emptyVerify(result.BeforeWindow)))
	b.WriteString(fmt.Sprintf("- after_window: %s\n", emptyVerify(result.AfterWindow)))
	b.WriteString(fmt.Sprintf("- entities: %d → %d\n", result.BeforeEntities, result.AfterEntities))
	b.WriteString(fmt.Sprintf("- ocr_lines: %d → %d\n", result.BeforeOCRLines, result.AfterOCRLines))

	if actionResult.Error != "" {
		b.WriteString("- error: ")
		b.WriteString(actionResult.Error)
		b.WriteString("\n")
	}

	result.Summary = b.String()

	return result
}

func sceneTextFingerprint(s scene.Scene) string {
	parts := make([]string, 0, len(s.OCRLines)+len(s.Entities))

	for _, line := range s.OCRLines {
		text := strings.TrimSpace(line.Text)
		if text != "" {
			parts = append(parts, text)
		}
	}

	for _, entity := range s.Entities {
		label := strings.TrimSpace(entity.Label)
		if label != "" {
			parts = append(parts, label)
		}
	}

	return strings.Join(parts, "\n")
}

func emptyVerify(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "—"
	}

	return value
}
