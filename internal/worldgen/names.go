// Package worldgen — names.go
//
// Small helper to look up a canonical block name by its id without making
// every test file in the package import livingworld/internal/world. The
// indirection exists only so tests can read names back from a generated
// chunk; production code should use world.StateName directly.

package worldgen

import "livingworld/internal/world"

func worldStateName(id int32) string { return world.StateName(id) }
