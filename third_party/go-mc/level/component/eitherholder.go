package component

import (
	"io"

	pk "github.com/Tnze/go-mc/net/packet"
)

// EitherHolder represents a value that is either a registry holder reference (VarInt)
// or a resource key (Identifier string). Wire: Boolean(true) + VarInt, or Boolean(false) + String.
//
// Java: EitherHolder<T> uses ByteBufCodecs.either(holderRegistry, ResourceKey.streamCodec).
// When isHolder=true, the VarInt is the registry protocol_id for Holder<T>.
// When isHolder=false, the String is the resource key (e.g. "minecraft:fall").
type EitherHolder struct {
	IsHolder pk.Boolean
	HolderID pk.VarInt     // only when IsHolder == true
	Key      pk.Identifier // only when IsHolder == false
}

func (e *EitherHolder) ReadFrom(r io.Reader) (n int64, err error) {
	n, err = e.IsHolder.ReadFrom(r)
	if err != nil {
		return
	}
	if e.IsHolder {
		n2, err := e.HolderID.ReadFrom(r)
		return n + n2, err
	}
	n2, err := e.Key.ReadFrom(r)
	return n + n2, err
}

func (e EitherHolder) WriteTo(w io.Writer) (n int64, err error) {
	n, err = e.IsHolder.WriteTo(w)
	if err != nil {
		return
	}
	if e.IsHolder {
		n2, err := e.HolderID.WriteTo(w)
		return n + n2, err
	}
	n2, err := e.Key.WriteTo(w)
	return n + n2, err
}
