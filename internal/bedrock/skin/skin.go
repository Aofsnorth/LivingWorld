package skin

import (
	_ "embed"
	"encoding/base64"
	"image/color"
	"strconv"
	"strings"

	"livingworld/internal/java/skin"
	"livingworld/internal/player"

	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/login"
)

//go:embed data/skin_resource_patch.json
var defaultSkinResourcePatch []byte

var slimSkinResourcePatch = []byte("{\n  \"geometry\" : {\n    \"default\" : \"geometry.humanoid.customSlim\"\n  }\n}\n")

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
		engineVersion = []byte(protocol.CurrentVersion)
	}
	if len(skinData) == 64*32*4 {
		skinData = skin.UpgradeLegacySkin(skinData, 64)
		data.SkinImageWidth, data.SkinImageHeight = 64, 64
	} else if len(skinData) == 128*64*4 {
		skinData = skin.UpgradeLegacySkin(skinData, 128)
		data.SkinImageWidth, data.SkinImageHeight = 128, 128
	}
	capeID := data.CapeID
	if capeID == "" {
		capeID = uuid.New().String()
	}
	skinID := data.SkinID
	if skinID == "" {
		skinID = uuid.New().String()
	}

	return protocol.Skin{
		SkinID:                    skinID,
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
		CapeID:                    capeID,
		FullID:                    skinID,
		SkinColour:                parseSkinColour(data.SkinColour),
		ArmSize:                   armSize(data.ArmSize),
		Trusted:                   true,
		OverrideAppearance:        true,
	}
}

// parseSkinColour converts a Bedrock hex skin colour into the color.RGBA the
// outgoing protocol.Skin expects. The client still reports it as a "#RRGGBB"
// (or "#AARRGGBB") string, but the wire field is big-endian ARGB. Values we
// cannot parse fall back to the fully transparent colour, which is what the
// "#0" placeholder older gophertunnel releases accepted.
func parseSkinColour(s string) color.RGBA {
	h := strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(h) == 6 {
		h = "FF" + h // opaque RGB
	}
	if len(h) != 8 {
		return color.RGBA{}
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return color.RGBA{}
	}
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: uint8(v >> 24)}
}

// armSize maps the client's "slim"/"wide" arm description onto the uint8
// field the outgoing protocol.Skin now carries. Anything unrecognised is
// treated as "wide", matching the classic Steve/Alex default.
func armSize(s string) uint8 {
	if strings.EqualFold(strings.TrimSpace(s), "slim") {
		return protocol.ArmSizeSlim
	}
	return protocol.ArmSizeWide
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
		GeometryDataEngineVersion: []byte(protocol.CurrentVersion),
		PremiumSkin:               false,
		PersonaSkin:               false,
		PersonaCapeOnClassicSkin:  false,
		PrimaryUser:               true,
		CapeID:                    uuid.New().String(),
		FullID:                    skinID,
		ArmSize:                   protocol.ArmSizeWide,
		Trusted:                   true,
		OverrideAppearance:        true,
	}
}

func JavaFallbackSkinForViewer(p player.PlayerSnapshot) protocol.Skin {
	if p.Skin != nil && len(p.Skin.Data) > 0 {
		skinData := p.Skin.Data
		w := 64
		h := 64
		switch len(skinData) {
		case 64 * 64 * 4:
			w, h = 64, 64
		case 64 * 32 * 4:
			// Upgrade legacy 64x32 skins to 64x64 so they match defaultSkinGeometry's
			// 64x64 UV layout; otherwise Bedrock 26.x disconnects with Error "Block".
			skinData = skin.UpgradeLegacySkin(skinData, 64)
			w, h = 64, 64
		case 128 * 128 * 4:
			w, h = 128, 128
		case 128 * 64 * 4:
			skinData = skin.UpgradeLegacySkin(skinData, 128)
			w, h = 128, 128
		default:
			return defaultJavaSkin(p.UUID.String())
		}

		arms := uint8(protocol.ArmSizeWide)
		resourcePatch := defaultSkinResourcePatch
		if p.Skin.IsSlim() {
			arms = protocol.ArmSizeSlim
			resourcePatch = slimSkinResourcePatch
		}

		skinID := "java_" + p.UUID.String()
		return protocol.Skin{
			SkinID:                    skinID,
			PlayFabID:                 "",
			SkinResourcePatch:         resourcePatch,
			SkinImageWidth:            uint32(w),
			SkinImageHeight:           uint32(h),
			SkinData:                  skinData,
			CapeImageWidth:            0,
			CapeImageHeight:           0,
			CapeData:                  []byte{},
			SkinGeometry:              defaultSkinGeometry,
			AnimationData:             []byte{},
			GeometryDataEngineVersion: []byte(protocol.CurrentVersion),
			PremiumSkin:               false,
			PersonaSkin:               false,
			PersonaCapeOnClassicSkin:  false,
			PrimaryUser:               true,
			CapeID:                    uuid.New().String(),
			FullID:                    skinID,
			ArmSize:                   arms,
			Trusted:                   true,
			OverrideAppearance:        true,
		}
	}
	// Do not borrow the viewer's skin/geometry. That made Java players look like
	// the Bedrock viewer or render as invalid cubes. Use a self-contained classic
	// fallback skin until Java texture translation is implemented.
	return defaultJavaSkin(p.UUID.String())
}

func classicSkinResourcePatch() []byte {
	return defaultSkinResourcePatch
}
