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
	"time"
)

// mojangClient is used for all Mojang API calls. A timeout keeps a slow/hanging
// Mojang response from blocking the player-join flow indefinitely.
var mojangClient = &http.Client{Timeout: 6 * time.Second}

// mojangGet performs a GET with a browser-like User-Agent. Some Mojang/Cloudflare
// endpoints rate-limit or reject the default Go user agent.
func mojangGet(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "LivingWorld/0.1 (+https://github.com/livingworld)")
	return mojangClient.Do(req)
}

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
	resp, err := mojangGet("https://api.mojang.com/users/profiles/minecraft/" + username)
	if err != nil {
		log.Printf("[Java] Mojang API lookup failed for %s: %v", username, err)
		return "", "wide", "", ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		// 404/204 means the name isn't a premium account (e.g. a cracked name):
		// there is simply no Mojang skin to show, so the client renders default.
		log.Printf("[Java] Mojang API returned %d for %q (no premium skin available)", resp.StatusCode, username)
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

	// Step 2: Get the session profile WITH the texture signature (unsigned=false).
	// A signed, whitelisted-domain (textures.minecraft.net) texture is the form
	// vanilla clients always accept.
	resp2, err := mojangGet("https://sessionserver.mojang.com/session/minecraft/profile/" + profile.ID + "?unsigned=false")
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
			log.Printf("[Java] Mojang skin resolved for %q: model=%s signed=%t url=%s", username, m, prop.Signature != "", u)
			return u, m, prop.Value, prop.Signature
		}
	}
	log.Printf("[Java] Mojang profile for %q has no textures property", username)
	return "", "wide", "", ""
}

// FetchAnySkin resolves a skin from any available source automatically, so the
// server works for mixed premium + cracked player bases without configuration.
// Order: Ely.by first (LegacyLauncher/TLauncher cracked skins), then Mojang
// (premium). Ely.by is tried first because premium names without an Ely.by skin
// return 404 quickly and fall through to Mojang, whereas Mojang-first would serve
// the premium *owner's* skin for a cracked name that happens to be registered.
func FetchAnySkin(username string) (skinURL, model, value, signature string) {
	if u, m, v, s := FetchElySkin(username); u != "" {
		return u, m, v, s
	}
	if u, m, v, s := FetchMojangSkin(username); u != "" {
		return u, m, v, s
	}
	log.Printf("[Java] No skin found for %q in any source", username)
	return "", "wide", "", ""
}

// FetchElySkin looks up a player's skin from the Ely.by skin system by username.
// This is the skin store used by LegacyLauncher / TLauncher and other cracked
// launchers. The returned texture value is UNSIGNED; clients using
// authlib-injector (which those launchers inject) accept the ely.by domain, and
// the Bedrock bridge downloads the PNG directly from skinURL.
func FetchElySkin(username string) (skinURL, model, value, signature string) {
	resp, err := mojangGet("http://skinsystem.ely.by/textures/" + username)
	if err != nil {
		log.Printf("[Java] Ely.by lookup failed for %q: %v", username, err)
		return "", "wide", "", ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("[Java] Ely.by returned %d for %q (no skin set there)", resp.StatusCode, username)
		return "", "wide", "", ""
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "wide", "", ""
	}
	// /textures/{name} returns the inner texture map: {"SKIN":{"url":...,"metadata":{"model":...}}}
	var tex struct {
		Skin struct {
			URL      string `json:"url"`
			Metadata struct {
				Model string `json:"model"`
			} `json:"metadata"`
		} `json:"SKIN"`
	}
	if err := json.Unmarshal(body, &tex); err != nil || tex.Skin.URL == "" {
		log.Printf("[Java] Ely.by profile for %q has no skin", username)
		return "", "wide", "", ""
	}
	model = "wide"
	if tex.Skin.Metadata.Model == "slim" {
		model = "slim"
	}
	value = buildTextureValue(username, tex.Skin.URL, model)
	log.Printf("[Java] Ely.by skin resolved for %q: model=%s url=%s", username, model, tex.Skin.URL)
	return tex.Skin.URL, model, value, ""
}

// buildTextureValue builds an (unsigned) base64 texture-property value pointing at
// an arbitrary skin URL, in the same shape Mojang uses.
func buildTextureValue(name, url, model string) string {
	type meta struct {
		Model string `json:"model,omitempty"`
	}
	type skinObj struct {
		URL      string `json:"url"`
		Metadata *meta  `json:"metadata,omitempty"`
	}
	payload := struct {
		Timestamp   int64  `json:"timestamp"`
		ProfileName string `json:"profileName"`
		Textures    struct {
			Skin skinObj `json:"SKIN"`
		} `json:"textures"`
	}{
		Timestamp:   time.Now().UnixMilli(),
		ProfileName: name,
	}
	payload.Textures.Skin.URL = url
	if model == "slim" {
		payload.Textures.Skin.Metadata = &meta{Model: "slim"}
	}
	b, _ := json.Marshal(payload)
	return base64.StdEncoding.EncodeToString(b)
}
