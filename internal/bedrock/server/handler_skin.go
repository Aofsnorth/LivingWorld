package server

import (
	"bytes"
	"image"
	"image/png"
	"log"

	"livingworld/internal/skinbridge"

	"github.com/google/uuid"
)

// uploadBedrockSkinToMineSkin uploads a Bedrock player's skin to MineSkin (async)
// to get a signed texture property on a Mojang-whitelisted domain. Once complete,
// UpdateProfileProperty triggers EventSkin â†’ Java clients despawn+respawn the
// avatar with the new property, making the skin visible to vanilla Java clients.
func (s *Server) uploadBedrockSkinToMineSkin(playerID uuid.UUID, playerName string, rgba []byte, w, h int) {
	// Convert RGBA to PNG for MineSkin upload
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	if len(rgba) >= w*h*4 {
		copy(img.Pix, rgba[:w*h*4])
	} else {
		log.Printf("[Bedrockâ†’MineSkin] %s: invalid RGBA size %d for %dx%d", playerName, len(rgba), w, h)
		return
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		log.Printf("[Bedrockâ†’MineSkin] %s: PNG encode failed: %v", playerName, err)
		return
	}

	log.Printf("[Bedrockâ†’MineSkin] %s: uploading %dx%d skin (%d KB)...", playerName, w, h, buf.Len()/1024)
	value, signature, err := skinbridge.UploadToMineSkin(buf.Bytes(), s.cfg.Java.MineSkinAPIKey)
	if err != nil {
		log.Printf("[Bedrockâ†’MineSkin] %s: upload failed: %v", playerName, err)
		return
	}
	log.Printf("[Bedrockâ†’MineSkin] %s: upload success, updating profile property (signed=%t)", playerName, signature != "")

	// Update the player's profile property with the signed MineSkin result.
	// This triggers EventSkin â†’ Java clients despawn+respawn with the new skin.
	s.pm.UpdateProfileProperty(playerID, "textures", value, signature)
}
