package computer

import (
	"fmt"
	"sort"
	"strings"
)

type WindowGroup struct {
	ProcessName string   `json:"process_name"`
	Titles      []string `json:"titles"`
	Count       int      `json:"count"`
}

func FormatObservationNeutral(obs Observation) string {
	var b strings.Builder

	if obs.ActiveWindow != nil {
		b.WriteString("Активное окно:\n")
		b.WriteString(fmt.Sprintf("- %s [%s]\n\n", cleanTitle(obs.ActiveWindow.Title), obs.ActiveWindow.ProcessName))
	}

	groups := groupWindowsByProcess(obs.Windows)

	b.WriteString("Открытые окна:\n")
	if len(groups) == 0 {
		b.WriteString("- открытых окон не найдено\n")
	} else {
		for _, group := range groups {
			if group.Count == 1 {
				b.WriteString(fmt.Sprintf("- %s\n", group.ProcessName))
			} else {
				b.WriteString(fmt.Sprintf("- %s — %d окна\n", group.ProcessName, group.Count))
			}

			limit := len(group.Titles)
			if limit > 4 {
				limit = 4
			}

			for i := 0; i < limit; i++ {
				title := cleanTitle(group.Titles[i])
				if title == "" {
					continue
				}
				b.WriteString(fmt.Sprintf("  · %s\n", title))
			}

			if len(group.Titles) > limit {
				b.WriteString(fmt.Sprintf("  · ...ещё %d окон\n", len(group.Titles)-limit))
			}
		}
	}

	b.WriteString(fmt.Sprintf("\nВсего окон: %d\n", len(obs.Windows)))
	b.WriteString(fmt.Sprintf("Всего процессов: %d\n", len(obs.Processes)))

	return b.String()
}

func groupWindowsByProcess(windows []WindowInfo) []WindowGroup {
	byProcess := map[string]*WindowGroup{}

	for _, w := range windows {
		process := strings.TrimSpace(w.ProcessName)
		if process == "" {
			process = "unknown"
		}

		key := strings.ToLower(process)

		if _, ok := byProcess[key]; !ok {
			byProcess[key] = &WindowGroup{
				ProcessName: process,
				Titles:      []string{},
			}
		}

		byProcess[key].Titles = append(byProcess[key].Titles, w.Title)
		byProcess[key].Count = len(byProcess[key].Titles)
	}

	result := make([]WindowGroup, 0, len(byProcess))
	for _, group := range byProcess {
		sort.Strings(group.Titles)
		result = append(result, *group)
	}

	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].ProcessName) < strings.ToLower(result[j].ProcessName)
	})

	return result
}

func cleanTitle(title string) string {
	title = strings.TrimSpace(title)
	title = strings.ReplaceAll(title, "\u200e", "")
	title = strings.ReplaceAll(title, "\u200f", "")
	return title
}
