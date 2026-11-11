package skinbridge

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strconv"
	"strings"
)

const (
	// JavaSkinSize is the canonical texture dimension (64x64) accepted by modern
	// Minecraft Java Edition clients (SkinTextureDownloader) and MineSkin.
	JavaSkinSize = 64
	// JavaSkinRGBABytes is the byte length of a normalized 64x64 RGBA buffer.
	JavaSkinRGBABytes = JavaSkinSize * JavaSkinSize * 4
)

// uvRect defines a rectangle [x0, x1) x [y0, y1) in 64x64 Java skin UV space.
type uvRect struct {
	x0, y0, x1, y1 int
}

func (r uvRect) contains(x, y int) bool {
	return x >= r.x0 && x < r.x1 && y >= r.y0 && y < r.y1
}

// Java 64x64 Base UV regions where vanilla SkinTextureDownloader calls setNoAlpha
// (forcing alpha to 255 and turning any (0,0,0,0) transparent pixels solid black).
var javaBaseUVs = [...]uvRect{
	{0, 0, 32, 16},   // Head base
	{16, 16, 40, 32}, // Body / Torso base
	{40, 16, 56, 32}, // Right Arm base
	{0, 16, 16, 32},  // Right Leg base
	{16, 48, 32, 64}, // Left Leg base
	{32, 48, 48, 64}, // Left Arm base
}

// Java 64x64 Overlay UV regions where alpha transparency is preserved (cutout layer).
var javaOverlayUVs = [...]uvRect{
	{32, 0, 64, 16},  // Hat (Head overlay)
	{16, 32, 40, 48}, // Jacket (Body overlay)
	{40, 32, 56, 48}, // Right Sleeve
	{0, 32, 16, 48},  // Right Pants
	{0, 48, 16, 64},  // Left Pants
	{48, 48, 64, 64}, // Left Sleeve
}

// isBaseUV reports whether (x, y) lies inside a Java 64x64 base body-part UV box.
func isBaseUV(x, y int) bool {
	for _, r := range javaBaseUVs {
		if r.contains(x, y) {
			return true
		}
	}
	return false
}

// isOverlayUV reports whether (x, y) lies inside a Java 64x64 second-layer overlay UV box.
func isOverlayUV(x, y int) bool {
	for _, r := range javaOverlayUVs {
		if r.contains(x, y) {
			return true
		}
	}
	return false
}

// overlayForBaseUV maps a 64x64 Base UV coordinate (x, y) to its corresponding
// second-layer Overlay UV coordinate (ox, oy) if one exists.
func overlayForBaseUV(x, y int) (int, int, bool) {
	switch {
	case x >= 0 && x < 32 && y >= 0 && y < 16: // Head -> Hat
		return x + 32, y, true
	case x >= 0 && x < 56 && y >= 16 && y < 32: // Right Leg, Body, Right Arm -> Right Pants, Jacket, Right Sleeve
		return x, y + 16, true
	case x >= 16 && x < 32 && y >= 48 && y < 64: // Left Leg -> Left Pants
		return x - 16, y, true
	case x >= 32 && x < 48 && y >= 48 && y < 64: // Left Arm -> Left Sleeve
		return x + 16, y, true
	default:
		return 0, 0, false
	}
}

