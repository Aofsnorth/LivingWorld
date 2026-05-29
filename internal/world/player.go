package world

type Player struct {
	UUID     uint64
	Username string
	World    *World
	Position Position
	Rotation Rotation
}

type Position struct {
	X, Y, Z float64
}

type Rotation struct {
	Pitch, Yaw float32
}
