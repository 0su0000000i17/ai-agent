package window

import "jarvis/internal/world"

type Sensor struct {
	selfPID int
	state   *world.WorldState
}

type Options struct {
	SelfPID int
	State   *world.WorldState
}
