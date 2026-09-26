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

func TestExactLoaderNeverUsesDefaultOrOtherCachedProfile(t *testing.T) {
	const hash = "f92e777f4341930bad9b2422283c4680d00dbc06"
	requests := make([]string, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		switch {
		case strings.HasSuffix(r.URL.Path, "/0.15.11/profile/json"):
			fmt.Fprint(w, `{"id":"fabric-loader-0.15.11-1.20.4","mainClass":"fabric.Main","libraries":[{"name":"group:lib:1"}]}`)
		case strings.HasSuffix(r.URL.Path, "/0.15.12/profile/json"):
			w.WriteHeader(http.StatusNotFound)
		case strings.HasSuffix(r.URL.Path, "/0.15.13/profile/json"):
			fmt.Fprint(w, `{"id":"fabric-loader-0.15.99-1.20.4","mainClass":"fabric.Main","libraries":[{"name":"group:lib:1"}]}`)
		case strings.HasSuffix(r.URL.Path, "/lib-1.jar.sha1"):
			fmt.Fprint(w, hash)
		default:
			fmt.Fprint(w, `[{"loader":{"version":"0.15.99"}}]`)
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
	var wg sync.WaitGroup
	fetch := func(_, _, sha string) error {
		defer wg.Done()
		if sha != hash {
			return fmt.Errorf("wrong hash: %s", sha)
		}
		return nil
	}
	resolve := func(loader string) (Profile, error) {
		return Resolve(context.Background(), "1.20.4", root, filepath.Join(root, "libraries"), &wg, fetch, loader)
	}
	profile, err := resolve("0.15.11")
	if err != nil || profile.LoaderVersion != "0.15.11" || profile.MinecraftVersion != "1.20.4" {
		t.Fatalf("exact loader profile = %+v, %v", profile, err)
	}
	if len(requests) != 2 || requests[0] != "/v2/versions/loader/1.20.4/0.15.11/profile/json" {
		t.Fatalf("requested endpoints %v; expected exact profile without default loader discovery", requests)
	}
	if _, err := resolve("0.15.12"); err == nil {
		t.Fatal("unsupported pinned loader silently used another version")
	}
	if _, err := resolve("0.15.13"); err == nil {
		t.Fatal("server returned a different loader identity")
	}
	server.Close()
	if _, err := resolve("0.15.11"); err != nil {
		t.Fatalf("pinned cache not reusable offline: %v", err)
	}
	if _, err := resolve("0.15.12"); err == nil {
		t.Fatal("other loader reused cached 0.15.11 profile offline")
	}
	if _, err := resolve("../escape"); err == nil {
		t.Fatal("invalid loader version accepted")
	}
}

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
