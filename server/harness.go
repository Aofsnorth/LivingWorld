package server

import (
	"io"
	"os"

	"livingworld/internal/harness"
)

// NewHarness builds a harness.Harness that drives this server's lifecycle.
//
// The server is registered as a single component (key "livingworld.server")
// whose Start/Stop delegate to the existing Server.Start/Stop methods, and a
// health probe reports listener readiness and live player count. The
// user-facing "Shutting down…" / "stopped" log lines are installed as
// before/after-stop hooks so they fire regardless of whether shutdown is
// triggered by signal, console, or an explicit Stop call.
//
// The returned harness has signal handling enabled (SIGINT/SIGTERM). Callers
// that own their own event loop — notably the TUI — should pass
// harness.WithNoopSignals() and drive shutdown via Harness.Stop.
//
// Callers may register additional components (e.g. the operator console) on
// the returned harness before calling Start or Run. This is the public
// extensibility seam: an embedder can plug in metrics, extra probes, or
// sidecar services without touching the server package.
func (s *Server) NewHarness(opts ...harness.Option) *harness.Harness {
	h := harness.New(append([]harness.Option{harness.WithLogger(s.logger)}, opts...)...)
	_ = h.Register(serverComponent{srv: s})
	h.OnHook(harness.PhaseBeforeStop, func(rt harness.Runtime) error {
		rt.Logger().Info("Shutting down LivingWorld...")
		return nil
	})
	h.OnHook(harness.PhaseAfterStop, func(rt harness.Runtime) error {
		rt.Logger().Info("LivingWorld stopped")
		return nil
	})
	return h
}

// serverComponent adapts *Server to the harness.Component interface. The
// server's wiring is performed in New (the composition root), so Init is a
// no-op; Start/Stop delegate to the existing methods. This keeps the harness
// integration non-invasive: the server's internals are unchanged, only the
// run path is re-hosted.
type serverComponent struct {
	srv *Server
}

func (c serverComponent) Key() string { return "livingworld.server" }

// Init is a no-op: New already wired every subsystem. Keeping the seam lets
// future work move late setup here without changing the harness contract.
func (c serverComponent) Init(harness.Runtime) error { return nil }

// Start delegates to Server.Start, which enables persistence, loads script
// plugins, and brings up the Java and Bedrock listeners.
func (c serverComponent) Start(harness.Runtime) error { return c.srv.Start() }

// Stop delegates to Server.Stop, which saves players, stops both listeners,
// and closes the worlds. Server.Stop is already idempotent, satisfying the
// harness's rollback requirement.
func (c serverComponent) Stop(harness.Runtime) error {
	c.srv.Stop()
	return nil
}

// Healthcheck reports listener readiness and live player count. It is the
// computational feedback sensor for the server: an orchestrator (or a future
// /health HTTP endpoint) can poll Harness.Health to decide whether the
// process is ready to receive traffic.
func (c serverComponent) Healthcheck(rt harness.Runtime) harness.Health {
	switch rt.State() {
	case harness.StateRunning:
		return harness.Health{
			Status: harness.HealthUp,
			Detail: map[string]any{
				"java":    c.srv.cfg.Address(),
				"bedrock": c.srv.cfg.BedrockAddress(),
				"players": c.srv.PlayerCount(),
			},
		}
	case harness.StateStopped, harness.StateStopping, harness.StateFailed:
		return harness.Health{Status: harness.HealthStopped}
	default:
		return harness.Health{Status: harness.HealthStarting}
	}
}

// consoleComponent wraps the operator console as a lifecycle component. Its
// "stop" command triggers harness shutdown via the stop callback wired at
// registration time, so the console and OS signals share the same graceful
// teardown path.
type consoleComponent struct {
	srv  *Server
	out  io.Writer
	stop func()
	con  *console
}

func (c *consoleComponent) Key() string         { return "livingworld.console" }
func (c *consoleComponent) DependsOn() []string { return []string{"livingworld.server"} }

// Init builds the console against the server capability surface and the
// harness stop trigger.
func (c *consoleComponent) Init(harness.Runtime) error {
	c.con = newConsole(c.srv, c.out, c.stop)
	return nil
}

// Start launches the stdin reader goroutine.
func (c *consoleComponent) Start(rt harness.Runtime) error {
	go c.con.run(os.Stdin)
	rt.Logger().Info("Console ready — type 'help' for commands")
	return nil
}

// Stop is a no-op: the console reads stdin until EOF, which there is no
// portable way to force-close. The goroutine exits naturally when the process
// shuts down; shutdown is not blocked on it.
func (c *consoleComponent) Stop(harness.Runtime) error { return nil }

// Compile-time assertions that the adapters satisfy the harness interfaces.
var (
	_ harness.Component     = serverComponent{}
	_ harness.Healthchecked = serverComponent{}
	_ harness.Component     = (*consoleComponent)(nil)
	_ harness.Dependent     = (*consoleComponent)(nil)
)
