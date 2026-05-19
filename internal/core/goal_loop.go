package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"jarvis/internal/actions"
	"jarvis/internal/planner"
)

type GoalLoopOptions struct {
	MaxSteps  int
	UseVision bool
}

func (a *Agent) RunGoalLoop(ctx context.Context, userGoal string, opts GoalLoopOptions) (string, bool) {
	if a == nil || a.planner == nil || a.targetManager == nil || a.sensorCore == nil {
		return "", false
	}

	maxSteps := opts.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 5
	}

	targetResult, err := a.targetManager.ResolveForGoal(ctx, userGoal)
	if err != nil {
		return "Target resolver error: " + err.Error(), true
	}

	if !targetResult.HasTarget {
		return "", false
	}

	targetWindow := targetResult.Target.Window

	history := []string{
		fmt.Sprintf(
			"resolved target: %s [%s], reason=%s, confidence=%.2f, launched=%v",
			targetWindow.Title,
			targetWindow.ProcessName,
			targetResult.Target.Reason,
			targetResult.Target.Confidence,
			targetResult.Target.Launched,
		),
	}

	var report strings.Builder

	report.WriteString("Goal Loop Report\n\n")
	report.WriteString("Goal:\n")
	report.WriteString(userGoal)
	report.WriteString("\n\n")

	report.WriteString("Initial target:\n")
	report.WriteString(fmt.Sprintf("- %s [%s]\n", targetWindow.Title, targetWindow.ProcessName))
	report.WriteString(fmt.Sprintf("- reason: %s\n", targetResult.Target.Reason))
	report.WriteString(fmt.Sprintf("- confidence: %.2f\n", targetResult.Target.Confidence))
	report.WriteString(fmt.Sprintf("- launched: %v\n\n", targetResult.Target.Launched))

	for step := 1; step <= maxSteps; step++ {
		sceneSnapshot, err := a.sensorCore.SceneForWindow(ctx, targetWindow, opts.UseVision)
		if err != nil {
			report.WriteString(fmt.Sprintf("Step %d scene error: %s\n", step, err.Error()))
			return report.String(), true
		}

		decision, err := a.planner.PlanWithHistory(ctx, userGoal, sceneSnapshot, history)
		if err != nil {
			report.WriteString(fmt.Sprintf("Step %d planner error: %s\n", step, err.Error()))
			return report.String(), true
		}

		report.WriteString(fmt.Sprintf("Step %d/%d\n", step, maxSteps))
		report.WriteString(fmt.Sprintf("- planner_type: %s\n", decision.Type))
		report.WriteString(fmt.Sprintf("- reason: %s\n", strings.TrimSpace(decision.Reason)))

		switch decision.Type {
		case planner.DecisionDone:
			reply := strings.TrimSpace(decision.Reply)
			if reply == "" {
				reply = "Готово."
			}

			report.WriteString("- status: done\n")
			report.WriteString("- reply: ")
			report.WriteString(reply)
			report.WriteString("\n")

			return report.String(), true

		case planner.DecisionChatReply:
			reply := strings.TrimSpace(decision.Reply)
			if reply == "" {
				reply = "Нужно уточнение."
			}

			report.WriteString("- status: chat_reply\n")
			report.WriteString("- reply: ")
			report.WriteString(reply)
			report.WriteString("\n")

			return report.String(), true

		case planner.DecisionAction:
			requests := decision.Actions
			if len(requests) == 0 && decision.Action.Kind != "" {
				requests = []actions.Request{decision.Action}
			}

			if len(requests) == 0 {
				report.WriteString("- status: planner returned empty action list\n")
				return report.String(), true
			}

			report.WriteString(fmt.Sprintf("- actions: %d\n\n", len(requests)))

			actionReport := a.sensorCore.ActionSequenceForScene(
				ctx,
				sceneSnapshot,
				requests,
				opts.UseVision,
			)

			report.WriteString(actionReport)
			report.WriteString("\n\n")

			history = append(history, summarizeLoopStep(step, decision, actionReport))

			// Retarget after action. If action opened/changed window, resolver can pick the right one again.
			updatedTarget, err := a.targetManager.ResolveForGoal(ctx, userGoal)
			if err == nil && updatedTarget.HasTarget {
				targetWindow = updatedTarget.Target.Window
				history = append(history, fmt.Sprintf(
					"retargeted: %s [%s], reason=%s, confidence=%.2f",
					targetWindow.Title,
					targetWindow.ProcessName,
					updatedTarget.Target.Reason,
					updatedTarget.Target.Confidence,
				))
			}

			time.Sleep(350 * time.Millisecond)

		default:
			report.WriteString(fmt.Sprintf("- status: unsupported planner type %s\n", decision.Type))
			return report.String(), true
		}
	}

	report.WriteString("Goal loop stopped: max steps reached.\n")
	return report.String(), true
}

func summarizeLoopStep(step int, decision planner.Decision, actionReport string) string {
	actionKinds := make([]string, 0, len(decision.Actions))

	for _, action := range decision.Actions {
		actionKinds = append(actionKinds, string(action.Kind))
	}

	summary := strings.TrimSpace(actionReport)
	runes := []rune(summary)
	if len(runes) > 800 {
		summary = string(runes[:800]) + "...truncated..."
	}

	return fmt.Sprintf(
		"step=%d type=%s actions=%s reason=%s report=%s",
		step,
		decision.Type,
		strings.Join(actionKinds, ","),
		strings.TrimSpace(decision.Reason),
		summary,
	)
}
