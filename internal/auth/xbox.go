package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	xboxUserAuthURL = "https://user.auth.xboxlive.com/user/authenticate"
	xstsAuthURL     = "https://xsts.auth.xboxlive.com/xsts/authorize"
	minecraftParty  = "rp://api.minecraftservices.com/"
)

type xboxAuthRequest struct {
	Properties   xboxAuthProps `json:"Properties"`
	RelyingParty string        `json:"RelyingParty"`
	TokenType    string        `json:"TokenType"`
}

type xboxAuthProps struct {
	AuthMethod string `json:"AuthMethod"`
	SiteName   string `json:"SiteName"`
	RpsTicket  string `json:"RpsTicket"`
}

type xstsAuthRequest struct {
	Properties   xstsAuthProps `json:"Properties"`
	RelyingParty string        `json:"RelyingParty"`
	TokenType    string        `json:"TokenType"`
}

type xstsAuthProps struct {
	SandboxId  string   `json:"SandboxId"`
	UserTokens []string `json:"UserTokens"`
}

// XboxAuthResponse represents token payload from Xbox Live.
type XboxAuthResponse struct {
	IssueInstant  string `json:"IssueInstant"`
	NotAfter      string `json:"NotAfter"`
	Token         string `json:"Token"`
	DisplayClaims struct {
		Xui []struct {
			Uhs string `json:"uhs"`
		} `json:"xui"`
	} `json:"DisplayClaims"`
}

// UserHash returns the user hash (uhs) from the display claims, or empty if none.
func (r *XboxAuthResponse) UserHash() string {
	if len(r.DisplayClaims.Xui) > 0 {
		return r.DisplayClaims.Xui[0].Uhs
	}
	return ""
}

// AuthenticateXbox exchanges Microsoft access token for Xbox Live User Token.
func AuthenticateXbox(ctx context.Context, msAccessToken string) (*XboxAuthResponse, error) {
	reqBody := xboxAuthRequest{
		Properties: xboxAuthProps{
			AuthMethod: "RPS",
			SiteName:   "user.auth.xboxlive.com",
			RpsTicket:  "d=" + msAccessToken,
		},
		RelyingParty: "http://auth.xboxlive.com",
		TokenType:    "JWT",
	}

	raw, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal xbox request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, xboxUserAuthURL, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("create xbox request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("xbox user auth request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read xbox response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("xbox authentication failed (%d): %s", resp.StatusCode, string(body))
	}

	var xar XboxAuthResponse
	if err := json.Unmarshal(body, &xar); err != nil {
		return nil, fmt.Errorf("unmarshal xbox response: %w", err)
	}
	if xar.Token == "" || xar.UserHash() == "" {
		return nil, fmt.Errorf("xbox response missing token or user hash")
	}
	return &xar, nil
}

// AuthorizeXSTS exchanges Xbox Live user token for an XSTS token scoped for Minecraft services.
func AuthorizeXSTS(ctx context.Context, xblToken string) (*XboxAuthResponse, error) {
	reqBody := xstsAuthRequest{
		Properties: xstsAuthProps{
			SandboxId:  "RETAIL",
			UserTokens: []string{xblToken},
		},
		RelyingParty: minecraftParty,
		TokenType:    "JWT",
	}

	raw, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal xsts request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, xstsAuthURL, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("create xsts request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("xsts auth request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read xsts response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Parse known XSTS error codes
		var errResp struct {
			XErr int64  `json:"XErr"`
			Msg  string `json:"Message"`
		}
		if json.Unmarshal(body, &errResp) == nil && errResp.XErr != 0 {
			switch errResp.XErr {
			case 2148916233:
				return nil, fmt.Errorf("microsoft account has no Xbox account (sign in at xbox.com first)")
			case 2148916235:
				return nil, fmt.Errorf("Xbox Live is not available in your country/region")
			case 2148916238:
				return nil, fmt.Errorf("child account needs to be added to a Family by an adult")
			}
		}
		return nil, fmt.Errorf("xsts authorization failed (%d): %s", resp.StatusCode, string(body))
	}

	var xar XboxAuthResponse
	if err := json.Unmarshal(body, &xar); err != nil {
		return nil, fmt.Errorf("unmarshal xsts response: %w", err)
	}
	if xar.Token == "" || xar.UserHash() == "" {
		return nil, fmt.Errorf("xsts response missing token or user hash")
	}
	return &xar, nil
}
