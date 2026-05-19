package vision

import (
	"context"

	screensensor "jarvis/internal/sensors/screen"
	"jarvis/internal/world"
)

type Analyzer interface {
	AnalyzeImage(ctx context.Context, prompt string, imagePath string) (string, error)
}

type Sensor struct {
	state        *world.WorldState
	screenSensor *screensensor.Sensor
	analyzer     Analyzer
}

type Options struct {
	State        *world.WorldState
	ScreenSensor *screensensor.Sensor
	Analyzer     Analyzer
}

type AnalyzeOptions struct {
	KeepDebugScreenshot bool
}
