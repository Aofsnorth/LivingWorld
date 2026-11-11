package server

import (
	"log"

	javaskin "livingworld/internal/java/skin"
	"livingworld/internal/skinbridge"

	"github.com/google/uuid"
)

// uploadBedrockSkinToMineSkin compresses a Bedrock player's raw RGBA skin into a
// perceptual 64x64 PNG and uploads it to MineSkin (async) to obtain a Mojang-signed
// texture property.
func (s *Server) uploadBedrockSkinToMineSkin(playerID uuid.UUID, playerName string, rgba []byte, w, h int) {
	_, pngData, err := skinbridge.CompressBedrockSkinRGBA(rgba, w, h, "")
	if err != nil {
		log.Printf("[Bedrock->MineSkin] %s: skin compression failed for %dx%d: %v", playerName, w, h, err)
		return
	}
	s.uploadBedrockSkinPNGToMineSkin(playerID, playerName, pngData, "wide")
}

// uploadBedrockSkinPNGToMineSkin uploads an already-normalized 64x64 PNG to MineSkin
// (with SHA-256 caching and model variant propagation). Once complete,
// UpdateProfileProperty triggers EventSkin so Java clients despawn+respawn the
// avatar with the signed texture property.
func (s *Server) uploadBedrockSkinPNGToMineSkin(playerID uuid.UUID, playerName string, pngData []byte, model string) {
	if len(pngData) == 0 {
		return
	}
	apiKey := ""
	if s.cfg != nil {
		apiKey = s.cfg.Java.MineSkinAPIKey
	}

	log.Printf("[Bedrock->MineSkin] %s: uploading compressed 64x64 skin (%d B, model=%s)...", playerName, len(pngData), model)
	value, signature, err := skinbridge.UploadToMineSkinWithModel(pngData, model, apiKey)
	if err != nil {
		log.Printf("[Bedrock->MineSkin] %s: upload failed: %v", playerName, err)
		// Fallback: if MineSkin is unreachable/rate-limited and Java SkinSource is enabled,
		// check if the player's username has a signed skin on Mojang/Ely.by so vanilla
		// Java clients still get a signed texture if one exists.
		if s.cfg != nil && s.cfg.Java.SkinSource != "" && s.cfg.Java.SkinSource != "off" {
			if _, _, fallbackVal, fallbackSig := javaskin.FetchMojangSkin(playerName); fallbackVal != "" && fallbackSig != "" {
				log.Printf("[Bedrock->MineSkin] %s: applied signed Mojang skin fallback", playerName)
				s.pm.UpdateProfileProperty(playerID, "textures", fallbackVal, fallbackSig)
			}
		}
		return
	}
	log.Printf("[Bedrock->MineSkin] %s: upload success, updating profile property (signed=%t)", playerName, signature != "")
	s.pm.UpdateProfileProperty(playerID, "textures", value, signature)
}
