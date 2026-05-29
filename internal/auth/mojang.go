package auth

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

const (
	AuthServer    = "https://authserver.mojang.com"
	SessionServer = "https://sessionserver.mojang.com"
	ApiServer     = "https://api.mojang.com"
)

type AuthResponse struct {
	AccessToken  string `json:"accessToken"`
	ClientToken  string `json:"clientToken"`
	SelectedProfile struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"selectedProfile"`
}

type JoinRequest struct {
	AccessToken     string `json:"accessToken"`
	SelectedProfile string `json:"selectedProfile"`
	ServerID        string `json:"serverId"`
}

type HasJoinedResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Properties []struct {
		Name      string `json:"name"`
		Value     string `json:"value"`
		Signature string `json:"signature"`
	} `json:"properties"`
}

func Authenticate(username, password, clientToken string) (*AuthResponse, error) {
	type AuthReq struct {
		Agent struct {
			Name    string `json:"name"`
			Version int    `json:"version"`
		} `json:"agent"`
		Username    string `json:"username"`
		Password    string `json:"password"`
		ClientToken string `json:"clientToken"`
	}

	reqBody := AuthReq{}
	reqBody.Agent.Name = "Minecraft"
	reqBody.Agent.Version = 1
	reqBody.Username = username
	reqBody.Password = password
	reqBody.ClientToken = clientToken

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(AuthServer+"/authentication", "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("auth failed: %d", resp.StatusCode)
	}

	var authResp AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return nil, err
	}
	return &authResp, nil
}

func JoinServer(accessToken, selectedProfile, serverID string) error {
	req := JoinRequest{
		AccessToken:     accessToken,
		SelectedProfile: selectedProfile,
		ServerID:        serverID,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(SessionServer+"/session/minecraft/join", "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("join failed: %d", resp.StatusCode)
	}
	return nil
}

func HasJoined(username, serverID string) (*HasJoinedResponse, error) {
	url := fmt.Sprintf("%s/session/minecraft/hasJoined?username=%s&serverId=%s", SessionServer, username, serverID)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hasjoined failed: %d", resp.StatusCode)
	}

	var hasJoined HasJoinedResponse
	if err := json.NewDecoder(resp.Body).Decode(&hasJoined); err != nil {
		return nil, err
	}
	return &hasJoined, nil
}

func GenerateOfflineUUID(username string) uuid.UUID {
	data := []byte("OfflinePlayer:" + username)
	hash := md5.Sum(data)
	hash[6] = (hash[6] & 0x0f) | 0x30
	hash[8] = (hash[8] & 0x3f) | 0x80
	return uuid.UUID(hash)
}
