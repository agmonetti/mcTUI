package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

const (
	minecraftLoginURL   = "https://api.minecraftservices.com/authentication/login_with_xbox"
	minecraftProfileURL = "https://api.minecraftservices.com/minecraft/profile"
)

// ErrMinecraftNotOwned indicates the Microsoft account does not have a registered Minecraft profile/license.
var ErrMinecraftNotOwned = errors.New("this Microsoft account does not own Minecraft Java Edition or has not created a profile yet")

type mcLoginRequest struct {
	IdentityToken string `json:"identityToken"`
}

type mcLoginResponse struct {
	Username    string `json:"username"`
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// LoginWithXbox exchanges XSTS token for Minecraft services bearer token.
func LoginWithXbox(ctx context.Context, userHash, xstsToken string) (string, int, error) {
	reqBody := mcLoginRequest{
		IdentityToken: fmt.Sprintf("XBL3.0 x=%s;%s", userHash, xstsToken),
	}

	raw, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, fmt.Errorf("marshal minecraft login: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, minecraftLoginURL, bytes.NewReader(raw))
	if err != nil {
		return "", 0, fmt.Errorf("create minecraft login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("minecraft login request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("read minecraft login response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("minecraft login failed (%d): %s", resp.StatusCode, string(body))
	}

	var mlr mcLoginResponse
	if err := json.Unmarshal(body, &mlr); err != nil {
		return "", 0, fmt.Errorf("unmarshal minecraft login: %w", err)
	}
	if mlr.AccessToken == "" {
		return "", 0, fmt.Errorf("minecraft login response missing access token")
	}
	if mlr.ExpiresIn <= 0 {
		mlr.ExpiresIn = 86400
	}
	return mlr.AccessToken, mlr.ExpiresIn, nil
}

// FetchProfile retrieves Minecraft profile (ID and Name) for a given Bearer access token.
func FetchProfile(ctx context.Context, mcAccessToken string) (*MinecraftProfile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, minecraftProfileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create profile request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+mcAccessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("profile request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read profile response: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrMinecraftNotOwned
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch profile failed (%d): %s", resp.StatusCode, string(body))
	}

	var prof MinecraftProfile
	if err := json.Unmarshal(body, &prof); err != nil {
		return nil, fmt.Errorf("unmarshal profile: %w", err)
	}
	if prof.ID == "" || prof.Name == "" {
		return nil, fmt.Errorf("profile response missing ID or Name")
	}
	return &prof, nil
}

// FormatUUID ensures standard 8-4-4-4-12 UUID format with hyphens.
func FormatUUID(raw string) string {
	cleaned := ""
	for _, r := range raw {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			cleaned += string(r)
		}
	}
	if len(cleaned) == 32 {
		return fmt.Sprintf("%s-%s-%s-%s-%s",
			cleaned[0:8], cleaned[8:12], cleaned[12:16], cleaned[16:20], cleaned[20:32])
	}
	return raw
}
