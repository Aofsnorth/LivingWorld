package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

var (
	ErrInvalidToken = errors.New("invalid xbox token")
	ErrAuthFailed   = errors.New("xbox auth failed")
)

type XSTSResponse struct {
	Token        string `json:"Token"`
	DisplayClaims struct {
		XUI []struct {
			USERHASH string `json:"uhs"`
			XID      string `json:"xid"`
		} `json:"xui"`
	} `json:"DisplayClaims"`
}

type ProfileInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Skins   []Skin `json:"skins"`
	Capes   []Cape `json:"capes"`
	Premium bool   `json:"premium"`
	Legacy  bool   `json:"legacy"`
	Demo    bool   `json:"demo"`
}

type Skin struct {
	ID      string `json:"id"`
	State   string `json:"state"`
	URL     string `json:"url"`
	Variant string `json:"variant"`
}

type Cape struct {
	ID    string `json:"id"`
	State string `json:"state"`
	URL   string `json:"url"`
}

func GetMinecraftToken(xstsToken, userHash string) (string, error) {
	data := map[string]string{
		"identityToken":         fmt.Sprintf("XBL3.0 x=%s;%s", userHash, xstsToken),
		"ensureLegacyEnabled":   "true",
	}

	jsonData, _ := json.Marshal(data)
	resp, err := http.Post(
		"https://api.minecraftservices.com/authentication/login_with_xbox",
		"application/json",
		strings.NewReader(string(jsonData)),
	)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", ErrAuthFailed
	}

	var mcResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&mcResp); err != nil {
		return "", err
	}

	return mcResp.AccessToken, nil
}

func ValidateXToken(token string) (*ProfileInfo, error) {
	req, _ := http.NewRequest("GET", "https://api.minecraftservices.com/minecraft/profile", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 204 || resp.StatusCode == 404 {
		return nil, ErrInvalidToken
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token validation failed: %d", resp.StatusCode)
	}

	var profile ProfileInfo
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, err
	}

	return &profile, nil
}

func XUIDToUUID(xuid uint64) string {
	b := make([]byte, 16)
	rand.Read(b)
	b[0] = 0xFF
	b[1] = 0xFF
	for i := 0; i < 8; i++ {
		b[12+i] = byte((xuid >> (56 - i*8)) & 0xFF)
	}
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		base64.RawURLEncoding.EncodeToString(b[0:4]),
		base64.RawURLEncoding.EncodeToString(b[4:6]),
		base64.RawURLEncoding.EncodeToString(b[6:8]),
		base64.RawURLEncoding.EncodeToString(b[8:12]),
		base64.RawURLEncoding.EncodeToString(b[12:16]),
	)
}
