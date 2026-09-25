package fabric

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestResolveRequiresHashAndReusesVerifiedProfileOffline(t *testing.T) {
	const hash = "f92e777f4341930bad9b2422283c4680d00dbc06" // SHA-1 of "jar".
	sidecar := "invalid"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/profile/json"):
			fmt.Fprint(w, `{"mainClass":"fabric.Main","libraries":[{"name":"group:lib:1"}]}`)
		case strings.HasSuffix(r.URL.Path, "/lib-1.jar.sha1"):
			fmt.Fprint(w, sidecar)
		default:
			fmt.Fprint(w, `[{"loader":{"version":"1"}}]`)
		}
	}))
	defer server.Close()
	previous := client
	client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r = r.Clone(r.Context())
		u := *r.URL
		u.Scheme, u.Host = "http", server.Listener.Addr().String()
		r.URL, r.Host = &u, ""
		return http.DefaultTransport.RoundTrip(r)
	})}
	t.Cleanup(func() { client = previous })
	root := t.TempDir()
	libs := filepath.Join(root, "libraries")
	var wg sync.WaitGroup
	called := 0
	fetch := func(url, dest, sha string) error {
		defer wg.Done()
		called++
		if sha != hash || !strings.HasSuffix(dest, filepath.Join("group", "lib", "1", "lib-1.jar")) {
			return fmt.Errorf("wrong artifact: %s %s %s", url, dest, sha)
		}
		return nil
	}
	if _, err := Resolve(context.Background(), "test", root, libs, &wg, fetch); err == nil || called != 0 {
		t.Fatalf("unhashed library accepted: %v", err)
	}
	sidecar = hash
	if _, err := Resolve(context.Background(), "test", root, libs, &wg, fetch); err != nil || called != 1 {
		t.Fatalf("valid library not prepared: %v, calls=%d", err, called)
	}
	if _, err := os.Stat(ProfilePath(root, "test")); err != nil {
		t.Fatal("profile not cached:", err)
	}
	server.Close()
	if _, err := Resolve(context.Background(), "test", root, libs, &wg, fetch); err != nil || called != 2 {
		t.Fatalf("offline profile did not verify library: %v, calls=%d", err, called)
	}
}