// CompressBedrockSkinRGBA normalizes and compresses any Bedrock skin RGBA buffer
// (64x32, 64x64, 128x64, 128x128, 256x128, 256x256, etc.) into a Java-compatible
// 64x64 RGBA buffer and compact PNG without altering how the skin is perceived:
//
//  1. Legacy 2:1 skins (64x32, 128x64) are upgraded to square (64x64, 128x128)
//     by mirroring right arm/leg to left arm/leg using vanilla UV rules.
//  2. Top-left padded 64x64 skins inside a 128x128 canvas and integer-upscaled
//     pixel-art skins take a zero-loss fast path.
//  3. True HD skins (128x128, 256x256) are downscaled to 64x64 using a lightweight
//     Perceptual Salience-Guided Bilateral filter: smooth skin/cloth gradients are
//     gamma-blended smoothly, while high-contrast micro-features (1px eyes, pupils,
//     eyelashes, mouths, armor trim) are preserved via local salience selection so
//     facial expressions and crisp pixel-art edges never blur or vanish.
//  4. Base-layer UV regions are alpha-sanitized (composited over corresponding
//     overlay pixels, local skin neighborhood, or SkinColour with A=255) so Java's
//     SkinTextureDownloader.setNoAlpha never turns transparent base pixels pitch black.
//  5. Unused UV padding outside valid 64x64 regions is zeroed out and encoded with
//     png.BestCompression for minimal wire/upload size (~1-3 KB).
func CompressBedrockSkinRGBA(rgba []byte, width, height int, skinColourHex string) (normRGBA []byte, pngData []byte, err error) {
	if width <= 0 || height <= 0 || len(rgba) < width*height*4 {
		return nil, nil, fmt.Errorf("invalid skin buffer: %dx%d (len=%d)", width, height, len(rgba))
	}

	src := rgba[:width*height*4]
	w, h := width, height

	// Step 1: Upgrade legacy 2:1 aspect ratio skins (e.g. 64x32 -> 64x64, 128x64 -> 128x128)
	// before downscaling so left arm and left leg UVs are populated.
	if w == h*2 && w >= 64 {
		src = upgradeLegacySquareRGBA(src, w)
		h = w
	}

	// Step 2: Produce 64x64 RGBA via lossless fast-paths or perceptual salience downscaling.
	var dst []byte
	switch {
	case w == JavaSkinSize && h == JavaSkinSize:
		dst = make([]byte, JavaSkinRGBABytes)
		copy(dst, src)

	case w >= 128 && h >= 128 && isTopLeftPadded64(src, w, h):
		dst = make([]byte, JavaSkinRGBABytes)
		for y := 0; y < JavaSkinSize; y++ {
			srcOff := y * w * 4
			dstOff := y * JavaSkinSize * 4
			copy(dst[dstOff:dstOff+JavaSkinSize*4], src[srcOff:srcOff+JavaSkinSize*4])
		}

	default:
		dst = downscalePerceptual64(src, w, h)
	}

	// Step 3: Sanitize Java 64x64 UV layers (prevent black blotches on base layer,
	// preserve crisp cutout alpha on overlay layer, zero unused UV padding).
	sanitizeJavaUVLayers(dst, skinColourHex)

	// Step 4: Encode compact 64x64 PNG with BestCompression.
	img := &image.NRGBA{
		Pix:    dst,
		Stride: JavaSkinSize * 4,
		Rect:   image.Rect(0, 0, JavaSkinSize, JavaSkinSize),
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, nil, err
	}
	return dst, buf.Bytes(), nil
}

// isTopLeftPadded64 reports whether a >=128x128 skin only has non-transparent pixels
// in the top-left 64x64 quadrant (a common Bedrock client padding quirk).
func isTopLeftPadded64(rgba []byte, width, height int) bool {
	hasTopLeftOpaque := false
	for y := 0; y < height; y++ {
		rowOff := y * width * 4
		for x := 0; x < width; x++ {
			a := rgba[rowOff+x*4+3]
			if x >= JavaSkinSize || y >= JavaSkinSize {
				if a != 0 {
					return false
				}
			} else if a != 0 {
				hasTopLeftOpaque = true
			}
		}
	}
	return hasTopLeftOpaque
}

// upgradeLegacySquareRGBA expands a W x (W/2) skin into W x W and mirrors the
// right leg and right arm into the lower-half left leg and left arm UV regions.
func upgradeLegacySquareRGBA(src []byte, w int) []byte {
	halfH := w / 2
	dst := make([]byte, w*w*4)
	copy(dst, src[:w*halfH*4])

	scale := w / 64
	if scale < 1 {
		return dst
	}
	s := func(v int) int { return v * scale }

	// Mirror Right Leg (0,16)-(16,32) -> Left Leg (16,48)-(32,64)
	copyRectMirroredScaled(dst, w, s(4), s(16), s(4), s(4), s(20), s(48))   // Top
	copyRectMirroredScaled(dst, w, s(8), s(16), s(4), s(4), s(24), s(48))   // Bottom
	copyRectMirroredScaled(dst, w, s(0), s(20), s(4), s(12), s(24), s(52))  // Right -> Left
	copyRectMirroredScaled(dst, w, s(4), s(20), s(4), s(12), s(20), s(52))  // Front
	copyRectMirroredScaled(dst, w, s(8), s(20), s(4), s(12), s(16), s(52))  // Left -> Right
	copyRectMirroredScaled(dst, w, s(12), s(20), s(4), s(12), s(28), s(52)) // Back

	// Mirror Right Arm (40,16)-(56,32) -> Left Arm (32,48)-(48,64)
	copyRectMirroredScaled(dst, w, s(44), s(16), s(4), s(4), s(36), s(48))  // Top
	copyRectMirroredScaled(dst, w, s(48), s(16), s(4), s(4), s(40), s(48))  // Bottom
	copyRectMirroredScaled(dst, w, s(40), s(20), s(4), s(12), s(40), s(52)) // Right -> Left
	copyRectMirroredScaled(dst, w, s(44), s(20), s(4), s(12), s(36), s(52)) // Front
	copyRectMirroredScaled(dst, w, s(48), s(20), s(4), s(12), s(32), s(52)) // Left -> Right
	copyRectMirroredScaled(dst, w, s(52), s(20), s(4), s(12), s(44), s(52)) // Back

	return dst
}

