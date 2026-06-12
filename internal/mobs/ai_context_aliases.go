package mobs

import (
	aictx "livingworld/internal/mobs/ai/context"
)

// AIContext re-exports aicontext.AIContext as mobs.AIContext so the
// existing call sites in the world layer keep working without
// changing imports.
type AIContext = aictx.AIContext

// PlayerTarget re-exports aicontext.PlayerTarget as
// mobs.PlayerTarget.
type PlayerTarget = aictx.PlayerTarget

// SoundEmit re-exports aicontext.SoundEmit as mobs.SoundEmit so
// the world layer's mobSound listener signature is unchanged.
type SoundEmit = aictx.SoundEmit

// HitEffect re-exports aicontext.HitEffect as mobs.HitEffect for
// the same reason.
type HitEffect = aictx.HitEffect
