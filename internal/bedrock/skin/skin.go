package skin

import (
	_ "embed"
	"encoding/base64"

	"livingworld/internal/player"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/login"
)

//go:embed data/skin_resource_patch.json
var defaultSkinResourcePatch []byte

//go:embed data/skin_geometry.json
var defaultSkinGeometry []byte

func SkinFromClientData(data login.ClientData) protocol.Skin {
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
	if resourcePatch == nil {
		resourcePatch = defaultSkinResourcePatch
	}
	if geometry == nil {
		geometry = defaultSkinGeometry
	}
	if engineVersion == nil {
		engineVersion = []byte("0.0.0")
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

func defaultJavaSkin(suffix string) protocol.Skin {
	data := make([]byte, 64*64*4)
	for i := 0; i < len(data); i += 4 {
		// Opaque Steve-like blue fallback. Keep it as a plain classic skin so
		// Bedrock can re-validate it when settings/skin UI opens.
		data[i], data[i+1], data[i+2], data[i+3] = 0x3f, 0x76, 0xe4, 0xff
	}

	// Use the same custom humanoid geometry/resource patch that gophertunnel
	// uses for classic skins. The previous fallback used an escaped raw JSON
	// patch and no geometry data, so modern Bedrock rejected the skin and the
	// Java actor rendered invisible for newly joined Bedrock clients.
	skinID := "livingworld_java_fallback_v2"
	if suffix != "" {
		skinID += "_" + suffix
	}

	return protocol.Skin{
		SkinID:                    skinID,
		PlayFabID:                 "",
		SkinResourcePatch:         defaultSkinResourcePatch,
		SkinImageWidth:            64,
		SkinImageHeight:           64,
		SkinData:                  data,
		CapeImageWidth:            0,
		CapeImageHeight:           0,
		CapeData:                  []byte{},
		SkinGeometry:              defaultSkinGeometry,
		AnimationData:             []byte{},
		GeometryDataEngineVersion: []byte("0.0.0"),
		PremiumSkin:               false,
		PersonaSkin:               false,
		PersonaCapeOnClassicSkin:  false,
		PrimaryUser:               false,
		CapeID:                    "",
		FullID:                    skinID,
		SkinColour:                "#0",
		ArmSize:                   "wide",
		Trusted:                   true,
		OverrideAppearance:        true,
	}
}

func JavaFallbackSkinForViewer(p player.PlayerSnapshot) protocol.Skin {
	if p.Skin != nil && len(p.Skin.Data) > 0 {
		w := 64
		h := 64
		// Standard Java skins are 64x64 or 64x32.
		if len(p.Skin.Data) == 64*32*4 {
			h = 32
		} else if len(p.Skin.Data) == 128*128*4 {
			w = 128
			h = 128
		} else if len(p.Skin.Data) == 128*64*4 {
			w = 128
			h = 64
		}

		armSize := "wide"
		if p.Skin.IsSlim() {
			armSize = "slim"
		}

		skinID := "java_" + p.UUID.String()
		return protocol.Skin{
			SkinID:                    skinID,
			PlayFabID:                 "",
			SkinResourcePatch:         defaultSkinResourcePatch,
			SkinImageWidth:            uint32(w),
			SkinImageHeight:           uint32(h),
			SkinData:                  p.Skin.Data,
			CapeImageWidth:            0,
			CapeImageHeight:           0,
			CapeData:                  []byte{},
			SkinGeometry:              defaultSkinGeometry,
			AnimationData:             []byte{},
			GeometryDataEngineVersion: []byte("0.0.0"),
			PremiumSkin:               false,
			PersonaSkin:               false,
			PersonaCapeOnClassicSkin:  false,
			PrimaryUser:               false,
			CapeID:                    "",
			FullID:                    skinID,
			SkinColour:                "#0",
			ArmSize:                   armSize,
			Trusted:                   true,
			OverrideAppearance:        true,
		}
	}
	// Do not borrow the viewer's skin/geometry. That made Java players look like
	// the Bedrock viewer or render as invalid cubes. Use a self-contained classic
	// fallback skin until Java texture translation is implemented.
	return defaultJavaSkin("")
}

func classicSkinResourcePatch() []byte {
	return defaultSkinResourcePatch
}
