// Package player is LivingWorld's canonical, cross-edition player
// model. It owns the shared player state (position, health, hunger,
// inventory, XP, effects, gamemode, view, etc.) and the per-edition
// Controller abstraction that lets the shared model act on a
// connected client without depending on any protocol code.
//
// As with package world, the Java and Bedrock edges share ONE
// *Manager so they observe the same canonical state. The shared
// state is held in Player structs keyed by UUID; the edges register
// a Controller for each connected session and use the bus
// (OnBlockUpdate, OnItemPickup, OnDamage, etc.) to react to changes
// initiated by the other edition.
//
// Phase 0/1 contract: gameplay logic (drop pickup, knockback, respawn)
// lives in this package. The edges translate packets; they MUST NOT
// re-implement gameplay rules.
package player