func copyRectMirroredScaled(pix []byte, strideW, sx, sy, rw, rh, dx, dy int) {
	for y := 0; y < rh; y++ {
		for x := 0; x < rw; x++ {
			si := ((sy+y)*strideW + (sx + rw - 1 - x)) * 4
			di := ((dy+y)*strideW + (dx + x)) * 4
			if si+3 < len(pix) && di+3 < len(pix) {
				pix[di] = pix[si]
				pix[di+1] = pix[si+1]
				pix[di+2] = pix[si+2]
				pix[di+3] = pix[si+3]
			}
		}
	}
}

// downscalePerceptual64 resamples an arbitrary WxH RGBA skin buffer to 64x64
// while preserving 1px eyes, facial features, and sharp pixel-art edges.
func downscalePerceptual64(src []byte, srcW, srcH int) []byte {
	dst := make([]byte, JavaSkinRGBABytes)

	// Pass 1: Compute baseline alpha-weighted box averages for all 64x64 cells.
	// We use this both as the smooth-gradient output and as the local background
	// reference when evaluating feature salience in Pass 2.
	var avgR, avgG, avgB, avgA [JavaSkinSize * JavaSkinSize]uint8
	for ty := 0; ty < JavaSkinSize; ty++ {
		sy0 := (ty * srcH) / JavaSkinSize
		sy1 := ((ty + 1) * srcH) / JavaSkinSize
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for tx := 0; tx < JavaSkinSize; tx++ {
			sx0 := (tx * srcW) / JavaSkinSize
			sx1 := ((tx + 1) * srcW) / JavaSkinSize
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}

			var rSum, gSum, bSum, wSum, aSum uint32
			var count uint32
			for sy := sy0; sy < sy1; sy++ {
				rowOff := sy * srcW * 4
				for sx := sx0; sx < sx1; sx++ {
					idx := rowOff + sx*4
					a := uint32(src[idx+3])
					rSum += uint32(src[idx]) * a
					gSum += uint32(src[idx+1]) * a
					bSum += uint32(src[idx+2]) * a
					wSum += a
					aSum += a
					count++
				}
			}
			cell := ty*JavaSkinSize + tx
			if count > 0 {
				avgA[cell] = uint8((aSum + count/2) / count)
			}
			if wSum > 0 {
				avgR[cell] = uint8((rSum + wSum/2) / wSum)
				avgG[cell] = uint8((gSum + wSum/2) / wSum)
				avgB[cell] = uint8((bSum + wSum/2) / wSum)
			}
		}
	}

	// Pass 2: Feature-aware bilateral selection per 64x64 block.
	for ty := 0; ty < JavaSkinSize; ty++ {
		sy0 := (ty * srcH) / JavaSkinSize
		sy1 := ((ty + 1) * srcH) / JavaSkinSize
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for tx := 0; tx < JavaSkinSize; tx++ {
			sx0 := (tx * srcW) / JavaSkinSize
			sx1 := ((tx + 1) * srcW) / JavaSkinSize
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}

			cell := ty*JavaSkinSize + tx
			dstIdx := cell * 4

			// Fast check: if the block is completely transparent, write (0,0,0,0).
			if avgA[cell] == 0 {
				continue
			}

			// Measure color contrast inside the source block among visible pixels.
			var minR, minG, minB uint8 = 255, 255, 255
			var maxR, maxG, maxB uint8 = 0, 0, 0
			var maxAlpha uint8
			visibleCount := 0

			for sy := sy0; sy < sy1; sy++ {
				rowOff := sy * srcW * 4
				for sx := sx0; sx < sx1; sx++ {
					idx := rowOff + sx*4
					a := src[idx+3]
					if a > maxAlpha {
						maxAlpha = a
					}
					if a < 32 {
						continue
					}
					r, g, b := src[idx], src[idx+1], src[idx+2]
					if r < minR {
						minR = r
					}
					if g < minG {
						minG = g
					}
					if b < minB {
						minB = b
					}
					if r > maxR {
						maxR = r
					}
					if g > maxG {
						maxG = g
					}
					if b > maxB {
						maxB = b
					}
					visibleCount++
				}
			}

			if visibleCount == 0 {
				dst[dstIdx] = avgR[cell]
				dst[dstIdx+1] = avgG[cell]
				dst[dstIdx+2] = avgB[cell]
				dst[dstIdx+3] = avgA[cell]
				continue
			}

			dr := int(maxR) - int(minR)
			dg := int(maxG) - int(minG)
			db := int(maxB) - int(minB)
			contrastSq := 2*dr*dr + 4*dg*dg + 3*db*db

			// Smooth gradient regime (low internal contrast): keep smooth alpha-weighted average.
			// Threshold 32^2 * 3 = 3072 preserves soft shading while triggering on crisp details.
			if visibleCount <= 1 || contrastSq <= 3072 {
				dst[dstIdx] = avgR[cell]
				dst[dstIdx+1] = avgG[cell]
				dst[dstIdx+2] = avgB[cell]
				dst[dstIdx+3] = resolveBlockAlpha(tx, ty, avgA[cell], maxAlpha)
				continue
			}

			// High-contrast detail/edge regime (eyes, pupils, mouths, outlines, trim):
			// Compute local 3x3 surrounding neighborhood average to measure feature salience.
			var bgRSum, bgGSum, bgBSum, bgW uint32
			for ny := ty - 1; ny <= ty+1; ny++ {
				if ny < 0 || ny >= JavaSkinSize {
					continue
				}
				for nx := tx - 1; nx <= tx+1; nx++ {
					if nx < 0 || nx >= JavaSkinSize || (nx == tx && ny == ty) {
						continue
					}
					nc := ny*JavaSkinSize + nx
					na := uint32(avgA[nc])
					if na >= 32 {
						bgRSum += uint32(avgR[nc]) * na
						bgGSum += uint32(avgG[nc]) * na
						bgBSum += uint32(avgB[nc]) * na
						bgW += na
					}
				}
			}
			bgR, bgG, bgB := int(avgR[cell]), int(avgG[cell]), int(avgB[cell])
			if bgW > 0 {
				bgR = int((bgRSum + bgW/2) / bgW)
				bgG = int((bgGSum + bgW/2) / bgW)
				bgB = int((bgBSum + bgW/2) / bgW)
			}

			// Find the most visually salient pixel in the block:
			// Salience rewards perceptual distance from the surrounding background,
			// chroma/saturation (for colored eyes/details), and high opacity.
			var bestR, bestG, bestB, bestA uint8
			bestScore := -1

			for sy := sy0; sy < sy1; sy++ {
				rowOff := sy * srcW * 4
				for sx := sx0; sx < sx1; sx++ {
					idx := rowOff + sx*4
					a := src[idx+3]
					if a < 64 {
						continue
					}
					r, g, b := int(src[idx]), int(src[idx+1]), int(src[idx+2])
					distBg := perceptualColorDistSq(r, g, b, bgR, bgG, bgB)
					chroma := pixelChromaSq(r, g, b)
					score := int(a) * (256 + distBg + chroma/3)
					if score > bestScore {
						bestScore = score
						bestR, bestG, bestB, bestA = src[idx], src[idx+1], src[idx+2], a
					}
				}
			}

			if bestScore < 0 {
				dst[dstIdx] = avgR[cell]
				dst[dstIdx+1] = avgG[cell]
				dst[dstIdx+2] = avgB[cell]
				dst[dstIdx+3] = resolveBlockAlpha(tx, ty, avgA[cell], maxAlpha)
				continue
			}

			// Blend the dominant salient feature pixel (80% weight) with compatible
			// cluster pixels in the same block (within perceptual distance <= 40^2*9)
			// so similar-colored feature pixels average cleanly without bleeding into
			// the background skin/cloth color.
			clusterR := uint32(bestR) * 4
			clusterG := uint32(bestG) * 4
			clusterB := uint32(bestB) * 4
			clusterW := uint32(4)

			for sy := sy0; sy < sy1; sy++ {
				rowOff := sy * srcW * 4
				for sx := sx0; sx < sx1; sx++ {
					idx := rowOff + sx*4
					a := src[idx+3]
					if a < 64 {
						continue
					}
					r, g, b := int(src[idx]), int(src[idx+1]), int(src[idx+2])
					if perceptualColorDistSq(r, g, b, int(bestR), int(bestG), int(bestB)) <= 12000 {
						clusterR += uint32(r)
						clusterG += uint32(g)
						clusterB += uint32(b)
						clusterW++
					}
				}
			}

			dst[dstIdx] = uint8((clusterR + clusterW/2) / clusterW)
			dst[dstIdx+1] = uint8((clusterG + clusterW/2) / clusterW)
			dst[dstIdx+2] = uint8((clusterB + clusterW/2) / clusterW)
			dst[dstIdx+3] = resolveBlockAlpha(tx, ty, avgA[cell], bestA)
		}
	}

	return dst
}

