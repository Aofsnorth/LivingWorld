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
		// Opaque Steve-like fallback. The important part is that the skin is a
		// valid 64x64 RGBA classic skin and comes with a classic resource patch,
		// otherwise Bedrock may render Java players as a tiny invalid cube.
		data[i], data[i+1], data[i+2], data[i+3] = 0xb9, 0x7a, 0x57, 0xff
	}
	// Shirt/pants color blocks to make the fallback visibly humanoid-ish.
	paintRect := func(x0, y0, x1, y1 int, r, g, b byte) {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				i := (y*64 + x) * 4
				data[i], data[i+1], data[i+2], data[i+3] = r, g, b, 0xff
			}
		}
	}
	paintRect(20, 20, 36, 32, 0x3f, 0x76, 0xe4) // torso front-ish
	paintRect(4, 20, 12, 32, 0x3f, 0x76, 0xe4)  // arm
	paintRect(44, 20, 52, 32, 0x3f, 0x76, 0xe4) // arm
	paintRect(4, 36, 12, 48, 0x35, 0x35, 0xa0)  // leg
	paintRect(20, 36, 28, 48, 0x35, 0x35, 0xa0) // leg

	return protocol.Skin{
		SkinID:             "livingworld_java_default",
		SkinResourcePatch:  classicSkinResourcePatch(),
		SkinImageWidth:     64,
		SkinImageHeight:    64,
		SkinData:           data,
		CapeData:           []byte{},
		SkinGeometry:       classicSkinGeometry(),
		SkinColour:         "#0",
		ArmSize:            "wide",
		Trusted:            true,
		PrimaryUser:        true,
		OverrideAppearance: true,
	}
}

func javaFallbackSkinForViewer(viewer *bedrockSession) protocol.Skin {
	// Do not borrow the viewer's skin/geometry. That made Java players look like
	// the Bedrock viewer or render as invalid cubes. Use a self-contained classic
	// fallback skin until Java texture translation is implemented.
	return defaultJavaSkin()
}

func classicSkinResourcePatch() []byte {
	return []byte(`{"geometry":{"default":"geometry.humanoid.custom"}}`)
}

func classicSkinGeometry() []byte {
	return []byte(`{"format_version":"1.12.0","minecraft:geometry":[{"description":{"identifier":"geometry.humanoid.custom","texture_width":64,"texture_height":64,"visible_bounds_width":2,"visible_bounds_height":3,"visible_bounds_offset":[0,1.5,0]},"bones":[{"name":"root","pivot":[0,0,0]},{"name":"waist","parent":"root","pivot":[0,12,0]},{"name":"body","parent":"waist","pivot":[0,24,0],"cubes":[{"origin":[-4,12,-2],"size":[8,12,4],"uv":[16,16]}]},{"name":"head","parent":"body","pivot":[0,24,0],"cubes":[{"origin":[-4,24,-4],"size":[8,8,8],"uv":[0,0]},{"origin":[-4,24,-4],"size":[8,8,8],"inflate":0.5,"uv":[32,0]}]},{"name":"hat","parent":"head","pivot":[0,24,0]},{"name":"rightArm","parent":"body","pivot":[-5,22,0],"cubes":[{"origin":[-8,12,-2],"size":[4,12,4],"uv":[40,16]}]},{"name":"leftArm","parent":"body","pivot":[5,22,0],"cubes":[{"origin":[4,12,-2],"size":[4,12,4],"uv":[32,48]}]},{"name":"rightLeg","parent":"root","pivot":[-1.9,12,0],"cubes":[{"origin":[-4,0,-2],"size":[4,12,4],"uv":[0,16]}]},{"name":"leftLeg","parent":"root","pivot":[1.9,12,0],"cubes":[{"origin":[0,0,-2],"size":[4,12,4],"uv":[16,48]}]}]}]}`)
}

func fillRectRGBA(img []byte, width, x0, y0, w, h int, r, g, b, a byte) {
	for y := y0; y < y0+h; y++ {
		for x := x0; x < x0+w; x++ {
			i := (y*width + x) * 4
			if i+3 >= 0 && i+3 < len(img) {
				img[i], img[i+1], img[i+2], img[i+3] = r, g, b, a
			}
		}
	}
}
