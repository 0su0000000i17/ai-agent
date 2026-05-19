package target

import (
	"context"
	"fmt"
	"strings"
	"time"

	"jarvis/internal/world"
)

func (m *Manager) ResolveByHint(ctx context.Context, targetHint string, appQuery string) (ResolveResult, error) {
	targetHint = strings.TrimSpace(targetHint)
	appQuery = strings.TrimSpace(appQuery)

	if m == nil || m.world == nil {
		return ResolveResult{}, fmt.Errorf("target manager is not initialized")
	}

	if targetHint == "" && appQuery == "" {
		return ResolveResult{
			HasTarget: false,
			Message:   "Не понял, в каком окне выполнять действие.",
		}, nil
	}

	snapshot := m.world.Snapshot()

	if window, ok := bestWindowMatch(snapshot, targetHint, appQuery); ok {
		return ResolveResult{
			HasTarget: true,
			Target: ResolvedTarget{
				Window:     window,
				Reason:     "matched by target hint/app query",
				Confidence: 0.86,
				Launched:   false,
			},
		}, nil
	}

	launchQuery := appQuery
	if launchQuery == "" {
		launchQuery = targetHint
	}

	if launchQuery == "" {
		return ResolveResult{
			HasTarget: false,
			Message:   "Не нашёл подходящее окно и не понял, что открыть.",
		}, nil
	}

	if err := launchAppQuery(ctx, launchQuery); err != nil {
		return ResolveResult{}, err
	}

	deadline := time.Now().Add(8 * time.Second)
	var previous = snapshot

	for time.Now().Before(deadline) {
		time.Sleep(650 * time.Millisecond)

		next, err := m.windowSensor.Observe(ctx, previous)
		if err == nil {
			previous = next
		} else {
			next = m.world.Snapshot()
		}

		if window, ok := bestWindowMatch(next, targetHint, appQuery); ok {
			return ResolveResult{
				HasTarget: true,
				Target: ResolvedTarget{
					Window:     window,
					Reason:     "launched and matched by target hint/app query",
					Confidence: 0.82,
					Launched:   true,
				},
			}, nil
		}
	}

	return ResolveResult{}, fmt.Errorf("could not resolve target after launch: hint=%q app_query=%q", targetHint, appQuery)
}

func bestWindowMatch(snapshot world.WindowSnapshot, targetHint string, appQuery string) (world.WindowRef, bool) {
	targetHint = normalizeForMatch(targetHint)
	appQuery = normalizeForMatch(appQuery)

	bestScore := 0
	var best world.WindowRef

	for _, window := range snapshot.VisibleWindows {
		if !isValidUserTarget(window) {
			continue
		}

		score := scoreWindow(window, targetHint, appQuery)
		if score > bestScore {
			bestScore = score
			best = window
		}
	}

	if bestScore < 35 {
		return world.WindowRef{}, false
	}

	return best, true
}

func scoreWindow(window world.WindowRef, targetHint string, appQuery string) int {
	title := normalizeForMatch(window.Title)
	process := normalizeForMatch(window.ProcessName)

	score := 0

	if targetHint != "" {
		score += scoreTextMatch(title, targetHint)
		score += scoreTextMatch(process, targetHint)
	}

	if appQuery != "" {
		score += scoreTextMatch(title, appQuery)
		score += scoreTextMatch(process, appQuery)
	}

	return score
}

func scoreTextMatch(haystack string, needle string) int {
	if haystack == "" || needle == "" {
		return 0
	}

	if haystack == needle {
		return 100
	}

	if strings.Contains(haystack, needle) {
		return 80
	}

	needleTokens := strings.Fields(needle)
	if len(needleTokens) == 0 {
		return 0
	}

	matches := 0
	for _, token := range needleTokens {
		if len([]rune(token)) < 2 {
			continue
		}

		if strings.Contains(haystack, token) {
			matches++
		}
	}

	if matches == 0 {
		return 0
	}

	return 20 + matches*10
}

func normalizeForMatch(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, ".exe", "")
	value = strings.ReplaceAll(value, "—", " ")
	value = strings.ReplaceAll(value, "-", " ")
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.ReplaceAll(value, ".", " ")
	value = strings.ReplaceAll(value, "|", " ")

	return strings.Join(strings.Fields(value), " ")
}
