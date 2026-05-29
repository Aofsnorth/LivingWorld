package skinbridge

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"

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
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Printf("[SkinBridge] disabled: %v", err)
		return
	}
	s.addr = "http://" + ln.Addr().String()
	mux := http.NewServeMux()
	mux.HandleFunc("/skins/", s.handleSkin)
	go func() {
		if err := http.Serve(ln, mux); err != nil {
			log.Printf("[SkinBridge] stopped: %v", err)
		}
	}()
	log.Printf("[SkinBridge] serving Bedrock skins for Java clients at %s", s.addr)
}

func (s *Service) RegisterRGBA(id uuid.UUID, width, height int, rgba []byte) string {
	if s == nil || s.addr == "" || width <= 0 || height <= 0 || len(rgba) < width*height*4 {
		return ""
	}
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	copy(img.Pix, rgba[:width*height*4])
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		log.Printf("[SkinBridge] encode skin %s failed: %v", id, err)
		return ""
	}
	key := id.String() + ".png"
	s.mu.Lock()
	s.skins[key] = append([]byte(nil), buf.Bytes()...)
	s.mu.Unlock()
	return s.addr + "/skins/" + key
}

func TextureProperty(profileID uuid.UUID, profileName, skinURL string) (name, value string) {
	payload := texturePayload{
		Timestamp:   0,
		ProfileID:   strings.ReplaceAll(profileID.String(), "-", ""),
		ProfileName: profileName,
		Textures:    textureMap{Skin: textureURL{URL: skinURL}},
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
	URL string `json:"url"`
}

func (s *Service) handleSkin(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/skins/")
	s.mu.RLock()
	data := append([]byte(nil), s.skins[key]...)
	s.mu.RUnlock()
	if len(data) == 0 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(data)
}
