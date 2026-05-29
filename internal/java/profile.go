package java

import (
	"livingworld/internal/player"

	"github.com/Tnze/go-mc/yggdrasil/user"
)

func javaProfileProperties(props []user.Property) []player.ProfileProperty {
	out := make([]player.ProfileProperty, 0, len(props))
	for _, prop := range props {
		out = append(out, player.ProfileProperty{Name: prop.Name, Value: prop.Value, Signature: prop.Signature})
	}
	return out
}
