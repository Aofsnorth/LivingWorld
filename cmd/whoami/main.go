// Command whoami memanggil Anthropic Messages API dan mencetak field `model`
// dari response. Field ini dikembalikan oleh server Anthropic — bukan teks yang
// diketik oleh model — jadi inilah bukti model mana yang benar-benar menjawab.
//
// Jalankan:
//
//	set ANTHROPIC_API_KEY=sk-ant-...   (Windows cmd)
//	$env:ANTHROPIC_API_KEY="sk-ant-..." (PowerShell)
//	go run ./cmd/whoami
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	apiURL        = "https://api.anthropic.com/v1/messages"
	apiVersion    = "2023-06-01"
	requestedModel = "claude-opus-4-8" // ganti sesuai model yang ingin kamu uji
)

type request struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	Messages  []message `json:"messages"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// response hanya mengambil field yang kita pedulikan; sisanya diabaikan.
type response struct {
	ID    string `json:"id"`
	Model string `json:"model"` // <- sumber kebenaran: diisi oleh server
	Role  string `json:"role"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func main() {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Fatal("ANTHROPIC_API_KEY belum di-set")
	}

	reqBody, err := json.Marshal(request{
		Model:     requestedModel,
		MaxTokens: 16,
		Messages:  []message{{Role: "user", Content: "ping"}},
	})
	if err != nil {
		log.Fatalf("marshal request: %v", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(reqBody))
	if err != nil {
		log.Fatalf("build request: %v", err)
	}
	httpReq.Header.Set("x-api-key", apiKey)
	httpReq.Header.Set("anthropic-version", apiVersion)
	httpReq.Header.Set("content-type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Fatalf("call API: %v", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		log.Fatalf("API error (HTTP %d): %s", resp.StatusCode, raw)
	}

	var parsed response
	if err := json.Unmarshal(raw, &parsed); err != nil {
		log.Fatalf("parse response: %v", err)
	}

	fmt.Println("=== Bukti dari server Anthropic ===")
	fmt.Printf("Model diminta : %s\n", requestedModel)
	fmt.Printf("Model dijawab : %s   <- field response.model dari server\n", parsed.Model)
	fmt.Printf("Response ID   : %s\n", parsed.ID)
	fmt.Printf("Token usage   : in=%d out=%d\n", parsed.Usage.InputTokens, parsed.Usage.OutputTokens)
}
