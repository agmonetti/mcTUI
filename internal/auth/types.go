package auth

import (
	"time"
)

// AccountType identifies whether an account is authenticated via Microsoft or is a local offline profile.
type AccountType string

const (
	// AccountTypeMicrosoft represents an account authenticated via Microsoft OAuth and Mojang services.
	AccountTypeMicrosoft AccountType = "microsoft"

	// AccountTypeOffline represents an unauthenticated offline player profile.
	AccountTypeOffline AccountType = "offline"
)

// Account represents a player profile (either Microsoft or Offline).
type Account struct {
	ID           string      `json:"id"`                      // Minecraft UUID
	Username     string      `json:"username"`                // In-game player name
	Type         AccountType `json:"type"`                    // "microsoft" or "offline"
	AccessToken  string      `json:"access_token,omitempty"`  // Minecraft Bearer Token (MSA only)
	RefreshToken string      `json:"refresh_token,omitempty"` // Microsoft OAuth Refresh Token (MSA only)
	ExpiresAt    time.Time   `json:"expires_at,omitempty"`    // Access token expiration time
	CreatedAt    time.Time   `json:"created_at"`
}

// IsExpired returns true if the account is a Microsoft account whose token has expired or is about to expire in 5 minutes.
func (a *Account) IsExpired() bool {
	if a.Type != AccountTypeMicrosoft {
		return false
	}
	if a.AccessToken == "" || a.ExpiresAt.IsZero() {
		return true
	}
	return time.Now().After(a.ExpiresAt.Add(-5 * time.Minute))
}

// Storage represents the persisted accounts file structure.
type Storage struct {
	ActiveAccountID string    `json:"active_account_id"`
	Accounts        []Account `json:"accounts"`
}

// DeviceCodeResponse holds the payload returned by Microsoft's device code endpoint.
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
	Message         string `json:"message"`
}

// TokenResponse represents Microsoft's OAuth token endpoint response.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	RefreshToken string `json:"refresh_token"`
	UserID       string `json:"user_id,omitempty"`
}

// MinecraftProfile represents the profile response from api.minecraftservices.com.
type MinecraftProfile struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Skins []struct {
		ID    string `json:"id"`
		State string `json:"state"`
		URL   string `json:"url"`
	} `json:"skins,omitempty"`
}
