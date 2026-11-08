package modpacks

import (
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// The only local-server exception is test-only: its exact HTTPS authority is
// explicitly allowed and its TLS connection uses httptest's trusted transport.
func localFetch(t *testing.T, server *httptest.Server, urls []string, hashes Hashes, size, limit int64, dir string, ctx context.Context) (string, error) {
	t.Helper()
	host := strings.TrimPrefix(server.URL, "https://")
	allow := func(u *url.URL, _ string) error {
		if u == nil || u.Scheme != "https" || u.Host != host || u.User != nil {
			return errors.New("unsafe local test URL")
		}
		return nil
	}
	return fetchWithPolicy(ctx, server.Client(), urls, hashes, size, limit, dir, allow, server.Client().Transport)
}

func testHashes(data []byte) Hashes {
	one := sha1.Sum(data)
	fiveTwelve := sha512.Sum512(data)
	return Hashes{SHA1: hex.EncodeToString(one[:]), SHA512: hex.EncodeToString(fiveTwelve[:])}
}

func tempFiles(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, ".mctui-download-*"))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestAllowedURL(t *testing.T) {
	for _, host := range []string{"cdn.modrinth.net", "cdn.modrinth.com", "github.com", "raw.githubusercontent.com", "gitlab.com"} {
		if err := AllowedURL("https://" + host + "/artifact?private=token"); err != nil {
			t.Errorf("%s rejected: %v", host, err)
		}
	}
	for _, raw := range []string{
		"http://github.com/file", "https://github.com.evil.test/file", "https://evilcdn.modrinth.net/file",
		"https://127.0.0.1/file", "https://localhost/file", "https://[::1]/file",
		"https://github.com:443/file", "https://user:pass@github.com/file",
		"https://release-assets.githubusercontent.com/file", "https://github.com./file",
		"https://github.com:/file", "https://GITHUB.COM/file",
	} {
		if err := AllowedURL(raw); err == nil || strings.Contains(err.Error(), raw) {
			t.Errorf("unsafe URL accepted or leaked: %q, %v", raw, err)
		}
	}
	if err := allowedFetchRedirect(mustURL(t, "https://release-assets.githubusercontent.com/file"), "github.com"); err != nil {
		t.Fatalf("legitimate GitHub asset redirect rejected: %v", err)
	}
	if err := allowedFetchRedirect(mustURL(t, "https://release-assets.githubusercontent.com/file"), "gitlab.com"); err == nil {
		t.Fatal("unrelated site can redirect to GitHub assets")
	}
	for _, ip := range []string{"127.0.0.1", "169.254.169.254", "10.0.0.1", "100.64.0.1", "192.0.2.1", "::1", "fc00::1", "::ffff:127.0.0.1"} {
		if publicFetchIP(mustIP(t, ip)) {
			t.Errorf("local or reserved IP allowed: %s", ip)
		}
	}
	if !publicFetchIP(mustIP(t, "1.1.1.1")) {
		t.Fatal("public IP rejected")
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func mustIP(t *testing.T, raw string) netip.Addr {
	t.Helper()
	ip, err := netip.ParseAddr(raw)
	if err != nil {
		t.Fatal(err)
	}
	return ip
}

func TestFetchVerifiedStreaming(t *testing.T) {
	body := []byte("verified pack bytes")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != fetchUserAgent {
			t.Errorf("unexpected User-Agent %q", r.Header.Get("User-Agent"))
		}
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/pack", http.StatusFound)
			return
		}
		if r.Header.Get("Referer") != "" {
			t.Errorf("redirect forwarded a secret-bearing Referer: %q", r.Header.Get("Referer"))
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		w.Write(body)
	}))
	defer server.Close()
	dir := t.TempDir()
	path, err := localFetch(t, server, []string{server.URL + "/redirect?secret=token"}, testHashes(body), int64(len(body)), 128, dir, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	actual, err := os.ReadFile(path)
	if err != nil || string(actual) != string(body) {
		t.Fatalf("returned file contents = %q, %v", actual, err)
	}
	if len(tempFiles(t, dir)) != 1 {
		t.Fatal("download file not retained for caller")
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("download file not private: %v, %v", info, err)
	}
}

