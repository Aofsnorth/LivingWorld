package server

import (
	"time"

	"github.com/go-gl/mathgl/mgl32"
)

func bedrockLocalClientPosFromFeet(x, y, z float64) mgl32.Vec3 {
	return mgl32.Vec3{float32(x), float32(y + bedrockLocalEyeHeight), float32(z)}
}

func bedrockSharedFeetFromLocalClient(pos mgl32.Vec3) (x, y, z float64) {
	// The local Bedrock player is spawned/teleported with a camera-like Y
	// (feet + 1.62). Movement packets from this gophertunnel path remain in the
	// same visual coordinate space. Convert back to the shared feet coordinate
	// used by Java/world state; otherwise Java viewers see Bedrock players
	// floating about one eye-height above the grass.
	return float64(pos[0]), float64(pos[1]) - bedrockLocalEyeHeight, float64(pos[2])
}

func bedrockPosFromFeet(x, y, z float64) mgl32.Vec3 {
	// A remote player entity rendered for a Bedrock viewer needs the same visual
	// Y offset the local Bedrock client uses for its own render position â€”
	// regardless of the source edition. Without it the entity is drawn one
	// eye-height too low and appears sunk into the ground. This applies equally
	// to Bedrock-origin players (see bedrockPosFromJavaFeet for the Java case).
	return mgl32.Vec3{float32(x), float32(y + bedrockLocalEyeHeight), float32(z)}
}

func bedrockPosFromJavaFeet(x, y, z float64) mgl32.Vec3 {
	// Remote Java player entities in Bedrock need the same visual offset that
	// the local Bedrock client expects for player render positions. Without this
	// the Java player appears buried below the grass block.
	return mgl32.Vec3{float32(x), float32(y + bedrockLocalEyeHeight), float32(z)}
}

func (s *bedrockSession) movementUpdate(now time.Time, x, y, z float64) (publish bool, correct bool) {
	if s.lastMovePublish.IsZero() {
		s.lastMovePublish = now
		s.lastX, s.lastY, s.lastZ = x, y, z
		return true, false
	}
	dt := now.Sub(s.lastMovePublish).Seconds()
	if dt <= 0 {
		return false, false
	}
	s.lastMovePublish = now
	s.lastX, s.lastY, s.lastZ = x, y, z
	return true, false
}
