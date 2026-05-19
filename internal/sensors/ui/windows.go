package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strings"
	"time"

	"jarvis/internal/world"
)

type UISnapshot struct {
	Window     world.WindowRef `json:"window"`
	Root       *UIElement      `json:"root,omitempty"`
	Elements   []UIElementFlat `json:"elements"`
	CapturedAt time.Time       `json:"captured_at"`
}

type UIElement struct {
	Name         string      `json:"name,omitempty"`
	ControlType  string      `json:"control_type,omitempty"`
	AutomationID string      `json:"automation_id,omitempty"`
	ClassName    string      `json:"class_name,omitempty"`
	Bounds       UIRect      `json:"bounds"`
	IsEnabled    bool        `json:"is_enabled"`
	Children     []UIElement `json:"children,omitempty"`
}

type UIElementFlat struct {
	ID           string  `json:"id"`
	Name         string  `json:"name,omitempty"`
	ControlType  string  `json:"control_type,omitempty"`
	AutomationID string  `json:"automation_id,omitempty"`
	ClassName    string  `json:"class_name,omitempty"`
	X            float64 `json:"x"`
	Y            float64 `json:"y"`
	Width        float64 `json:"width"`
	Height       float64 `json:"height"`
	IsEnabled    bool    `json:"is_enabled"`
	Depth        int     `json:"depth"`
}

type UIRect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

func NewSensor(opts Options) *Sensor {
	return &Sensor{
		state: opts.State,
	}
}

func (s *Sensor) SnapshotWindow(ctx context.Context, target world.WindowRef, opts SnapshotOptions) (UISnapshot, error) {
	if target.HWND == 0 {
		return UISnapshot{}, fmt.Errorf("invalid target hwnd")
	}

	maxDepth := opts.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 6
	}

	maxChildren := opts.MaxChildren
	if maxChildren <= 0 {
		maxChildren = 220
	}

	script := fmt.Sprintf(`
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
$OutputEncoding = [System.Text.UTF8Encoding]::new($false)

Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes

function Safe-Number($value) {
	if ($value -eq $null) { return 0 }
	if ([Double]::IsInfinity($value)) { return 0 }
	if ([Double]::IsNaN($value)) { return 0 }
	return $value
}

function Get-ControlTypeName($element) {
	try {
		$raw = $element.Current.ControlType.ProgrammaticName
		if ($raw -eq $null) { return "" }
		return $raw.Replace("ControlType.", "")
	} catch {
		return ""
	}
}

function Convert-Element($element, $level, $maxDepth, $maxChildren) {
	if ($null -eq $element) {
		return $null
	}

	$rect = $element.Current.BoundingRectangle

	$item = [ordered]@{
		name = $element.Current.Name
		control_type = Get-ControlTypeName $element
		automation_id = $element.Current.AutomationId
		class_name = $element.Current.ClassName
		bounds = [ordered]@{
			x = Safe-Number $rect.X
			y = Safe-Number $rect.Y
			width = Safe-Number $rect.Width
			height = Safe-Number $rect.Height
		}
		is_enabled = $element.Current.IsEnabled
		children = @()
	}

	if ($level -ge $maxDepth) {
		return $item
	}

	$children = $element.FindAll(
		[System.Windows.Automation.TreeScope]::Children,
		[System.Windows.Automation.Condition]::TrueCondition
	)

	$count = 0
	foreach ($child in $children) {
		if ($count -ge $maxChildren) {
			break
		}

		$converted = Convert-Element $child ($level + 1) $maxDepth $maxChildren
		if ($null -ne $converted) {
			$item.children += $converted
			$count++
		}
	}

	return $item
}

$hwnd = [IntPtr]%d
$root = [System.Windows.Automation.AutomationElement]::FromHandle($hwnd)

$result = [ordered]@{
	root = $null
}

if ($root -ne $null) {
	$result.root = Convert-Element $root 0 %d %d
}

$result | ConvertTo-Json -Depth 30 -Compress
`, target.HWND, maxDepth, maxChildren)

	cmd := exec.CommandContext(
		ctx,
		"powershell",
		"-STA",
		"-NoProfile",
		"-ExecutionPolicy",
		"Bypass",
		"-Command",
		script,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return UISnapshot{}, fmt.Errorf("%w: %s", err, string(out))
	}

	clean := strings.TrimSpace(string(out))
	if clean == "" {
		return UISnapshot{}, fmt.Errorf("empty UI snapshot")
	}

	clean = sanitizeJSONNumbers(clean)

	var parsed struct {
		Root *UIElement `json:"root"`
	}

	if err := json.Unmarshal([]byte(clean), &parsed); err != nil {
		return UISnapshot{}, fmt.Errorf("cannot parse UI snapshot: %w. raw=%s", err, clean)
	}

	sanitizeUIElement(parsed.Root)

	snapshot := UISnapshot{
		Window:     target,
		Root:       parsed.Root,
		CapturedAt: time.Now(),
	}

	if parsed.Root != nil {
		snapshot.Elements = flattenUI(*parsed.Root)
	}

	return snapshot, nil
}

func flattenUI(root UIElement) []UIElementFlat {
	result := make([]UIElementFlat, 0)

	var walk func(el UIElement, depth int)
	walk = func(el UIElement, depth int) {
		id := fmt.Sprintf("ui_%d", len(result)+1)

		result = append(result, UIElementFlat{
			ID:           id,
			Name:         strings.TrimSpace(el.Name),
			ControlType:  strings.TrimSpace(el.ControlType),
			AutomationID: strings.TrimSpace(el.AutomationID),
			ClassName:    strings.TrimSpace(el.ClassName),
			X:            el.Bounds.X,
			Y:            el.Bounds.Y,
			Width:        el.Bounds.Width,
			Height:       el.Bounds.Height,
			IsEnabled:    el.IsEnabled,
			Depth:        depth,
		})

		for _, child := range el.Children {
			walk(child, depth+1)
		}
	}

	walk(root, 0)

	return result
}

func sanitizeUIElement(el *UIElement) {
	if el == nil {
		return
	}

	el.Bounds.X = safeFloat(el.Bounds.X)
	el.Bounds.Y = safeFloat(el.Bounds.Y)
	el.Bounds.Width = safeFloat(el.Bounds.Width)
	el.Bounds.Height = safeFloat(el.Bounds.Height)

	for i := range el.Children {
		sanitizeUIElement(&el.Children[i])
	}
}

func safeFloat(v float64) float64 {
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return 0
	}

	return v
}

func sanitizeJSONNumbers(raw string) string {
	raw = strings.ReplaceAll(raw, ":Infinity", ":0")
	raw = strings.ReplaceAll(raw, ":-Infinity", ":0")
	raw = strings.ReplaceAll(raw, ":NaN", ":0")
	return raw
}