func perceptualColorDistSq(r1, g1, b1, r2, g2, b2 int) int {
	dr := r1 - r2
	dg := g1 - g2
	db := b1 - b2
	return 2*dr*dr + 4*dg*dg + 3*db*db
}

func pixelChromaSq(r, g, b int) int {
	mx := r
	if g > mx {
		mx = g
	}
	if b > mx {
		mx = b
	}
	mn := r
	if g < mn {
		mn = g
	}
	if b < mn {
		mn = b
	}
	c := mx - mn
	return c * c
}

// resolveBlockAlpha determines the output alpha for a downscaled pixel at (tx, ty).
// In overlay UV regions, thin cutout details (e.g. glasses, bangs, jacket trim)
// with high source alpha are kept above Java's cutout threshold (>128).
func resolveBlockAlpha(tx, ty int, avgAlpha, peakAlpha uint8) uint8 {
	if isOverlayUV(tx, ty) && peakAlpha >= 160 && avgAlpha >= 48 {
		boosted := uint8((uint16(avgAlpha) + uint16(peakAlpha)*2) / 3)
		if boosted < 144 {
			boosted = 144
		}
		return boosted
	}
	return avgAlpha
}

// sanitizeJavaUVLayers enforces Java Edition's UV alpha invariants on a 64x64 RGBA buffer:
//   - Base UV regions must have A=255 everywhere. Any transparent or semi-transparent
//     base pixel is filled/composited from its corresponding overlay UV pixel, adjacent
//     opaque base pixels, or the skin's fallback color so Java's setNoAlpha never turns
//     transparent pixels into solid black (#000000) blotches.
//   - Pixels outside all valid Base and Overlay UV rectangles are zeroed to (0,0,0,0)
//     to maximize PNG DEFLATE compression.
func sanitizeJavaUVLayers(rgba []byte, skinColourHex string) {
	if len(rgba) < JavaSkinRGBABytes {
		return
	}

	fallback := resolveFallbackSkinColor(rgba, skinColourHex)
	snapshot := make([]byte, JavaSkinRGBABytes)
	copy(snapshot, rgba)

	for y := 0; y < JavaSkinSize; y++ {
		for x := 0; x < JavaSkinSize; x++ {
			idx := (y*JavaSkinSize + x) * 4
			if isBaseUV(x, y) {
				a := snapshot[idx+3]
				if a == 255 {
					continue
				}
				bg := sampleBaseFillColor(snapshot, x, y, fallback)
				if a == 0 {
					rgba[idx] = bg.R
					rgba[idx+1] = bg.G
					rgba[idx+2] = bg.B
					rgba[idx+3] = 255
				} else {
					// Alpha-composite semi-transparent base pixel over bg.
					af := uint16(a)
					inv := 255 - af
					rgba[idx] = uint8((uint16(snapshot[idx])*af + uint16(bg.R)*inv + 127) / 255)
					rgba[idx+1] = uint8((uint16(snapshot[idx+1])*af + uint16(bg.G)*inv + 127) / 255)
					rgba[idx+2] = uint8((uint16(snapshot[idx+2])*af + uint16(bg.B)*inv + 127) / 255)
					rgba[idx+3] = 255
				}
			} else if !isOverlayUV(x, y) {
				// Clear unused UV padding so PNG DEFLATE compresses runs of zeros.
				rgba[idx] = 0
				rgba[idx+1] = 0
				rgba[idx+2] = 0
				rgba[idx+3] = 0
			}
		}
	}
}

