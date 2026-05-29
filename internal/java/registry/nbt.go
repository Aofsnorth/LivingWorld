package registry

import (
	"bytes"

	"github.com/Tnze/go-mc/nbt"
)

func marshalNBT(v any) nbt.RawMessage {
	b, err := nbt.Marshal(v)
	if err != nil {
		panic(err)
	}
	return nbt.RawMessage{
		Type: nbt.TagCompound,
		Data: b[3:],
	}
}

func marshalNBTNetwork(v any) nbt.RawMessage {
	var buf bytes.Buffer
	enc := nbt.NewEncoder(&buf)
	enc.NetworkFormat(true)
	err := enc.Encode(v, "")
	if err != nil {
		panic(err)
	}
	return nbt.RawMessage{
		Type: nbt.TagCompound,
		Data: buf.Bytes(),
	}
}
