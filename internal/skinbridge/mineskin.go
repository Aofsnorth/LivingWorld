package skinbridge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

type MineSkinResponse struct {
	Data struct {
		Texture struct {
			Value     string `json:"value"`
			Signature string `json:"signature"`
		} `json:"texture"`
	} `json:"data"`
}

// mineSkinNormalize ensures the PNG is a dimension MineSkin accepts (64x64 or
// 64x32). HD skins served to Java are kept at 128x128 by RegisterRGBA, but
// MineSkin rejects anything other than 64x64/64x32, so for the upload path only
// we downscale a 128x128 skin to 64x64 (2:1 box average — the signed texture is
// only used to satisfy Java's skin-URL whitelist, not for HD rendering). Skins
// already 64x64/64x32 pass through untouched.
func mineSkinNormalize(pngData []byte) []byte {
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return pngData // not a PNG we can read; let the API reject/accept as-is
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if (w == 64 && h == 64) || (w == 64 && h == 32) {
		return pngData // already an accepted dimension
	}
	if w != 128 || h != 128 {
		return pngData // unexpected size; don't guess, send original
	}
	// 128x128 → 64x64 with an ALPHA-WEIGHTED average of each 2x2 block: RGB is
	// averaged weighted by the source alpha (so fully-transparent overlay pixels
	// don't bleed black halos into edges), and alpha is averaged. This is smoother
	// than the previous most-opaque pick (which kept harsh/noisy pixels — the
	// "burik" look) while staying edge-correct, and it's the best a 64x64 skin can
	// represent of a 128 source.
	out := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			var r, g, bl, wsum, asum float64
			for dy := 0; dy < 2; dy++ {
				for dx := 0; dx < 2; dx++ {
					c := color.NRGBAModel.Convert(img.At(b.Min.X+x*2+dx, b.Min.Y+y*2+dy)).(color.NRGBA)
					w := float64(c.A)
					r += float64(c.R) * w
					g += float64(c.G) * w
					bl += float64(c.B) * w
					wsum += w
					asum += float64(c.A)
				}
			}
			px := color.NRGBA{A: uint8(asum / 4)}
			if wsum > 0 {
				px.R, px.G, px.B = uint8(r/wsum), uint8(g/wsum), uint8(bl/wsum)
			}
			out.SetNRGBA(x, y, px)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return pngData
	}
	return buf.Bytes()
}

// UploadToMineSkin uploads a PNG to MineSkin API v1 (with Bearer token if using v2 keys)
func UploadToMineSkin(pngData []byte, apiKey string) (value, signature string, err error) {
	pngData = mineSkinNormalize(pngData)

	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	// MineSkin v1 expects "file"
	fw, err := w.CreateFormFile("file", "skin.png")
	if err != nil {
		return "", "", err
	}
	_, err = fw.Write(pngData)
	if err != nil {
		return "", "", err
	}

	// Add variant = classic (or slim, but we assume classic for now)
	if err := w.WriteField("variant", "classic"); err != nil {
		return "", "", err
	}
	// visibility = 1 (public) or 0 (private). Use private to not spam the gallery
	if err := w.WriteField("visibility", "0"); err != nil {
		return "", "", err
	}
	w.Close()

	req, err := http.NewRequest("POST", "https://api.mineskin.org/generate/upload", &b)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("mineskin error %d: %s", resp.StatusCode, string(body))
	}

	var res MineSkinResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", "", err
	}

	return res.Data.Texture.Value, res.Data.Texture.Signature, nil
}
