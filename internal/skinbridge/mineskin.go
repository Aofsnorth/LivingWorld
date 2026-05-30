package skinbridge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
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

// UploadToMineSkin uploads a PNG to MineSkin API v1 (with Bearer token if using v2 keys)
func UploadToMineSkin(pngData []byte, apiKey string) (value, signature string, err error) {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	
	// MineSkin v1 expects "file"
	fw, err := w.CreateFormFile("file", "skin.png")
	if err != nil {
		return "", "", err
	}
	_, err = fw.Write(pngData)
	if err != nil {
		return "", "", err
	}
	
	// Add variant = classic (or slim, but we assume classic for now)
	if err := w.WriteField("variant", "classic"); err != nil {
		return "", "", err
	}
	// visibility = 1 (public) or 0 (private). Use private to not spam the gallery
	if err := w.WriteField("visibility", "0"); err != nil {
		return "", "", err
	}
	w.Close()

	req, err := http.NewRequest("POST", "https://api.mineskin.org/generate/upload", &b)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("mineskin error %d: %s", resp.StatusCode, string(body))
	}

	var res MineSkinResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", "", err
	}

	return res.Data.Texture.Value, res.Data.Texture.Signature, nil
}
