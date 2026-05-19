package ui

import "jarvis/internal/world"

type Sensor struct {
	state *world.WorldState
}

type Options struct {
	State *world.WorldState
}

type SnapshotOptions struct {
	MaxDepth    int
	MaxChildren int
}
