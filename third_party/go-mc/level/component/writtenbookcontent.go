// writtenbookcontent.go contains helper types for the WrittenBookContent data component.
package component

import (
	"io"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/nbt/dynbt"
	pk "github.com/Tnze/go-mc/net/packet"
)

type WrittenPage struct {
	Content chat.Message // anonymousNbt
	// filteredContent: anonOptionalNbt — optional NBT, we store as dynbt.Value
	FilteredContent dynbt.Value
}

func (p *WrittenPage) ReadFrom(r io.Reader) (n int64, err error) {
	n, err = p.Content.ReadFrom(r)
	if err != nil {
		return
	}
	// anonOptionalNbt — read NBT that may be TagEnd
	n2, err := pk.NBTField{V: &p.FilteredContent, AllowUnknownFields: true}.ReadFrom(r)
	return n + n2, err
}

func (p WrittenPage) WriteTo(w io.Writer) (n int64, err error) {
	n, err = p.Content.WriteTo(w)
	if err != nil {
		return
	}
	n2, err := pk.NBTField{V: &p.FilteredContent, AllowUnknownFields: true}.WriteTo(w)
	return n + n2, err
}
