package skinbridge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
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

type cachedSignedSkin struct {
	value     string
	signature string
}

var (
	mineSkinCacheMu sync.RWMutex
	mineSkinCache   = make(map[string]cachedSignedSkin)
)

// mineSkinNormalize ensures the PNG is a Java-compatible 64x64 PNG with sanitized
// base-layer alpha using CompressBedrockSkinRGBA.
func mineSkinNormalize(pngData []byte) []byte {
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return pngData
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return pngData
	}

	rgba := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			idx := (y*w + x) * 4
			rgba[idx] = c.R
			rgba[idx+1] = c.G
			rgba[idx+2] = c.B
			rgba[idx+3] = c.A
		}
	}

	_, compressedPNG, err := CompressBedrockSkinRGBA(rgba, w, h, "")
	if err != nil || len(compressedPNG) == 0 {
		return pngData
	}
	return compressedPNG
}

// UploadToMineSkin uploads a PNG to MineSkin API (with classic model default).
func UploadToMineSkin(pngData []byte, apiKey string) (value, signature string, err error) {
	return UploadToMineSkinWithModel(pngData, "classic", apiKey)
}

// UploadToMineSkinWithModel uploads a normalized 64x64 PNG to MineSkin API with the
// appropriate arm variant ("slim" or "classic") and caches signed results by SHA-256.
func UploadToMineSkinWithModel(pngData []byte, model, apiKey string) (value, signature string, err error) {
	pngData = mineSkinNormalize(pngData)

	variant := "classic"
	if strings.EqualFold(model, "slim") {
		variant = "slim"
	}

	// Check SHA-256 in-memory cache so reconnects or shared skins resolve in 0ms.
	hasher := sha256.New()
	hasher.Write(pngData)
	hasher.Write([]byte("|" + variant))
	cacheKey := hex.EncodeToString(hasher.Sum(nil))

	mineSkinCacheMu.RLock()
	if cached, ok := mineSkinCache[cacheKey]; ok && cached.value != "" && cached.signature != "" {
		mineSkinCacheMu.RUnlock()
		return cached.value, cached.signature, nil
	}
	mineSkinCacheMu.RUnlock()

	client := &http.Client{Timeout: 30 * time.Second}

	for attempt := 0; attempt < 2; attempt++ {
		var b bytes.Buffer
		w := multipart.NewWriter(&b)

		fw, err := w.CreateFormFile("file", "skin.png")
		if err != nil {
			return "", "", err
		}
		if _, err := fw.Write(pngData); err != nil {
			return "", "", err
		}
		if err := w.WriteField("variant", variant); err != nil {
			return "", "", err
		}
		// visibility = 0 (unlisted) so player skins don't clutter public listings
		if err := w.WriteField("visibility", "0"); err != nil {
			return "", "", err
		}
		if err := w.Close(); err != nil {
			return "", "", err
		}

		req, err := http.NewRequest("POST", "https://api.mineskin.org/generate/upload", &b)
		if err != nil {
			return "", "", err
		}
		req.Header.Set("Content-Type", w.FormDataContentType())
		req.Header.Set("User-Agent", "LivingWorld-SkinBridge/1.0 (+https://github.com/Aofsnorth/LivingWorld)")
		req.Header.Set("Accept", "application/json")
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}

		resp, err := client.Do(req)
		if err != nil {
			return "", "", err
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests && attempt == 0 {
			time.Sleep(3 * time.Second)
			continue
		}

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			return "", "", fmt.Errorf("mineskin error %d: %s", resp.StatusCode, string(respBody))
		}

		var res MineSkinResponse
		if err := json.Unmarshal(respBody, &res); err != nil {
			return "", "", err
		}

		val := res.Data.Texture.Value
		sig := res.Data.Texture.Signature
		if val != "" && sig != "" {
			mineSkinCacheMu.Lock()
			mineSkinCache[cacheKey] = cachedSignedSkin{value: val, signature: sig}
			mineSkinCacheMu.Unlock()
		}
		return val, sig, nil
	}

	return "", "", fmt.Errorf("mineskin upload exhausted retries")
}
