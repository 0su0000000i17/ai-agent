package ocr

import (
	screensensor "jarvis/internal/sensors/screen"
	"jarvis/internal/world"
)

type Sensor struct {
	state        *world.WorldState
	screenSensor *screensensor.Sensor
}

type Options struct {
	State        *world.WorldState
	ScreenSensor *screensensor.Sensor
}

type OCRCaptureOptions struct {
	KeepDebugScreenshot bool
}
