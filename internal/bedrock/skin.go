package bedrock

import (
	"encoding/base64"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/login"
)

// skinFromClientData converts the skin fields from Bedrock login.ClientData
// into the protocol.Skin required by PlayerList. Keeping this isolated makes
// future protocol-version skin changes easier to maintain.
func skinFromClientData(data login.ClientData) protocol.Skin {
	decode := func(s string) []byte {
		if s == "" {
			return nil
		}
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil
		}
		return b
	}

	skinData := decode(data.SkinData)
	capeData := decode(data.CapeData)
	geometry := decode(data.SkinGeometry)
	resourcePatch := decode(data.SkinResourcePatch)
	engineVersion := decode(data.SkinGeometryVersion)
	animationData := decode(data.SkinAnimationData)

	if data.SkinImageWidth == 0 || data.SkinImageHeight == 0 {
		data.SkinImageWidth, data.SkinImageHeight = 64, 64
	}
	if capeData == nil {
		// protocol.Skin validates width*height*4 == len(CapeData). For no cape,
		// leave dimensions at 0 and data empty.
		capeData = []byte{}
	}
	if skinData == nil {
		// Last-resort opaque white 64x64 skin to keep PlayerList encoding valid
		// even if a client sent broken skin data.
		data.SkinImageWidth, data.SkinImageHeight = 64, 64
		skinData = make([]byte, 64*64*4)
		for i := 0; i < len(skinData); i += 4 {
			skinData[i], skinData[i+1], skinData[i+2], skinData[i+3] = 0xff, 0xff, 0xff, 0xff
		}
	}

	armSize := data.ArmSize
	if armSize == "" {
		armSize = "wide"
	}
	colour := data.SkinColour
	if colour == "" {
		colour = "#0"
	}

	return protocol.Skin{
		SkinID:                    data.SkinID,
		PlayFabID:                 data.PlayFabID,
		SkinResourcePatch:         resourcePatch,
		SkinImageWidth:            uint32(data.SkinImageWidth),
		SkinImageHeight:           uint32(data.SkinImageHeight),
		SkinData:                  skinData,
		CapeImageWidth:            uint32(data.CapeImageWidth),
		CapeImageHeight:           uint32(data.CapeImageHeight),
		CapeData:                  capeData,
		SkinGeometry:              geometry,
		AnimationData:             animationData,
		GeometryDataEngineVersion: engineVersion,
		PremiumSkin:               data.PremiumSkin,
		PersonaSkin:               data.PersonaSkin,
		PersonaCapeOnClassicSkin:  data.CapeOnClassicSkin,
		PrimaryUser:               true,
		CapeID:                    data.CapeID,
		FullID:                    data.SkinID,
		SkinColour:                colour,
		ArmSize:                   armSize,
		Trusted:                   true,
		OverrideAppearance:        true,
	}
}
