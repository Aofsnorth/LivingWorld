// tool.go contains helper types for the Tool data component.
package component

import (
	"io"

	pk "github.com/Tnze/go-mc/net/packet"
)

type ToolRule struct {
	Blocks               IDSet
	Speed                pk.Option[pk.Float, *pk.Float]
	CorrectDropForBlocks pk.Option[pk.Boolean, *pk.Boolean]
}

func (r *ToolRule) ReadFrom(rd io.Reader) (n int64, err error) {
	return pk.Tuple{&r.Blocks, &r.Speed, &r.CorrectDropForBlocks}.ReadFrom(rd)
}

func (r ToolRule) WriteTo(w io.Writer) (n int64, err error) {
	return pk.Tuple{&r.Blocks, &r.Speed, &r.CorrectDropForBlocks}.WriteTo(w)
}
