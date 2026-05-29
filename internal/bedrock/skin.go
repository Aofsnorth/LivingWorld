package bedrock

import (
	"encoding/base64"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/login"
)

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
		capeData = []byte{}
	}
	if skinData == nil {
		data.SkinImageWidth, data.SkinImageHeight = 64, 64
		skinData = opaqueSkin(64, 64, 0xff, 0xff, 0xff)
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

func opaqueSkin(w, h int, r, g, b byte) []byte {
	data := make([]byte, w*h*4)
	for i := 0; i < len(data); i += 4 {
		data[i], data[i+1], data[i+2], data[i+3] = r, g, b, 0xff
	}
	return data
}

func defaultJavaSkin() protocol.Skin {
	data := make([]byte, 64*64*4)
	for i := 0; i < len(data); i += 4 {
		// Opaque Steve-like blue fallback. Keep it as a plain classic skin so
		// Bedrock can re-validate it when settings/skin UI opens.
		data[i], data[i+1], data[i+2], data[i+3] = 0x3f, 0x76, 0xe4, 0xff
	}

	// Bedrock validates the skin again when opening settings. A custom geometry
	// with an empty engine version can make the actor turn invisible. Use the
	// built-in humanoid geometry and a stable FullID instead of incomplete custom
	// geometry data.
	resourcePatch := []byte(`{"geometry":{"default":"geometry.humanoid"}}`)

	return protocol.Skin{
		SkinID:                    "livingworld_java_default",
		PlayFabID:                 "",
		SkinResourcePatch:         resourcePatch,
		SkinImageWidth:            64,
		SkinImageHeight:           64,
		SkinData:                  data,
		CapeImageWidth:            0,
		CapeImageHeight:           0,
		CapeData:                  []byte{},
		SkinGeometry:              []byte{},
		AnimationData:             []byte{},
		GeometryDataEngineVersion: []byte(protocol.CurrentVersion),
		PremiumSkin:               false,
		PersonaSkin:               false,
		PersonaCapeOnClassicSkin:  false,
		PrimaryUser:               false,
		CapeID:                    "",
		FullID:                    "livingworld:java_default:geometry.humanoid",
		SkinColour:                "#0",
		ArmSize:                   "wide",
		Trusted:                   true,
		OverrideAppearance:        false,
	}
}

func javaFallbackSkinForViewer(viewer *bedrockSession) protocol.Skin {
	// Do not borrow the viewer's skin/geometry. That made Java players look like
	// the Bedrock viewer or render as invalid cubes. Use a self-contained classic
	// fallback skin until Java texture translation is implemented.
	return defaultJavaSkin()
}

func classicSkinResourcePatch() []byte {
	return []byte(`{"geometry":{"default":"geometry.humanoid"}}`)
}
