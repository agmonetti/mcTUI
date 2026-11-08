package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCompleteDeviceFlowWithMockServer(t *testing.T) {
	// Mock server that responds to Xbox, XSTS, Minecraft Login, and Minecraft Profile
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/user/authenticate":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"Token": "mock_xbl_token",
				"DisplayClaims": map[string]interface{}{
					"xui": []map[string]string{{"uhs": "mock_user_hash"}},
				},
			})
		case "/xsts/authorize":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"Token": "mock_xsts_token",
				"DisplayClaims": map[string]interface{}{
					"xui": []map[string]string{{"uhs": "mock_user_hash"}},
				},
			})
		case "/authentication/login_with_xbox":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"username":     "TestPlayer",
				"access_token": "mock_minecraft_token",
				"token_type":   "Bearer",
				"expires_in":   86400,
			})
		case "/minecraft/profile":
			if r.Header.Get("Authorization") != "Bearer mock_minecraft_token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":   "12345678123412341234123456789abc",
				"name": "TestPlayer",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Redirect endpoints to mock server for testing
	// Save original client
	origClient := httpClient
	defer func() { httpClient = origClient }()
	httpClient = server.Client()

	// Test Xbox Auth directly against mock
	xblResp, err := func() (*XboxAuthResponse, error) {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL+"/user/authenticate", nil)
		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		var res XboxAuthResponse
		_ = json.NewDecoder(resp.Body).Decode(&res)
		return &res, nil
	}()
	if err != nil || xblResp.Token != "mock_xbl_token" || xblResp.UserHash() != "mock_user_hash" {
		t.Fatalf("expected valid xbox auth, got %+v (err: %v)", xblResp, err)
	}

	// Test Profile Fetch
	prof, err := func() (*MinecraftProfile, error) {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/minecraft/profile", nil)
		req.Header.Set("Authorization", "Bearer mock_minecraft_token")
		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		var res MinecraftProfile
		_ = json.NewDecoder(resp.Body).Decode(&res)
		return &res, nil
	}()
	if err != nil || prof.Name != "TestPlayer" {
		t.Fatalf("expected profile TestPlayer, got %+v (err: %v)", prof, err)
	}
}

func TestEnsureValidAccount(t *testing.T) {
	ctx := context.Background()

	// Offline account should always be valid immediately
	offlineAcc := &Account{
		ID:       "123",
		Username: "OfflineGuy",
		Type:     AccountTypeOffline,
	}
	if err := EnsureValidAccount(ctx, offlineAcc); err != nil {
		t.Fatalf("offline account should be valid without refresh: %v", err)
	}

	// Unexpired Microsoft account should be valid immediately
	validMsa := &Account{
		ID:          "456",
		Username:    "OnlineGuy",
		Type:        AccountTypeMicrosoft,
		AccessToken: "valid_token",
		ExpiresAt:   time.Now().Add(2 * time.Hour),
	}
	if err := EnsureValidAccount(ctx, validMsa); err != nil {
		t.Fatalf("unexpired MSA account should be valid: %v", err)
	}

	// Expired Microsoft account with no refresh token should return error
	expiredMsa := &Account{
		ID:          "789",
		Username:    "ExpiredGuy",
		Type:        AccountTypeMicrosoft,
		AccessToken: "old_token",
		ExpiresAt:   time.Now().Add(-10 * time.Minute),
	}
	if err := EnsureValidAccount(ctx, expiredMsa); err == nil {
		t.Fatalf("expected error when refreshing account with no refresh token")
	}
}

func TestIsLiveClientID(t *testing.T) {
	if !isLiveClientID("00000000402b5328") {
		t.Errorf("expected 00000000402b5328 to be recognized as Live client ID")
	}
	if !isLiveClientID("00000000441cc96b") {
		t.Errorf("expected 00000000441cc96b to be recognized as Live client ID")
	}
	if isLiveClientID("c36a9fb6-4f2a-41ff-90bd-ae7cc92031eb") {
		t.Errorf("expected Prism UUID to not be recognized as Live client ID")
	}
}
