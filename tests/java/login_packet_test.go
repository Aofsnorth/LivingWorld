package javatests

import (
	"bytes"
	"net"
	"testing"

	"livingworld/config"
	"livingworld/internal/java/protocol"
	javaworld "livingworld/internal/java/world"
	"livingworld/internal/player"
	"livingworld/internal/world"

	"github.com/Tnze/go-mc/data/packetid"
	gmnet "github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

type mockSession struct {
	conn *gmnet.Conn
	cfg  *config.Config
	id   uuid.UUID
}

func (m *mockSession) Conn() *gmnet.Conn                { return m.conn }
func (m *mockSession) UUID() uuid.UUID                  { return m.id }
func (m *mockSession) Username() string                 { return "TestPlayer" }
func (m *mockSession) EntityID() int32                  { return 42 }
func (m *mockSession) Config() *config.Config           { return m.cfg }
func (m *mockSession) PlayerManager() *player.Manager   { return nil }
func (m *mockSession) WorldManager() *world.Manager     { return nil }
func (m *mockSession) GameMode() int32                  { return 1 } // Creative
func (m *mockSession) SendSpawnPosition() error         { return nil }
func (m *mockSession) SendHealth() error                { return nil }
func (m *mockSession) UpdateChunks()                    {}
func (m *mockSession) SendWorldState()                  {}
func (m *mockSession) HandleChatCommand(p pk.Packet)    {}
func (m *mockSession) HandleMovePos(p pk.Packet)        {}
func (m *mockSession) HandleMovePosRot(p pk.Packet)     {}
func (m *mockSession) HandleMoveRot(p pk.Packet)        {}
func (m *mockSession) HandleMoveStatusOnly(p pk.Packet) {}
func (m *mockSession) HandlePlayerAction(p pk.Packet)   {}
func (m *mockSession) HandleUseItemOn(p pk.Packet)      {}
func (m *mockSession) HandleChat(p pk.Packet)           {}
func (m *mockSession) HandleCreativeSlot(p pk.Packet)   {}
func (m *mockSession) HandleSetCarriedItem(p pk.Packet) {}
func (m *mockSession) HandleContainerClick(p pk.Packet) {}
func (m *mockSession) HandleContainerClose(p pk.Packet) {}
func (m *mockSession) HandleSwing(p pk.Packet)          {}
func (m *mockSession) HandlePlayerCommand(p pk.Packet)  {}
func (m *mockSession) HandleClientCommand(p pk.Packet)  {}
func (m *mockSession) HandleInteract(p pk.Packet)       {}
func (m *mockSession) HandleAttack(p pk.Packet)         {}
func (m *mockSession) SendPacket(p pk.Packet) error     { return m.conn.WritePacket(p) }

// TestClientboundGameLogin777WireFormat verifies that Handler777 encodes
// clientbound/minecraft:login (Play 0x32) matching Minecraft 26.3's exact
// ClientboundLoginPacket.STREAM_CODEC and CommonPlayerSpawnInfo.STREAM_CODEC,
// leaving zero unconsumed trailing bytes.
func TestClientboundGameLogin777WireFormat(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	cfg := config.Default()
	cfg.World.Seed = 123456789
	cfg.Java.ViewDistance = 10
	cfg.Java.SimulationDistance = 8
	cfg.Java.OnlineMode = true

	sess := &mockSession{
		conn: gmnet.WrapConn(serverSide),
		cfg:  cfg,
		id:   uuid.New(),
	}

	h, ok := protocol.GetVersionHandler(777)
	if !ok {
		t.Fatal("protocol 777 handler not registered")
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- h.SendInitialPlayPackets(sess)
	}()

	clientConn := gmnet.WrapConn(clientSide)
	var loginPkt pk.Packet
	if err := clientConn.ReadPacket(&loginPkt); err != nil {
		t.Fatalf("read login packet: %v", err)
	}
	// Drain remaining packets sent by SendInitialPlayPackets.
	go func() {
		var p pk.Packet
		for clientConn.ReadPacket(&p) == nil {
		}
	}()

	if err := <-errCh; err != nil {
		t.Fatalf("SendInitialPlayPackets failed: %v", err)
	}

	if loginPkt.ID != int32(packetid.ClientboundGameLogin) {
		t.Fatalf("first packet ID = 0x%02x, want 0x%02x (ClientboundGameLogin)", loginPkt.ID, packetid.ClientboundGameLogin)
	}

	r := bytes.NewReader(loginPkt.Data)
	var (
		playerID           pk.Int
		hardcore           pk.Boolean
		levelCount         pk.VarInt
		level0             pk.Identifier
		maxPlayers         pk.VarInt
		chunkRadius        pk.VarInt
		simulationDistance pk.VarInt
		reducedDebugInfo   pk.Boolean
		showDeathScreen    pk.Boolean
		doLimitedCrafting  pk.Boolean
		// CommonPlayerSpawnInfo
		dimensionType    pk.VarInt
		dimension        pk.Identifier
		seed             pk.Long
		gameType         pk.VarInt
		previousGameType pk.VarInt // ByteBufCodecs.OPTIONAL_VAR_INT (0 = empty)
		isDebug          pk.Boolean
		isFlat           pk.Boolean
		hasDeathLocation pk.Boolean
		portalCooldown   pk.VarInt
		seaLevel         pk.VarInt
		// Trailing 26.3 ClientboundLoginPacket fields
		onlineMode         pk.Boolean
		enforcesSecureChat pk.Boolean
	)

	fields := []pk.FieldDecoder{
		&playerID,
		&hardcore,
		&levelCount,
		&level0,
		&maxPlayers,
		&chunkRadius,
		&simulationDistance,
		&reducedDebugInfo,
		&showDeathScreen,
		&doLimitedCrafting,
		&dimensionType,
		&dimension,
		&seed,
		&gameType,
		&previousGameType,
		&isDebug,
		&isFlat,
		&hasDeathLocation,
		&portalCooldown,
		&seaLevel,
		&onlineMode,
		&enforcesSecureChat,
	}
	for i, f := range fields {
		if _, err := f.ReadFrom(r); err != nil {
			t.Fatalf("failed decoding field #%d: %v", i, err)
		}
	}

	if r.Len() != 0 {
		t.Fatalf("packet has %d trailing unread bytes (Netty DecoderException)", r.Len())
	}

	if int32(playerID) != 42 {
		t.Errorf("playerID = %d, want 42", playerID)
	}
	if int32(chunkRadius) != 10 {
		t.Errorf("chunkRadius = %d, want 10", chunkRadius)
	}
	if int32(simulationDistance) != 8 {
		t.Errorf("simulationDistance = %d, want 8", simulationDistance)
	}
	if int64(seed) != 123456789 {
		t.Errorf("seed = %d, want 123456789", seed)
	}
	if int32(gameType) != 1 {
		t.Errorf("gameType = %d, want 1 (Creative)", gameType)
	}
	if int32(previousGameType) != 0 {
		t.Errorf("previousGameType (OPTIONAL_VAR_INT) = %d, want 0 (Optional.empty)", previousGameType)
	}
	if int32(seaLevel) != 63 {
		t.Errorf("seaLevel = %d, want 63", seaLevel)
	}
	if !bool(onlineMode) {
		t.Errorf("onlineMode = false, want true")
	}
	if bool(enforcesSecureChat) {
		t.Errorf("enforcesSecureChat = true, want false")
	}
}

func TestClientboundGameSwingAnimation777WireFormat(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	sess := &mockSession{
		conn: gmnet.WrapConn(serverSide),
		cfg:  config.Default(),
		id:   uuid.New(),
	}

	h, ok := protocol.GetVersionHandler(777)
	if !ok {
		t.Fatal("protocol 777 handler not registered")
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- h.SwingForeignAvatar(sess, player.PlayerSnapshot{
			UUID:            uuid.New(),
			EntityRuntimeID: 99,
		})
	}()

	clientConn := gmnet.WrapConn(clientSide)
	var swingPkt pk.Packet
	if err := clientConn.ReadPacket(&swingPkt); err != nil {
		t.Fatalf("read swing packet: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("SwingForeignAvatar failed: %v", err)
	}

	if swingPkt.ID != int32(packetid.ClientboundGameSwingAnimation) {
		t.Fatalf("swing packet ID = %d (0x%02x), want %d (ClientboundGameSwingAnimation)", swingPkt.ID, swingPkt.ID, packetid.ClientboundGameSwingAnimation)
	}

	r := bytes.NewReader(swingPkt.Data)
	var entityID, hand, animType, duration pk.VarInt
	for i, f := range []*pk.VarInt{&entityID, &hand, &animType, &duration} {
		if _, err := f.ReadFrom(r); err != nil {
			t.Fatalf("decode swing field #%d: %v", i, err)
		}
	}
	if r.Len() != 0 {
		t.Fatalf("swing packet has %d trailing bytes", r.Len())
	}
	if int32(entityID) != 99 || int32(hand) != 0 || int32(animType) != 1 || int32(duration) != 6 {
		t.Fatalf("swing fields = (entityID=%d, hand=%d, type=%d, duration=%d), want (99, 0, 1, 6)", entityID, hand, animType, duration)
	}
}

func TestConvertToLevelChunkFluidCount(t *testing.T) {
	c := world.NewChunk()

	// Section index 8 corresponds to Y = 64..79 (-64 + 8*16 = 64)
	c.SetBlock(0, 64, 0, world.BlockByName("minecraft:water"))
	c.SetBlock(1, 64, 0, world.BlockByName("minecraft:kelp_plant"))
	c.SetBlock(2, 64, 0, world.BlockByName("minecraft:lava"))
	c.SetBlock(3, 64, 0, world.BlockByName("minecraft:stone"))

	lc := javaworld.ConvertToLevelChunk(c)
	sec := lc.Sections[8]
	if sec.BlockCount != 4 {
		t.Fatalf("Section[8].BlockCount = %d, want 4", sec.BlockCount)
	}
	if sec.FluidCount != 3 {
		t.Fatalf("Section[8].FluidCount = %d, want 3 (water + kelp_plant + lava); client EntityFluidInteraction.hasFluid() requires FluidCount > 0 to allow swimming", sec.FluidCount)
	}
}

