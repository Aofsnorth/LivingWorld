package bedrocktests

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"livingworld/config"
	"livingworld/internal/skinbridge"
)

func setPixel(buf []byte, stride, x, y int, r, g, b, a uint8) {
	idx := y*stride + x*4
	buf[idx] = r
	buf[idx+1] = g
	buf[idx+2] = b
	buf[idx+3] = a
}

func getPixel(buf []byte, stride, x, y int) (r, g, b, a uint8) {
	idx := y*stride + x*4
	return buf[idx], buf[idx+1], buf[idx+2], buf[idx+3]
}

func fillRect(buf []byte, stride, x0, y0, w, h int, r, g, b, a uint8) {
	for y := y0; y < y0+h; y++ {
		for x := x0; x < x0+w; x++ {
			setPixel(buf, stride, x, y, r, g, b, a)
		}
	}
}

func TestCompressBedrockSkinPadded128Lossless(t *testing.T) {
	// Bedrock clients often wrap a standard 64x64 skin inside a 128x128 canvas
	// where everything outside top-left 64x64 has alpha == 0.
	src := make([]byte, 128*128*4)
	stride128 := 128 * 4

	// Fill Head Base UV [8,0,16,8] and [0,8,32,8] with skin tone
	fillRect(src, stride128, 8, 0, 16, 8, 210, 161, 130, 255)
	fillRect(src, stride128, 0, 8, 32, 8, 210, 161, 130, 255)
	// Place a distinct 1px blue eye at (10, 12)
	setPixel(src, stride128, 10, 12, 20, 80, 220, 255)

	normRGBA, pngData, err := skinbridge.CompressBedrockSkinRGBA(src, 128, 128, "#d2a182")
	if err != nil {
		t.Fatalf("CompressBedrockSkinRGBA returned error: %v", err)
	}
	if len(normRGBA) != skinbridge.JavaSkinRGBABytes {
		t.Fatalf("len(normRGBA) = %d, want %d", len(normRGBA), skinbridge.JavaSkinRGBABytes)
	}

	// Verify exact lossless preservation of the 1px eye at (10, 12)
	r, g, b, a := getPixel(normRGBA, 64*4, 10, 12)
	if r != 20 || g != 80 || b != 220 || a != 255 {
		t.Fatalf("eye pixel at (10,12) = (%d,%d,%d,%d), want (20,80,220,255)", r, g, b, a)
	}

	// Verify PNG decodes to 64x64
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		t.Fatalf("png.Decode failed: %v", err)
	}
	if img.Bounds().Dx() != 64 || img.Bounds().Dy() != 64 {
		t.Fatalf("PNG bounds = %dx%d, want 64x64", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func TestCompressBedrockSkinTrueHD128PreservesFeatures(t *testing.T) {
	// True 128x128 HD skin (has opaque content outside top-left 64x64)
	src := make([]byte, 128*128*4)
	stride128 := 128 * 4

	// Fill entire 128x128 Head Base UV [0,16,64,16] with warm skin tone (220, 175, 145)
	fillRect(src, stride128, 0, 16, 64, 16, 220, 175, 145, 255)
	// Also put an opaque pixel at (80, 80) so it's detected as a true 128x128 HD skin
	setPixel(src, stride128, 80, 80, 100, 120, 140, 255)

	// Place a single 1px dark pupil at odd coordinates (x=21, y=25) inside the 2x2 block for (10, 12).
	// Naive nearest-neighbor (even indices only) would completely miss (21, 25),
	// and naive 2x2 box averaging would wash it out into skin tone.
	setPixel(src, stride128, 21, 25, 15, 20, 35, 255)

	normRGBA, _, err := skinbridge.CompressBedrockSkinRGBA(src, 128, 128, "#dcaf91")
	if err != nil {
		t.Fatalf("CompressBedrockSkinRGBA returned error: %v", err)
	}

	r, g, b, a := getPixel(normRGBA, 64*4, 10, 12)
	if a != 255 {
		t.Fatalf("pupil alpha = %d, want 255", a)
	}
	// The perceptual salience filter must preserve the dark pupil (substantially darker than 220,175,145)
	if r > 70 || g > 70 || b > 80 {
		t.Fatalf("salient 1px pupil at (21,25) was washed out to (%d,%d,%d); expected dark feature preservation", r, g, b)
	}
}

func TestCompressBedrockSkinBaseAlphaSanitizationAndOverlayCutout(t *testing.T) {
	src := make([]byte, 64*64*4)
	stride64 := 64 * 4

	// Leave Head Base UV [8,8,8,8] completely transparent (A=0),
	// except put a colored pixel on the matching Hat Overlay UV at (8+32, 8) = (40, 8)
	setPixel(src, stride64, 40, 8, 180, 40, 60, 255)

	// Also put a thin semi-transparent accessory pixel in Hat Overlay at (42, 10) with alpha=90
	setPixel(src, stride64, 42, 10, 50, 200, 240, 90)

	// Put garbage data in an unused UV region (60, 20) which is outside all Java Base & Overlay boxes
	setPixel(src, stride64, 60, 20, 123, 234, 56, 200)

	normRGBA, _, err := skinbridge.CompressBedrockSkinRGBA(src, 64, 64, "#d2a182")
	if err != nil {
		t.Fatalf("CompressBedrockSkinRGBA returned error: %v", err)
	}

	// 1. Base pixel (8, 8) must have inherited the matching overlay color (180, 40, 60) with A=255
	r, g, b, a := getPixel(normRGBA, stride64, 8, 8)
	if r != 180 || g != 40 || b != 60 || a != 255 {
		t.Fatalf("base pixel (8,8) = (%d,%d,%d,%d), want (180,40,60,255) from overlay fallback", r, g, b, a)
	}

	// 2. Distant base pixel (20, 28) in Body Base UV [16,20,24,12] with no neighbors or overlay
	// must be filled with parsed SkinColour (#d2a182 -> 210, 161, 130) and A=255, never black (0,0,0)!
	br, bg, bb, ba := getPixel(normRGBA, stride64, 20, 28)
	if br != 210 || bg != 161 || bb != 130 || ba != 255 {
		t.Fatalf("body base pixel (20,28) = (%d,%d,%d,%d), want (210,161,130,255) from SkinColour fallback", br, bg, bb, ba)
	}

	// 3. Overlay pixel (42, 10) must preserve transparency (0 < A < 255)
	_, _, _, oa := getPixel(normRGBA, stride64, 42, 10)
	if oa == 0 {
		t.Fatalf("overlay pixel (42,10) alpha was dropped to 0")
	}

	// 4. Unused UV padding at (60, 20) must be zeroed for optimal DEFLATE compression
	ur, ug, ub, ua := getPixel(normRGBA, stride64, 60, 20)
	if ur != 0 || ug != 0 || ub != 0 || ua != 0 {
		t.Fatalf("unused UV pixel (60,20) = (%d,%d,%d,%d), want (0,0,0,0)", ur, ug, ub, ua)
	}
}

func TestCompressBedrockSkinLegacy64x32Upgrade(t *testing.T) {
	src := make([]byte, 64*32*4)
	stride64 := 64 * 4

	// Paint Right Arm front face at (44, 20)
	setPixel(src, stride64, 44, 20, 250, 100, 30, 255)

	normRGBA, _, err := skinbridge.CompressBedrockSkinRGBA(src, 64, 32, "")
	if err != nil {
		t.Fatalf("CompressBedrockSkinRGBA failed on 64x32: %v", err)
	}

	// Mirrored Left Arm front face at (36..39, 52) should contain the mirrored pixel at (39, 52)
	r, g, b, a := getPixel(normRGBA, stride64, 39, 52)
	if r != 250 || g != 100 || b != 30 || a != 255 {
		t.Fatalf("mirrored left arm pixel (39,52) = (%d,%d,%d,%d), want (250,100,30,255)", r, g, b, a)
	}
}

func TestSkinBridgeServiceServes64x64PNGAndSlimMetadata(t *testing.T) {
	svc := skinbridge.New()
	svc.Start()

	id := uuid.New()
	src128 := make([]byte, 128*128*4)
	fillRect(src128, 128*4, 8, 8, 16, 16, 120, 180, 240, 255)

	url, normRGBA, pngData := svc.RegisterSkin(id, 128, 128, src128, "#d2a182")
	if url == "" {
		t.Fatal("RegisterSkin returned empty URL")
	}
	if len(normRGBA) != skinbridge.JavaSkinRGBABytes {
		t.Fatalf("len(normRGBA) = %d, want %d", len(normRGBA), skinbridge.JavaSkinRGBABytes)
	}
	if len(pngData) == 0 {
		t.Fatal("RegisterSkin returned empty pngData")
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s failed: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d, want 200", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("io.ReadAll failed: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("png.Decode HTTP response failed: %v", err)
	}
	if img.Bounds().Dx() != 64 || img.Bounds().Dy() != 64 {
		t.Fatalf("HTTP served PNG dimensions = %dx%d, want 64x64", img.Bounds().Dx(), img.Bounds().Dy())
	}

	// Verify TexturePropertyWithModel encodes slim metadata
	name, value := skinbridge.TexturePropertyWithModel(id, "BedrockPlayer", url, "slim")
	if name != "textures" {
		t.Fatalf("property name = %q, want \"textures\"", name)
	}
	rawJSON, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatalf("base64 decode failed: %v", err)
	}
	var payload struct {
		Textures struct {
			Skin struct {
				URL      string `json:"url"`
				Metadata struct {
					Model string `json:"model"`
				} `json:"metadata"`
			} `json:"SKIN"`
		} `json:"textures"`
	}
	if err := json.Unmarshal(rawJSON, &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if payload.Textures.Skin.URL != url {
		t.Fatalf("SKIN.url = %q, want %q", payload.Textures.Skin.URL, url)
	}
	if payload.Textures.Skin.Metadata.Model != "slim" {
		t.Fatalf("SKIN.metadata.model = %q, want \"slim\"", payload.Textures.Skin.Metadata.Model)
	}
}

func TestDefaultWorldSeedIsUnsetForRandomGeneration(t *testing.T) {
	cfg := config.Default()
	if cfg.World.Seed != 0 {
		t.Fatalf("config.Default().World.Seed = %d, want 0 (unset so server generates a random seed)", cfg.World.Seed)
	}
}
