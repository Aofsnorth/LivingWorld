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

func (s *Service) GetAddr() string {
	return s.addr
}

func (s *Service) RegisterRGBA(id uuid.UUID, width, height int, rgba []byte) string {
	if s == nil || s.addr == "" || width <= 0 || height <= 0 || len(rgba) < width*height*4 {
		return ""
	}
	var img *image.NRGBA

	isPadded64 := false
	if width == 128 && height == 128 {
		isPadded64 = true
		for y := 0; y < 128; y++ {
			for x := 0; x < 128; x++ {
				if x >= 64 || y >= 64 {
					idx := (y*128 + x) * 4
					if rgba[idx+3] != 0 {
						isPadded64 = false
						break
					}
				}
			}
			if !isPadded64 {
				break
			}
		}
	}

	if isPadded64 {
		// A 64×64 skin the Bedrock client padded into a 128 canvas: crop back to
		// the real 64×64 content (lossless).
		img = image.NewNRGBA(image.Rect(0, 0, 64, 64))
		for y := 0; y < 64; y++ {
			for x := 0; x < 64; x++ {
				srcIdx := (y*128 + x) * 4
				dstIdx := (y*64 + x) * 4
				img.Pix[dstIdx] = rgba[srcIdx]
				img.Pix[dstIdx+1] = rgba[srcIdx+1]
				img.Pix[dstIdx+2] = rgba[srcIdx+2]
				img.Pix[dstIdx+3] = rgba[srcIdx+3]
			}
		}
	} else {
		// Pass the skin through at its FULL native resolution. Java supports HD
		// (128×128) skins natively, so a genuine HD Bedrock skin must NOT be
		// downscaled to 64×64 — decimating it threw away 3 of every 4 pixels and
		// produced the "burik"/blocky look. PNG is lossless, so this preserves the
		// skin exactly. Valid Minecraft skins are 64×64, legacy 64×32, or HD
		// 128×128; any of these copy through unchanged.
		img = image.NewNRGBA(image.Rect(0, 0, width, height))
		copy(img.Pix, rgba[:width*height*4])
	}
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
		Timestamp:   time.Now().UnixMilli(),
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