func TestFetchRejectsBadResponsesAndCleansUp(t *testing.T) {
	body := []byte("good bytes")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/status":
			w.WriteHeader(http.StatusTooManyRequests)
		case "/html":
			w.Header().Set("Content-Type", "text/html")
			w.Write(body)
		case "/redirect":
			http.Redirect(w, r, "http://127.0.0.1/private?secret=token", http.StatusFound)
		case "/badhost":
			http.Redirect(w, r, "https://localhost/private?secret=token", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop?secret=token", http.StatusFound)
		case "/truncated":
			w.Header().Set("Content-Length", "30")
			w.Write(body)
		case "/oversize":
			w.(http.Flusher).Flush()
			w.Write([]byte(strings.Repeat("x", 200)))
		default:
			w.Write(body)
		}
	}))
	defer server.Close()
	for _, tc := range []struct {
		name, path  string
		hashes      Hashes
		size, limit int64
	}{
		{"status", "/status", Hashes{}, 0, 128},
		{"html", "/html", Hashes{}, 0, 128},
		{"redirect", "/redirect", Hashes{}, 0, 128},
		{"HTTPS redirect to local host", "/badhost", Hashes{}, 0, 128},
		{"redirect limit", "/loop", Hashes{}, 0, 128},
		{"truncated", "/truncated", Hashes{}, 0, 128},
		{"oversize unknown length", "/oversize", Hashes{}, 0, 32},
		{"wrong SHA1", "/good", Hashes{SHA1: strings.Repeat("0", 40)}, 0, 128},
		{"wrong SHA512", "/good", Hashes{SHA512: strings.Repeat("0", 128)}, 0, 128},
		{"wrong expected size", "/good", Hashes{}, 2, 128},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path, err := localFetch(t, server, []string{server.URL + tc.path + "?secret=token"}, tc.hashes, tc.size, tc.limit, dir, context.Background())
			if err == nil || path != "" || strings.Contains(err.Error(), "token") {
				t.Fatalf("unexpected result %q, %v", path, err)
			}
			if files := tempFiles(t, dir); len(files) != 0 {
				t.Fatalf("orphaned download files: %v", files)
			}
		})
	}
}

func TestFetchAlternativesAndPreflight(t *testing.T) {
	body := []byte("fallback")
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/first" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write(body)
	}))
	defer server.Close()
	dir := t.TempDir()
	name, err := localFetch(t, server, []string{server.URL + "/first", server.URL + "/second"}, testHashes(body), int64(len(body)), 20, dir, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(name)
	count := requests.Load()
	if _, err := localFetch(t, server, []string{server.URL + "/first", "http://localhost/private?token=secret"}, Hashes{}, 0, 20, dir, context.Background()); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("unsafe alternative was not rejected before requesting: %v", err)
	}
	if requests.Load() != count {
		t.Fatal("unsafe alternative was checked only after a network request")
	}
	if files := tempFiles(t, dir); len(files) != 0 {
		t.Fatalf("orphaned download files: %v", files)
	}
}

func TestFetchCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("first chunk"))
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	result := make(chan error, 1)
	go func() {
		_, err := localFetch(t, server, []string{server.URL}, Hashes{}, 0, 128, dir, ctx)
		result <- err
	}()
	<-started
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation not reported: %v", err)
	}
	if files := tempFiles(t, dir); len(files) != 0 {
		t.Fatalf("cancelled download left files: %v", files)
	}
}

func TestFetchClientCannotInjectUnsafeTransport(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("unsafe transport was called")
		return nil, io.EOF
	})}
	_, err := Fetch(context.Background(), client, []string{"https://cdn.modrinth.net/file"}, Hashes{}, 0, 20, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "HTTP transport") {
		t.Fatalf("unsafe transport accepted: %v", err)
	}
}

func TestSecureFetchTransportDiscardsCallerHooks(t *testing.T) {
	unsafe := &http.Transport{
		Proxy: func(*http.Request) (*url.URL, error) {
			t.Fatal("caller proxy used")
			return nil, nil
		},
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			t.Fatal("caller dial hook used")
			return nil, io.EOF
		},
	}
	safe, err := secureFetchTransport(unsafe)
	if err != nil {
		t.Fatal(err)
	}
	defer safe.CloseIdleConnections()
	if safe.Proxy != nil || safe.DialContext == nil {
		t.Fatal("fetch transport retained caller network policy")
	}
	if _, err := safe.DialContext(context.Background(), "tcp", "127.0.0.1:443"); err == nil {
		t.Fatal("loopback bypassed destination policy")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
