package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		v1, v2 string
		want   int
	}{
		{"v2.0.0", "v2.0.0", 0},
		{"2.0.0", "v2.0.0", 0},
		{"v2.1.0", "v2.0.0", 1},
		{"v2.0.0", "v2.1.0", -1},
		{"v2.0.1", "v2.0.0", 1},
		{"v1.9.9", "v2.0.0", -1},
		{"v2.1", "v2.0.5", 1},
		{"v2.0.0", "v2.0.0-rc.1", 1},
		{"v2.0.0-rc.1", "v2.0.0", -1},
		{"v2.0.0-rc.2", "v2.0.0-rc.1", 1},
		{"dev", "v2.0.0", 0},
		{"v2.0.0", "dev", 0},
	}

	for _, tt := range tests {
		got := CompareSemver(tt.v1, tt.v2)
		if got != tt.want {
			t.Errorf("CompareSemver(%q, %q) = %d, want %d", tt.v1, tt.v2, got, tt.want)
		}
	}
}

func TestIsNewer(t *testing.T) {
	tests := []struct {
		latest, current string
		want            bool
	}{
		{"v2.1.0", "v2.0.0", true},
		{"v2.0.1", "v2.0.0", true},
		{"v3.0.0", "v2.5.0", true},
		{"v2.0.0", "v2.0.0", false},
		{"v1.9.0", "v2.0.0", false},
		{"v2.1.0", "dev", false},
		{"dev", "v2.0.0", false},
	}

	for _, tt := range tests {
		got := IsNewer(tt.latest, tt.current)
		if got != tt.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
		}
	}
}

func TestParseChecksums(t *testing.T) {
	input := `
# Comment line
e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855  mctui-linux-amd64
a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3 *mctui-windows-amd64.exe
`
	checksums := ParseChecksums(input)
	if len(checksums) != 2 {
		t.Fatalf("expected 2 checksum entries, got %d", len(checksums))
	}
	if checksums["mctui-linux-amd64"] != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Errorf("unexpected linux checksum: %s", checksums["mctui-linux-amd64"])
	}
	if checksums["mctui-windows-amd64.exe"] != "a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3" {
		t.Errorf("unexpected windows checksum: %s", checksums["mctui-windows-amd64.exe"])
	}
}

func TestVerifySHA256(t *testing.T) {
	tmpDir := t.TempDir()
	sampleFile := filepath.Join(tmpDir, "test.bin")
	content := []byte("Hello, mcTUI world!")
	if err := os.WriteFile(sampleFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	hasher := sha256.New()
	hasher.Write(content)
	expectedHex := hex.EncodeToString(hasher.Sum(nil))

	if err := VerifySHA256(sampleFile, expectedHex); err != nil {
		t.Errorf("expected valid SHA-256 check, got: %v", err)
	}

	badHex := "0000000000000000000000000000000000000000000000000000000000000000"
	if err := VerifySHA256(sampleFile, badHex); err == nil {
		t.Errorf("expected error on mismatched SHA-256, got nil")
	}
}

func TestTargetAssetName(t *testing.T) {
	if got := TargetAssetName("linux", "amd64"); got != "mctui-linux-amd64" {
		t.Errorf("got %s, want mctui-linux-amd64", got)
	}
	if got := TargetAssetName("windows", "amd64"); got != "mctui-windows-amd64.exe" {
		t.Errorf("got %s, want mctui-windows-amd64.exe", got)
	}
	if got := TargetAssetName("darwin", "arm64"); got != "mctui-darwin-arm64" {
		t.Errorf("got %s, want mctui-darwin-arm64", got)
	}
}

func TestCheckWithURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"tag_name": "v2.1.0",
			"html_url": "https://github.com/agmonetti/mcTUI/releases/tag/v2.1.0",
			"body": "Fixes and new features",
			"assets": [
				{"name": "checksums.txt", "size": 100, "browser_download_url": "https://example.com/checksums.txt"},
				{"name": "mctui-linux-amd64", "size": 5000000, "browser_download_url": "https://example.com/binary"}
			]
		}`))
	}))
	defer server.Close()

	// Current version is v2.0.0, latest is v2.1.0 -> should find update
	rel, ok := CheckWithURL(server.URL, "v2.0.0")
	if !ok {
		t.Fatalf("expected update available from v2.0.0 to v2.1.0")
	}
	if rel.TagName != "v2.1.0" {
		t.Errorf("expected tag v2.1.0, got %s", rel.TagName)
	}
	if len(rel.Assets) != 2 {
		t.Errorf("expected 2 assets, got %d", len(rel.Assets))
	}

	// Current version is v2.1.0 -> no update
	_, ok = CheckWithURL(server.URL, "v2.1.0")
	if ok {
		t.Fatalf("expected ok=false when version is already v2.1.0")
	}

	// Current version is dev -> no update
	_, ok = CheckWithURL(server.URL, "dev")
	if ok {
		t.Fatalf("expected ok=false for dev build")
	}
}

func TestApplyUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	origBin := filepath.Join(tmpDir, "mctui")
	newBin := filepath.Join(tmpDir, "mctui.tmp")

	if err := os.WriteFile(origBin, []byte("v1"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newBin, []byte("v2"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := ApplyUpdate(newBin, origBin); err != nil {
		t.Fatalf("ApplyUpdate failed: %v", err)
	}

	content, err := os.ReadFile(origBin)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "v2" {
		t.Errorf("expected content 'v2', got %s", string(content))
	}
}

func TestCheckWithURLStatusPublicAPISucceedsWithoutFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v2.1.0","assets":[]}`))
	}))
	defer server.Close()

	original := ghLatestReleaseJSON
	t.Cleanup(func() { ghLatestReleaseJSON = original })
	called := 0
	ghLatestReleaseJSON = func(string) ([]byte, error) {
		called++
		return nil, errors.New("fallback must not run")
	}

	result := CheckWithURLStatus(server.URL, "v2.0.0")
	if !result.Available || result.Err != nil {
		t.Fatalf("result = %+v, want an available update", result)
	}
	if called != 0 {
		t.Errorf("gh fallback called %d times on a successful public check", called)
	}
}

