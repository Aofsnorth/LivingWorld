package server

import (
	"context"
	"log/slog"
	"net"
	"strings"
	"sync"

	"github.com/sandertv/go-raknet"
	"github.com/sandertv/gophertunnel/minecraft"
)

// PROBLEM #8 — Bedrock LAN/friends list shows "Creative" even though the in-game
// GameData is Survival.
//
// The gamemode advertised in the RakNet unconnected-pong is the 9th
// semicolon-separated field of the MCPE pong string, and gophertunnel hardcodes
// it to the literal "Creative" inside (*Listener).updatePongData:
//
//	MCPE;name;proto;version;players;max;guid;subName;Creative;1;portV4;portV6;0;
//	 0    1    2      3        4     5    6     7        8     9  10     11    12
//
// The public StatusProvider/ServerStatus API exposes no gamemode field, so
// minecraft.NewStatusProvider can never correct it, and a 4-second ticker
// re-runs updatePongData() (overwriting any one-shot external write). The only
// robust fix is to intercept the raw PongData([]byte) sink.
//
// We register a replacement "raknet" network whose listener wraps go-raknet's and
// rewrites the gamemode field on every PongData call (including the ticker's
// re-writes). DialContext/PingContext delegate unchanged so outbound RakNet still
// works. This mirrors gophertunnel's own minecraft.RakNet implementation — we
// cannot embed minecraft.RakNet directly because its logger field is unexported.

var registerSurvivalNetworkOnce sync.Once

// registerSurvivalNetwork replaces the process-global "raknet" network with one
// that forces the pong gamemode to Survival. Safe to call repeatedly; only the
// first call takes effect (RegisterNetwork mutates a global map).
func registerSurvivalNetwork() {
	registerSurvivalNetworkOnce.Do(func() {
		minecraft.RegisterNetwork("raknet", func(l *slog.Logger) minecraft.Network {
			return survivalNetwork{l: l}
		})
	})
}

// survivalNetwork is a drop-in for gophertunnel's minecraft.RakNet that only
// differs in the listener it returns (see survivalPongListener).
type survivalNetwork struct {
	l *slog.Logger
}

func (n survivalNetwork) logger() *slog.Logger {
	if n.l == nil {
		return slog.Default()
	}
	return n.l.With("net origin", "raknet")
}

func (n survivalNetwork) DialContext(ctx context.Context, address string) (net.Conn, error) {
	return raknet.Dialer{ErrorLog: n.logger()}.DialContext(ctx, address)
}

func (n survivalNetwork) PingContext(ctx context.Context, address string) (response []byte, err error) {
	return raknet.Dialer{ErrorLog: n.logger()}.PingContext(ctx, address)
}

func (n survivalNetwork) Listen(address string) (minecraft.NetworkListener, error) {
	l, err := raknet.ListenConfig{ErrorLog: n.logger()}.Listen(address)
	if err != nil {
		return nil, err
	}
	return survivalPongListener{Listener: l}, nil
}

// survivalPongListener embeds *raknet.Listener (which already satisfies
// net.Listener + ID() int64 + PongData([]byte)) and rewrites the gamemode field
// before storing the pong bytes.
type survivalPongListener struct {
	*raknet.Listener
}

func (l survivalPongListener) PongData(data []byte) {
	l.Listener.PongData(rewriteGamemode(data))
}

// rewriteGamemode sets the pong's gamemode (field 8) to "Survival" and the
// numeric gamemode (field 9) to "0", preserving every other field including the
// trailing empty fragment. Malformed/short pongs are passed through untouched.
func rewriteGamemode(data []byte) []byte {
	parts := strings.Split(string(data), ";")
	if len(parts) <= 8 {
		return data // not a well-formed MCPE pong; leave it alone
	}
	parts[8] = "Survival"
	if len(parts) > 9 {
		parts[9] = "0" // 0 = Survival, 1 = Creative
	}
	return []byte(strings.Join(parts, ";"))
}
