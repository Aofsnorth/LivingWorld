package protocol

import (
	"bytes"
	"encoding/binary"

	"livingworld/internal/item"

	"github.com/Tnze/go-mc/data/entity"
	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

// itemEntityUUID derives a stable UUID for an item entity from its id. The value
// only needs to be unique among spawned entities, not globally meaningful.
func itemEntityUUID(entityID int32) uuid.UUID {
	var u uuid.UUID
	binary.BigEndian.PutUint32(u[12:], uint32(entityID))
	u[0] = 0xDD // tag the high byte so it never collides with a player UUID
	return u
}

// Item-entity metadata layout for MC 26.1 (protocol 775), verified by
// decompiling the 26.1 jar (ItemEntity.class / Entity.class /
// EntityDataSerializers.class):
//   - base Entity defines 8 synched fields (indices 0-7), so ItemEntity's own
//     "Item" field is index 8.
//   - the ItemStack serializer's numeric id is 7 (BYTE0 INT1 LONG2 FLOAT3
//     STRING4 COMPONENT5 OPTIONAL_COMPONENT6 ITEM_STACK7 ...).
const (
	MetaIndexItemEntityItem = 8
	MetaTypeItemStack       = 7
)

// writeItemStack writes a network ItemStack (MC 26.1) for the item with the
// given Java protocol id and count. Verified format: VarInt count; if count==0
// the stack is empty and nothing else is written; else VarInt itemID followed by
// the DataComponentPatch (VarInt nAdd, VarInt nRemove). Block drops carry no
// components, so nAdd=nRemove=0.
func writeItemStack(buf *bytes.Buffer, itemID int32, count int) {
	if count <= 0 {
		_, _ = pk.VarInt(0).WriteTo(buf)
		return
	}
	_, _ = pk.VarInt(count).WriteTo(buf)
	_, _ = pk.VarInt(itemID).WriteTo(buf)
	_, _ = pk.VarInt(0).WriteTo(buf) // components added
	_, _ = pk.VarInt(0).WriteTo(buf) // components removed
}

// ItemNetworkID resolves an item name ("minecraft:cobblestone") to its Java
// protocol item id. Returns 0, false for unknown items.
func ItemNetworkID(name string) (int32, bool) {
	it, ok := item.ByName(name)
	if !ok {
		return 0, false
	}
	return it.ID, true
}

// SpawnItemEntity sends ClientboundGameAddEntity for a dropped item entity, then
// ClientboundGameSetEntityData carrying the item stack so the client renders the
// correct item. entityID is the drop's id (mapped to a Java entity id by the
// caller); itemID is the Java item id; count is the stack size.
func (h *Handler775) SpawnItemEntity(s Session, entityID int32, itemName string, count int, x, y, z float64) error {
	itemID, ok := ItemNetworkID(itemName)
	if !ok {
		return nil // unknown item: skip rather than send a bad stack
	}

	// AddEntity layout mirrors the working player-avatar spawn (SpawnForeignAvatar):
	// id, uuid, type, x/y/z, pitch(Angle), yaw(Angle), headYaw(Angle), data(VarInt),
	// then velocity vx/vy/vz (Short). Items spawn at rest.
	if err := s.SendPacket(pk.Marshal(
		packetid.ClientboundGameAddEntity,
		pk.VarInt(entityID),
		pk.UUID(itemEntityUUID(entityID)),
		pk.VarInt(entity.Item.ID), // 71 = minecraft:item
		pk.Double(x), pk.Double(y), pk.Double(z),
		pk.Angle(0), // pitch
		pk.Angle(0), // yaw
		pk.Angle(0), // head yaw (unused for items)
		pk.VarInt(0), // object data
		pk.Short(0), pk.Short(0), pk.Short(0), // velocity x/y/z
	)); err != nil {
		return err
	}

	// Metadata: index 8 (Item), serializer 7 (ItemStack), value = the stack.
	var buf bytes.Buffer
	_, _ = pk.VarInt(entityID).WriteTo(&buf)
	_, _ = pk.Byte(MetaIndexItemEntityItem).WriteTo(&buf)
	_, _ = pk.VarInt(MetaTypeItemStack).WriteTo(&buf)
	writeItemStack(&buf, itemID, count)
	_, _ = pk.Byte(-1).WriteTo(&buf) // metadata terminator
	return s.SendPacket(pk.Packet{ID: int32(packetid.ClientboundGameSetEntityData), Data: buf.Bytes()})
}

// RemoveItemEntity despawns a dropped item entity on the client.
func (h *Handler775) RemoveItemEntity(s Session, entityID int32) error {
	return s.SendPacket(pk.Marshal(
		packetid.ClientboundGameRemoveEntities,
		pk.Ary[pk.VarInt]{Ary: []pk.VarInt{pk.VarInt(entityID)}},
	))
}

// TakeItemEntity plays the pickup animation (item flies into the collector).
// This is animation-only; the caller must also despawn the entity and update the
// collector's inventory.
func (h *Handler775) TakeItemEntity(s Session, itemEntityID, collectorEntityID int32, count int) error {
	return s.SendPacket(pk.Marshal(
		packetid.ClientboundGameTakeItemEntity,
		pk.VarInt(itemEntityID),
		pk.VarInt(collectorEntityID),
		pk.VarInt(count),
	))
}