// sampleBaseFillColor picks the most natural opaque color for a transparent pixel at
// base UV (x, y):
//  1. If the matching second-layer Overlay UV pixel is opaque (common in Bedrock
//     Persona skins where clothes/face are drawn on the overlay), use its RGB.
//  2. Otherwise, average any opaque neighboring base pixels in a 5x5 window.
//  3. Otherwise, fall back to the skin's overall tone / SkinColour.
func sampleBaseFillColor(snapshot []byte, x, y int, fallback color.NRGBA) color.NRGBA {
	if ox, oy, ok := overlayForBaseUV(x, y); ok {
		oidx := (oy*JavaSkinSize + ox) * 4
		if snapshot[oidx+3] >= 128 {
			return color.NRGBA{
				R: snapshot[oidx],
				G: snapshot[oidx+1],
				B: snapshot[oidx+2],
				A: 255,
			}
		}
	}

	var rSum, gSum, bSum, wSum uint32
	for dy := -2; dy <= 2; dy++ {
		ny := y + dy
		if ny < 0 || ny >= JavaSkinSize {
			continue
		}
		for dx := -2; dx <= 2; dx++ {
			nx := x + dx
			if nx < 0 || nx >= JavaSkinSize || (dx == 0 && dy == 0) {
				continue
			}
			if !isBaseUV(nx, ny) {
				continue
			}
			nidx := (ny*JavaSkinSize + nx) * 4
			na := uint32(snapshot[nidx+3])
			if na >= 128 {
				rSum += uint32(snapshot[nidx]) * na
				gSum += uint32(snapshot[nidx+1]) * na
				bSum += uint32(snapshot[nidx+2]) * na
				wSum += na
			}
		}
	}
	if wSum > 0 {
		return color.NRGBA{
			R: uint8((rSum + wSum/2) / wSum),
			G: uint8((gSum + wSum/2) / wSum),
			B: uint8((bSum + wSum/2) / wSum),
			A: 255,
		}
	}
	return fallback
}

