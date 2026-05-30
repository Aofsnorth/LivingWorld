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
	} else if width > 64 || height > 64 {
		img = image.NewNRGBA(image.Rect(0, 0, 64, 64))
		scaleX := width / 64
		scaleY := height / 64
		if scaleX == 0 { scaleX = 1 }
		if scaleY == 0 { scaleY = 1 }
		for y := 0; y < 64; y++ {
			for x := 0; x < 64; x++ {
				// Use nearest-neighbor sampling to preserve pixel-art sharpness.
				// Averaging pixels makes the skin look blurry/compressed.
				sx := x * scaleX
				sy := y * scaleY
				
				var r, g, b, a byte
				if sx < width && sy < height {
					srcIdx := (sy*width + sx) * 4
					r = rgba[srcIdx]
					g = rgba[srcIdx+1]
					b = rgba[srcIdx+2]
					a = rgba[srcIdx+3]
				}

				dstIdx := (y*64 + x) * 4
				img.Pix[dstIdx] = r
				img.Pix[dstIdx+1] = g
				img.Pix[dstIdx+2] = b
				img.Pix[dstIdx+3] = a
			}
		}
	} else {
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
