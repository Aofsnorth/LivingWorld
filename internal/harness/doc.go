// Package harness implements LivingWorld's application harness.
//
// Harness engineering is the discipline of designing the full environment a
// long-running application operates within: its lifecycle, the components that
// make it up, the order in which they start and stop, the health probes that
// observe them, the hooks that extend them, and the signals that govern their
// shutdown. The model is "Agent = Model + Harness" applied to a server: every
// concern that is not the domain logic itself belongs to the harness.
//
// The package is built around five small, focused abstractions (SOLID):
//
//   - Component   — the unit of composition. Each component owns one
//     responsibility (SRP) and is wired to its collaborators
//     through the Runtime, never by reaching into globals (DIP).
//   - Runtime     — the per-phase context handed to a component. It is a
//     context.Context that also exposes the logger, the metrics
//     recorder, and a typed lookup of sibling components
//     (dependency injection). Interfaces are kept narrow (ISP).
//   - Registry    — orders components by their declared dependencies
//     (topological sort) so startup is dependency-ordered and
//     shutdown is the reverse. New components extend the system
//     without modifying the harness (OCP).
//   - Health      — computational feedback sensors. Each component may expose
//     a probe; the harness aggregates them into a single report.
//   - Harness     — the orchestrator. It drives the lifecycle state machine,
//     runs phase hooks, coordinates graceful shutdown, and
//     rolls back partially-started components on failure.
//
// The harness is intentionally free of Minecraft-specific knowledge. The
// server package adapts its subsystems into Components and constructs a
// Harness via server.NewHarness.
package harness
