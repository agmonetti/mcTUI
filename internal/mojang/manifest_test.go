package mojang

import (
	"crypto/sha1"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

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
	download := Download(&wg)
	fetch := func(endpoint, expected string) error {
		wg.Add(1)
		return download(server.URL+endpoint, path, expected)
	}
	hash := fmt.Sprintf("%x", sha1.Sum([]byte(body)))
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
