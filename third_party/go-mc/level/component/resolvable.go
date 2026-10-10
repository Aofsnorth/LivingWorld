package component

import (
	"io"

	pk "github.com/Tnze/go-mc/net/packet"
)

// ResolvableInt and ResolvableFloat mirror Minecraft's loot "resolvable"
// number providers, which 26.x started using as data component types
// (minecraft:brewing_fuel, minecraft:cooking_fuel, minecraft:compostable).
// The vanilla schema extractor reports them as opaque named types rather
// than expanding their fields, so the component generator emits a
// reference to these declarations.
//
// Wire format (verified against the 26.3 server jar, not inferred):
//
//	ResolvableInt.STREAM_CODEC = StreamCodec.map(
//	    ByteBufCodecs.either(Constant.STREAM_CODEC, Reference.STREAM_CODEC), ...)
//	  where Constant.STREAM_CODEC wraps ByteBufCodecs.INT
//	    and   Reference.STREAM_CODEC wraps a CONTEXT_INT_PROVIDER ResourceKey
//
//	ResolvableFloat is the same shape with ByteBufCodecs.FLOAT for Constant.
//
// ByteBufCodecs.either encodes its discriminator with ByteBuf.writeBoolean,
// where true selects the left codec (Constant) and false the right one
// (Reference). Only the selected branch's payload follows the tag, so these
// types encode/decode conditionally rather than as a flat tuple.
const (
	// ResolvableConstantTag selects the inline-value branch.
	ResolvableConstantTag = pk.Boolean(true)
	// ResolvableReferenceTag selects the registry-reference branch.
	ResolvableReferenceTag = pk.Boolean(false)
)

// ResolvableInt is a tagged union of an inline int (Constant) or a
// reference to an entry in the minecraft:context_int_provider registry.
//
// IsConstant selects the branch. Value is only meaningful when IsConstant
// is ResolvableConstantTag; Reference only when it is
// ResolvableReferenceTag. The inactive field stays at its zero value.
type ResolvableInt struct {
	IsConstant pk.Boolean
	Value      pk.VarInt
	Reference  pk.Identifier
}

// ReadFrom implements FieldDecoder.
func (r *ResolvableInt) ReadFrom(rd io.Reader) (n int64, err error) {
	if n, err = r.IsConstant.ReadFrom(rd); err != nil {
		return
	}
	if r.IsConstant == ResolvableConstantTag {
		n, err = r.Value.ReadFrom(rd)
		return
	}
	n, err = r.Reference.ReadFrom(rd)
	return
}

// WriteTo implements FieldEncoder.
func (r ResolvableInt) WriteTo(w io.Writer) (n int64, err error) {
	if n, err = r.IsConstant.WriteTo(w); err != nil {
		return
	}
	if r.IsConstant == ResolvableConstantTag {
		n, err = r.Value.WriteTo(w)
		return
	}
	n, err = r.Reference.WriteTo(w)
	return
}

// ConstantInt builds a ResolvableInt holding a literal value.
func ConstantInt(v int32) ResolvableInt {
	return ResolvableInt{IsConstant: ResolvableConstantTag, Value: pk.VarInt(v)}
}

// ConstantFloat builds a ResolvableFloat holding a literal value.
func ConstantFloat(v float32) ResolvableFloat {
	return ResolvableFloat{IsConstant: ResolvableConstantTag, Value: pk.Float(v)}
}

// ResolvableFloat is a tagged union of an inline float (Constant) or a
// reference to an entry in the minecraft:context_float_provider registry.
// See ResolvableInt for the wire format.
type ResolvableFloat struct {
	IsConstant pk.Boolean
	Value      pk.Float
	Reference  pk.Identifier
}

// ReadFrom implements FieldDecoder.
func (r *ResolvableFloat) ReadFrom(rd io.Reader) (n int64, err error) {
	if n, err = r.IsConstant.ReadFrom(rd); err != nil {
		return
	}
	if r.IsConstant == ResolvableConstantTag {
		n, err = r.Value.ReadFrom(rd)
		return
	}
	n, err = r.Reference.ReadFrom(rd)
	return
}

// WriteTo implements FieldEncoder.
func (r ResolvableFloat) WriteTo(w io.Writer) (n int64, err error) {
	if n, err = r.IsConstant.WriteTo(w); err != nil {
		return
	}
	if r.IsConstant == ResolvableConstantTag {
		n, err = r.Value.WriteTo(w)
		return
	}
	n, err = r.Reference.WriteTo(w)
	return
}
