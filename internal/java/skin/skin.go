package skin

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"log"
	"net/http"
	"strings"
)

type TexturePropertyJSON struct {
	Textures struct {
		Skin struct {
			URL      string `json:"url"`
			Metadata struct {
				Model string `json:"model"`
			} `json:"metadata"`
		} `json:"SKIN"`
	} `json:"textures"`
}

func ParseJavaSkinProperty(base64Val string) (url string, model string) {
	decoded, err := base64.StdEncoding.DecodeString(base64Val)
	if err != nil {
		return "", "wide"
	}
	var tex TexturePropertyJSON
	if err := json.Unmarshal(decoded, &tex); err != nil {
		return "", "wide"
	}
	url = tex.Textures.Skin.URL
	model = "wide"
	if tex.Textures.Skin.Metadata.Model == "slim" {
		model = "slim"
	}
	return url, model
}

func DownloadAndDecodeSkin(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("http get failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	img, err := png.Decode(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("png decode failed: %w", err)
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if (w != 64 || (h != 64 && h != 32)) && (w != 128 || (h != 128 && h != 64)) {
		return nil, fmt.Errorf("invalid skin dimensions: %dx%d", w, h)
	}

	rgba := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, a := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			idx := (y*w + x) * 4
			rgba[idx] = byte(r >> 8)
			rgba[idx+1] = byte(g >> 8)
			rgba[idx+2] = byte(b >> 8)
			rgba[idx+3] = byte(a >> 8)
		}
	}
	return rgba, nil
}

// FetchMojangSkin looks up a player's skin via Mojang API by username.
// This is needed when the server runs in offline mode and the client doesn't
// provide textures profile properties.
func FetchMojangSkin(username string) (skinURL, model, value, signature string) {
	// Step 1: Get the Mojang UUID for this username.
	resp, err := http.Get("https://api.mojang.com/users/profiles/minecraft/" + username)
	if err != nil {
		log.Printf("[Java] Mojang API lookup failed for %s: %v", username, err)
		return "", "wide", "", ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("[Java] Mojang API returned %d for %s", resp.StatusCode, username)
		return "", "wide", "", ""
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "wide", "", ""
	}
	var profile struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &profile); err != nil || profile.ID == "" {
		log.Printf("[Java] Mojang profile parse failed for %s: %v", username, err)
		return "", "wide", "", ""
	}

	// Step 2: Get the session profile with textures.
	resp2, err := http.Get("https://sessionserver.mojang.com/session/minecraft/profile/" + profile.ID)
	if err != nil {
		log.Printf("[Java] Mojang session lookup failed for %s: %v", username, err)
		return "", "wide", "", ""
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		return "", "wide", "", ""
	}
	body2, err := io.ReadAll(resp2.Body)
	if err != nil {
		return "", "wide", "", ""
	}
	var sessionProfile struct {
		Properties []struct {
			Name      string `json:"name"`
			Value     string `json:"value"`
			Signature string `json:"signature"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(body2, &sessionProfile); err != nil {
		return "", "wide", "", ""
	}

	for _, prop := range sessionProfile.Properties {
		if strings.EqualFold(prop.Name, "textures") {
			u, m := ParseJavaSkinProperty(prop.Value)
			return u, m, prop.Value, prop.Signature
		}
	}
	return "", "wide", "", ""
}
