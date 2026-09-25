package mojang

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMetadataErrorsAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest":
			fmt.Fprintf(w, `{"versions":[{"id":"test","url":%q,"sha1":%q},{"id":"bad","url":%q,"sha1":%q}]}`, "http://"+r.Host+"/version", strings.Repeat("0", 40), "http://"+r.Host+"/bad", strings.Repeat("0", 40))
		case "/version":
			w.WriteHeader(http.StatusNotFound)
		case "/bad":
			fmt.Fprint(w, `{"assetIndex":{"id":"assets","url":"https://example.org/index"},"downloads":{"client":{"url":"https://example.org/client"}}}`)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	if _, err := fetchVersionData(context.Background(), server.URL+"/manifest", "test", root); err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("metadata failure hidden: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fetchVersionData(ctx, server.URL+"/manifest", "test", root); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	if _, err := fetchVersionData(context.Background(), server.URL+"/manifest", "bad", root); err == nil || !strings.Contains(err.Error(), "SHA-1 mismatch") {
		t.Fatalf("metadata hash mismatch accepted: %v", err)
	}
	for _, version := range []string{"test", "bad"} {
		if _, err := os.Stat(filepath.Join(root, "versions", version, "version.json")); !os.IsNotExist(err) {
			t.Fatalf("invalid metadata cached: %v", err)
		}
	}
}

func TestDownloadRejectsUnverifiedAndCanceled(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "test.jar")
	if err := os.WriteFile(path, []byte("cached"), 0600); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	if err := Download(context.Background(), &wg)("https://example.org/file", path, ""); err == nil {
		t.Fatal("unhashed cache accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	wg.Add(1)
	if err := Download(ctx, &wg)("https://example.org/file", path, strings.Repeat("a", 40)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel lost: %v", err)
	}
	if data, _ := os.ReadFile(path); string(data) != "cached" {
		t.Fatal("failed download changed destination")
	}
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel = context.WithCancel(context.Background())
	result := make(chan error, 1)
	wg.Add(1)
	go func() {
		result <- Download(ctx, &wg)(server.URL, filepath.Join(root, "pending.jar"), strings.Repeat("a", 40))
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("request never started")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("in-flight request was not canceled: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "pending.jar")); !os.IsNotExist(err) {
		t.Fatalf("canceled download published a file: %v", err)
	}
}

func TestDownloadVerifiedAndPreservesDestinationOnFailure(t *testing.T) {
	const body = "valid jar bytes"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/missing":
			http.NotFound(w, r)
		case "/html":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, "<html>not a jar</html>")
		case "/truncated":
			w.Header().Set("Content-Length", "100")
			fmt.Fprint(w, "short")
		default:
			fmt.Fprint(w, body)
		}
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "libraries", "example.jar")
	var wg sync.WaitGroup
	download := Download(context.Background(), &wg)
	fetch := func(endpoint, expected string) error {
		wg.Add(1)
		return download(server.URL+endpoint, path, expected)
	}
	hash := "ffa154019e3845ca1a02f24534cada695b4d3258" // Mojang SHA-1 of "valid jar bytes".
	if err := fetch("/ok", hash); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := fetch("/missing", hash); err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("missing HTTP error: %v", err)
	}
	for _, endpoint := range []string{"/html", "/truncated"} {
		if err := fetch(endpoint, hash); err == nil {
			t.Fatalf("%s accepted", endpoint)
		}
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "corrupt" {
		t.Fatalf("failed transfer replaced old file: %q, %v", got, err)
	}
	if err := fetch("/ok", hash); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != body {
		t.Fatalf("cache not repaired: %q, %v", got, err)
	}
	wg.Wait()
}
