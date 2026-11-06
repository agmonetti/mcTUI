package auth

import (
	"context"
	"fmt"
	"time"
)

// CompleteDeviceFlow processes the authentication pipeline starting from Microsoft TokenResponse
// all the way to obtaining Minecraft tokens and profile, returning a ready-to-use Account.
func CompleteDeviceFlow(ctx context.Context, msTokens *TokenResponse) (*Account, error) {
	if msTokens == nil || msTokens.AccessToken == "" {
		return nil, fmt.Errorf("invalid microsoft tokens")
	}

	// 1. Xbox Live User Auth
	xblResp, err := AuthenticateXbox(ctx, msTokens.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("xbox auth: %w", err)
	}

	// 2. XSTS Auth
	xstsResp, err := AuthorizeXSTS(ctx, xblResp.Token)
	if err != nil {
		return nil, fmt.Errorf("xsts auth: %w", err)
	}

	// 3. Minecraft Service Login
	mcToken, expiresIn, err := LoginWithXbox(ctx, xstsResp.UserHash(), xstsResp.Token)
	if err != nil {
		return nil, fmt.Errorf("minecraft login: %w", err)
	}

	// 4. Fetch Minecraft Profile
	prof, err := FetchProfile(ctx, mcToken)
	if err != nil {
		return nil, fmt.Errorf("minecraft profile: %w", err)
	}

	acc := &Account{
		ID:           FormatUUID(prof.ID),
		Username:     prof.Name,
		Type:         AccountTypeMicrosoft,
		AccessToken:  mcToken,
		RefreshToken: msTokens.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(expiresIn) * time.Second),
		CreatedAt:    time.Now(),
	}
	return acc, nil
}

// RefreshAccount renews tokens for an existing Microsoft account using its RefreshToken.
func RefreshAccount(ctx context.Context, acc *Account) error {
	if acc.Type != AccountTypeMicrosoft {
		return nil
	}
	if acc.RefreshToken == "" {
		return fmt.Errorf("no refresh token available for account %s", acc.Username)
	}

	msTokens, err := RefreshMicrosoftToken(ctx, GetClientID(), acc.RefreshToken)
	if err != nil {
		return fmt.Errorf("microsoft refresh: %w", err)
	}

	// Exchange new MS token through Xbox -> XSTS -> Minecraft
	xblResp, err := AuthenticateXbox(ctx, msTokens.AccessToken)
	if err != nil {
		return fmt.Errorf("xbox refresh: %w", err)
	}

	xstsResp, err := AuthorizeXSTS(ctx, xblResp.Token)
	if err != nil {
		return fmt.Errorf("xsts refresh: %w", err)
	}

	mcToken, expiresIn, err := LoginWithXbox(ctx, xstsResp.UserHash(), xstsResp.Token)
	if err != nil {
		return fmt.Errorf("minecraft login refresh: %w", err)
	}

	acc.AccessToken = mcToken
	acc.RefreshToken = msTokens.RefreshToken
	acc.ExpiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second)
	return nil
}

// EnsureValidAccount ensures that the given account has a valid, unexpired token.
// If the account is Offline, it returns nil immediately.
// If the token is still valid, it returns nil immediately.
// If expired, it attempts a silent refresh.
func EnsureValidAccount(ctx context.Context, acc *Account) error {
	if acc == nil {
		return fmt.Errorf("nil account")
	}
	if acc.Type == AccountTypeOffline {
		return nil
	}
	if !acc.IsExpired() {
		return nil
	}
	return RefreshAccount(ctx, acc)
}
