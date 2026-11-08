package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// Official Minecraft Live endpoints (Client ID: 00000000402b5328)
	liveDeviceCodeURL = "https://login.live.com/oauth20_connect.srf"
	liveTokenURL      = "https://login.live.com/oauth20_token.srf"
	liveScope         = "service::user.auth.xboxlive.com::MBI_SSL"

	// Microsoft Entra ID / Graph v2 endpoints (UUID client IDs)
	msDeviceCodeURL = "https://login.microsoftonline.com/consumers/oauth2/v2.0/devicecode"
	msTokenURL      = "https://login.microsoftonline.com/consumers/oauth2/v2.0/token"
	msScope         = "XboxLive.signin offline_access"
)

func isLiveClientID(clientID string) bool {
	return strings.HasPrefix(clientID, "0000") || len(clientID) == 16
}

var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

// RequestDeviceCode initiates Microsoft Device Authorization Grant flow.
func RequestDeviceCode(ctx context.Context, clientID string) (*DeviceCodeResponse, error) {
	if clientID == "" {
		clientID = GetClientID()
	}

	data := url.Values{}
	data.Set("client_id", clientID)

	var endpoint string
	if isLiveClientID(clientID) {
		endpoint = liveDeviceCodeURL
		data.Set("scope", liveScope)
		data.Set("response_type", "device_code")
	} else {
		endpoint = msDeviceCodeURL
		data.Set("scope", msScope)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create device code request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request device code: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read device code response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device code failed (%d): %s", resp.StatusCode, string(body))
	}

	var dcr DeviceCodeResponse
	if err := json.Unmarshal(body, &dcr); err != nil {
		return nil, fmt.Errorf("unmarshal device code: %w", err)
	}
	if dcr.Interval <= 0 {
		dcr.Interval = 5
	}
	return &dcr, nil
}

// PollError represents standard OAuth 2.0 error responses during device code polling.
type PollError struct {
	ErrorType string `json:"error"`
	ErrorDesc string `json:"error_description"`
}

func (e *PollError) Error() string {
	return fmt.Sprintf("%s: %s", e.ErrorType, e.ErrorDesc)
}

// PollDeviceToken polls Microsoft token endpoint until user approves or code expires.
// onTick is an optional callback invoked on every poll attempt (can be nil).
func PollDeviceToken(ctx context.Context, clientID, deviceCode string, interval int, onTick func()) (*TokenResponse, error) {
	if clientID == "" {
		clientID = GetClientID()
	}
	if interval <= 0 {
		interval = 5
	}

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			if onTick != nil {
				onTick()
			}

			data := url.Values{}
			data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
			data.Set("client_id", clientID)
			data.Set("device_code", deviceCode)

			var tokenURL string
			if isLiveClientID(clientID) {
				tokenURL = liveTokenURL
			} else {
				tokenURL = msTokenURL
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
			if err != nil {
				return nil, fmt.Errorf("create token request: %w", err)
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			resp, err := httpClient.Do(req)
			if err != nil {
				// Temporary network hiccup, retry next tick
				continue
			}

			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				continue
			}

			if resp.StatusCode == http.StatusOK {
				var tr TokenResponse
				if err := json.Unmarshal(body, &tr); err != nil {
					return nil, fmt.Errorf("unmarshal token: %w", err)
				}
				return &tr, nil
			}

			var pe PollError
			if err := json.Unmarshal(body, &pe); err == nil {
				switch pe.ErrorType {
				case "authorization_pending":
					// User has not entered code yet, keep waiting
					continue
				case "slow_down":
					// Server asks to increase polling interval
					interval += 5
					ticker.Reset(time.Duration(interval) * time.Second)
					continue
				case "expired_token":
					return nil, fmt.Errorf("device code expired, please try again")
				case "access_denied", "authorization_declined":
					return nil, fmt.Errorf("login was declined by user")
				default:
					return nil, &pe
				}
			}

			return nil, fmt.Errorf("token request returned status %d: %s", resp.StatusCode, string(body))
		}
	}
}

// RefreshMicrosoftToken exchanges a refresh_token for a new access_token and refresh_token.
func RefreshMicrosoftToken(ctx context.Context, clientID, refreshToken string) (*TokenResponse, error) {
	if clientID == "" {
		clientID = GetClientID()
	}

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("client_id", clientID)
	data.Set("refresh_token", refreshToken)

	var tokenURL string
	if isLiveClientID(clientID) {
		tokenURL = liveTokenURL
		data.Set("scope", liveScope)
	} else {
		tokenURL = msTokenURL
		data.Set("scope", msScope)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh failed (%d): %s", resp.StatusCode, string(body))
	}

	var tr TokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("unmarshal refresh token: %w", err)
	}
	return &tr, nil
}
