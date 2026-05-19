package screen

import "jarvis/internal/world"

type Sensor struct {
	state *world.WorldState
}

type Options struct {
	State *world.WorldState
}

type CaptureOptions struct {
	KeepFile bool
}