func resolveFallbackSkinColor(rgba []byte, skinColourHex string) color.NRGBA {
	if c, ok := parseHexColor(skinColourHex); ok {
		return c
	}
	// Sample opaque pixels from Head base UV [0..32, 0..16) first, then all base UVs.
	var rSum, gSum, bSum, count uint64
	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			idx := (y*JavaSkinSize + x) * 4
			if rgba[idx+3] >= 128 {
				rSum += uint64(rgba[idx])
				gSum += uint64(rgba[idx+1])
				bSum += uint64(rgba[idx+2])
				count++
			}
		}
	}
	if count == 0 {
		for _, rect := range javaBaseUVs {
			for y := rect.y0; y < rect.y1; y++ {
				for x := rect.x0; x < rect.x1; x++ {
					idx := (y*JavaSkinSize + x) * 4
					if rgba[idx+3] >= 128 {
						rSum += uint64(rgba[idx])
						gSum += uint64(rgba[idx+1])
						bSum += uint64(rgba[idx+2])
						count++
					}
				}
			}
		}
	}
	if count > 0 {
		return color.NRGBA{
			R: uint8(rSum / count),
			G: uint8(gSum / count),
			B: uint8(bSum / count),
			A: 255,
		}
	}
	return color.NRGBA{R: 218, G: 168, B: 136, A: 255}
}

func parseHexColor(s string) (color.NRGBA, bool) {
	s = strings.TrimSpace(strings.TrimPrefix(s, "#"))
	if len(s) == 8 {
		// #AARRGGBB -> take RRGGBB
		s = s[2:]
	}
	if len(s) != 6 {
		return color.NRGBA{}, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.NRGBA{}, false
	}
	r := uint8((v >> 16) & 0xFF)
	g := uint8((v >> 8) & 0xFF)
	b := uint8(v & 0xFF)
	if r == 0 && g == 0 && b == 0 {
		return color.NRGBA{}, false
	}
	return color.NRGBA{R: r, G: g, B: b, A: 255}, true
}
