package computer

import (
	"net/url"
	"strings"
)

type ResolveKind string

const (
	ResolveKindUnknown ResolveKind = "unknown"
	ResolveKindWebsite ResolveKind = "website"
	ResolveKindApp     ResolveKind = "app"
	ResolveKindWindow  ResolveKind = "window"
	ResolveKindProcess ResolveKind = "process"
)

type ResolveResult struct {
	Resolved   bool        `json:"resolved"`
	Kind       ResolveKind `json:"kind"`
	Target     string      `json:"target,omitempty"`
	Display    string      `json:"display,omitempty"`
	Confidence float64     `json:"confidence"`
	Candidates []Candidate `json:"candidates,omitempty"`
	Reason     string      `json:"reason,omitempty"`
}

type Candidate struct {
	Kind       ResolveKind `json:"kind"`
	Target     string      `json:"target"`
	Display    string      `json:"display"`
	Confidence float64     `json:"confidence"`
	Reason     string      `json:"reason,omitempty"`
}

type Resolver struct {
	state *RuntimeState
}

func NewResolver(state *RuntimeState) *Resolver {
	return &Resolver{state: state}
}

func (r *Resolver) Resolve(input string) ResolveResult {
	input = normalize(input)
	if input == "" {
		return ResolveResult{
			Resolved: false,
			Kind:     ResolveKindUnknown,
			Reason:   "empty_input",
		}
	}

	candidates := make([]Candidate, 0)

	candidates = append(candidates, r.resolveFromObservedWindows(input)...)
	candidates = append(candidates, r.resolveFromObservedProcesses(input)...)

	if looksLikeDomain(input) {
		target := input
		if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
			target = "https://" + target
		}

		candidates = append(candidates, Candidate{
			Kind:       ResolveKindWebsite,
			Target:     target,
			Display:    target,
			Confidence: 0.78,
			Reason:     "looks_like_domain",
		})
	}

	return chooseBest(input, candidates)
}

func (r *Resolver) resolveFromObservedWindows(input string) []Candidate {
	if r.state == nil {
		return nil
	}

	snapshot := r.state.Snapshot()
	if snapshot.LastObservation == nil {
		return nil
	}

	result := make([]Candidate, 0)

	for _, w := range snapshot.LastObservation.Windows {
		title := normalize(w.Title)
		process := normalize(w.ProcessName)

		score := fuzzyScore(input, title, process)
		if score <= 0 {
			continue
		}

		result = append(result, Candidate{
			Kind:       ResolveKindWindow,
			Target:     w.ProcessName,
			Display:    w.Title,
			Confidence: score,
			Reason:     "observed_window",
		})
	}

	return result
}

func (r *Resolver) resolveFromObservedProcesses(input string) []Candidate {
	if r.state == nil {
		return nil
	}

	snapshot := r.state.Snapshot()
	if snapshot.LastObservation == nil {
		return nil
	}

	result := make([]Candidate, 0)

	for _, p := range snapshot.LastObservation.Processes {
		name := normalize(p.Name)

		score := fuzzyScore(input, name)
		if score <= 0 {
			continue
		}

		result = append(result, Candidate{
			Kind:       ResolveKindProcess,
			Target:     p.Name,
			Display:    p.Name,
			Confidence: score,
			Reason:     "observed_process",
		})
	}

	return result
}

func chooseBest(input string, candidates []Candidate) ResolveResult {
	if len(candidates) == 0 {
		return ResolveResult{
			Resolved: false,
			Kind:     ResolveKindUnknown,
			Reason:   "no_candidates",
		}
	}

	best := candidates[0]
	for _, c := range candidates {
		if c.Confidence > best.Confidence {
			best = c
		}
	}

	closeCandidates := make([]Candidate, 0)
	for _, c := range candidates {
		if c.Target != best.Target && c.Confidence >= best.Confidence-0.08 {
			closeCandidates = append(closeCandidates, c)
		}
	}

	if len(closeCandidates) > 0 && best.Confidence < 0.92 {
		return ResolveResult{
			Resolved:   false,
			Kind:       ResolveKindUnknown,
			Confidence: best.Confidence,
			Candidates: candidates,
			Reason:     "ambiguous_candidates",
		}
	}

	return ResolveResult{
		Resolved:   true,
		Kind:       best.Kind,
		Target:     best.Target,
		Display:    best.Display,
		Confidence: best.Confidence,
		Candidates: candidates,
		Reason:     best.Reason,
	}
}

func fuzzyScore(input string, values ...string) float64 {
	best := 0.0

	for _, value := range values {
		value = normalize(value)

		switch {
		case value == input:
			if best < 0.96 {
				best = 0.96
			}
		case strings.Contains(value, input):
			if best < 0.82 {
				best = 0.82
			}
		case strings.Contains(input, value):
			if best < 0.72 {
				best = 0.72
			}
		}
	}

	return best
}

func looksLikeDomain(input string) bool {
	if strings.Contains(input, " ") {
		return false
	}

	if strings.Contains(input, ".") {
		_, err := url.Parse("https://" + input)
		return err == nil
	}

	return false
}

func normalize(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "ё", "е")
	value = strings.ReplaceAll(value, "www.", "")
	value = strings.TrimSuffix(value, ".exe")
	value = strings.Join(strings.Fields(value), " ")
	return value
}