func TestCheckWithURLStatusRateLimitUsesGHFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	defer server.Close()

	original := ghLatestReleaseJSON
	t.Cleanup(func() { ghLatestReleaseJSON = original })
	ghLatestReleaseJSON = func(string) ([]byte, error) {
		return []byte(`{"tag_name":"v2.1.0","assets":[` +
			`{"name":"checksums.txt","browser_download_url":"https://example.com/checksums.txt"},` +
			`{"name":"mctui-linux-amd64","browser_download_url":"https://example.com/mctui"}]}`), nil
	}

	result := CheckWithURLStatus(server.URL, "v2.0.0")
	if !result.Available || result.Err != nil {
		t.Fatalf("result = %+v, want an available update from the fallback", result)
	}
	if result.Release.TagName != "v2.1.0" || len(result.Release.Assets) != 2 {
		t.Errorf("fallback metadata lost release details: %+v", result.Release)
	}
}

func TestCheckWithURLStatusRateLimitWithoutFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	original := ghLatestReleaseJSON
	t.Cleanup(func() { ghLatestReleaseJSON = original })
	ghLatestReleaseJSON = func(string) ([]byte, error) {
		return nil, errors.New("gh not found")
	}

	result := CheckWithURLStatus(server.URL, "v2.0.0")
	if !errors.Is(result.Err, ErrRateLimited) {
		t.Fatalf("Err = %v, want ErrRateLimited", result.Err)
	}
	if result.Available {
		t.Error("rate-limited check must not report an available update")
	}
}

func TestCheckWithURLKeepsSilentContractForOtherErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	if _, ok := CheckWithURL(server.URL, "v2.0.0"); ok {
		t.Error("CheckWithURL must stay silent/no-update on non-rate-limit errors")
	}
}

func TestGHAPIPath(t *testing.T) {
	got, err := ghAPIPath(defaultAPIURL)
	if err != nil {
		t.Fatal(err)
	}
	if want := "repos/agmonetti/mcTUI/releases/latest"; got != want {
		t.Errorf("ghAPIPath() = %q, want %q", got, want)
	}

	for _, raw := range []string{
		"http://api.github.com/repos/o/r/releases/latest",
		"https://example.com/repos/o/r/releases/latest",
		"https://api.github.com/repos/o/r/tags",
	} {
		if _, err := ghAPIPath(raw); err == nil {
			t.Errorf("ghAPIPath(%q) expected error", raw)
		}
	}
}
