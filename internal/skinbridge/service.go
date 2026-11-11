package skinbridge

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	mu    sync.RWMutex
	addr  string
	skins map[string][]byte
}

func New() *Service {
	return &Service{skins: make(map[string][]byte)}
}

func (s *Service) Start() {
	s.mu.Lock()
	if s.addr != "" {
		s.mu.Unlock()
		return
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		s.mu.Unlock()
		log.Printf("[SkinBridge] disabled: %v", err)
		return
	}
	s.addr = "http://" + ln.Addr().String()
	s.mu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("/skins/", s.handleSkin)
	go func() {
		if err := http.Serve(ln, mux); err != nil {
			log.Printf("[SkinBridge] stopped: %v", err)
		}
	}()
	log.Printf("[SkinBridge] serving Bedrock skins for Java clients at %s", s.addr)
}

func (s *Service) GetAddr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.addr
}

// RegisterSkin normalizes and compresses a Bedrock player's raw RGBA skin (64x32,
// 64x64, 128x64, 128x128, 256x256, etc.) into a Java-compatible 64x64 PNG using
// CompressBedrockSkinRGBA, registers it on the local HTTP server, and returns the
// HTTP URL, the normalized 64x64 RGBA bytes, and the encoded PNG bytes.
func (s *Service) RegisterSkin(id uuid.UUID, width, height int, rgba []byte, skinColourHex string) (skinURL string, normRGBA []byte, pngData []byte) {
	if width <= 0 || height <= 0 || len(rgba) < width*height*4 {
		return "", nil, nil
	}
	normRGBA, pngData, err := CompressBedrockSkinRGBA(rgba, width, height, skinColourHex)
	if err != nil {
		log.Printf("[SkinBridge] compress skin %s (%dx%d) failed: %v", id, width, height, err)
		return "", nil, nil
	}
	if s == nil {
		return "", normRGBA, pngData
	}
	s.mu.RLock()
	addr := s.addr
	s.mu.RUnlock()
	if addr == "" {
		return "", normRGBA, pngData
	}

	key := id.String() + ".png"
	s.mu.Lock()
	s.skins[key] = append([]byte(nil), pngData...)
	s.mu.Unlock()
	return addr + "/skins/" + key, normRGBA, pngData
}

// RegisterRGBA normalizes and registers a Bedrock skin on the local HTTP server,
// always serving a Java-compatible 64x64 PNG so Java clients never reject 128x128
// textures in SkinTextureDownloader.
func (s *Service) RegisterRGBA(id uuid.UUID, width, height int, rgba []byte) string {
	url, _, _ := s.RegisterSkin(id, width, height, rgba, "")
	return url
}

func TextureProperty(profileID uuid.UUID, profileName, skinURL string) (name, value string) {
	return TexturePropertyWithModel(profileID, profileName, skinURL, "wide")
}

func TexturePropertyWithModel(profileID uuid.UUID, profileName, skinURL, model string) (name, value string) {
	skinEntry := textureURL{URL: skinURL}
	if strings.EqualFold(model, "slim") {
		skinEntry.Metadata = &textureMetadata{Model: "slim"}
	}
	payload := texturePayload{
		Timestamp:   time.Now().UnixMilli(),
		ProfileID:   strings.ReplaceAll(profileID.String(), "-", ""),
		ProfileName: profileName,
		Textures:    textureMap{Skin: skinEntry},
	}
	b, _ := json.Marshal(payload)
	return "textures", base64.StdEncoding.EncodeToString(b)
}

type texturePayload struct {
	Timestamp   int64      `json:"timestamp"`
	ProfileID   string     `json:"profileId"`
	ProfileName string     `json:"profileName"`
	Textures    textureMap `json:"textures"`
}

type textureMap struct {
	Skin textureURL `json:"SKIN"`
}

type textureURL struct {
	URL      string           `json:"url"`
	Metadata *textureMetadata `json:"metadata,omitempty"`
}

type textureMetadata struct {
	Model string `json:"model,omitempty"`
}

func (s *Service) handleSkin(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/skins/")
	data := s.GetSkin(key)
	if len(data) == 0 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(data)
}

func (s *Service) GetSkin(key string) []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]byte(nil), s.skins[key]...)
}
