package mojang

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func nativeZIP(t *testing.T, names ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, name := range names {
		f, err := w.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte("native contents")); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestNativesRebuiltOfflineAndFailuresNeverPublished(t *testing.T) {
	root := t.TempDir()
	body := nativeZIP(t, "libtest.so")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
	defer server.Close()
	var data VersionData
	// Mojang requires SHA-1 in its metadata; this is an integrity fixture, not a security primitive.
	h := sha1.New() // nosemgrep: go.lang.security.audit.crypto.use_of_weak_crypto.use-of-sha1
	_, _ = h.Write(body)
	meta := fmt.Sprintf(`{"libraries":[{"name":%q,"downloads":{"artifact":{"path":"test/native.jar","url":%q,"sha1":"%x"}}}]}`,
		"org.lwjgl:test:1:"+nativeClassifier(runtime.GOOS, runtime.GOARCH), server.URL, h.Sum(nil))
	if err := json.Unmarshal([]byte(meta), &data); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	prepare := func() (string, error) {
		return DownloadAndExtractNatives(context.Background(), data, root, "test", &wg, Download(context.Background(), &wg))
	}
	dir, err := prepare()
	if err != nil || dir != NativeLibDir(root, "test") {
		t.Fatalf("prepare: %q, %v", dir, err)
	}
	server.Close()
	dest := filepath.Join(dir, "libtest.so")
	if err := os.WriteFile(dest, []byte("altered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepare(); err != nil {
		t.Fatal("offline extraction:", err)
	}
	if got, err := os.ReadFile(dest); err != nil || string(got) != "native contents" {
		t.Fatalf("native cache not repaired: %q, %v", got, err)
	}
	// Nonempty extraction must not hide a broken archive/cache.
	if err := os.WriteFile(filepath.Join(root, "libraries", "test", "native.jar"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepare(); err == nil {
		t.Fatal("broken archive accepted offline")
	}
	data.Libraries[0].Downloads.Artifact.Path = "../../escape.jar"
	if _, err := prepare(); err == nil {
		t.Fatal("native path traversal accepted")
	}
	if got, err := os.ReadFile(dest); err != nil || string(got) != "native contents" {
		t.Fatalf("failure changed published native: %q, %v", got, err)
	}
	stages, _ := filepath.Glob(filepath.Join(root, "versions", "test", ".mctui-natives-*"))
	if len(stages) != 0 {
		t.Fatalf("staging leaked: %v", stages)
	}
	wg.Wait()
}

func TestNativeArchiveRejectsUnsafeOrCorruptContent(t *testing.T) {
	corrupt := nativeZIP(t, "lib.so")
	corrupt = bytes.Replace(corrupt, []byte("native contents"), []byte("broken contents"), 1)
	for name, body := range map[string][]byte{
		"not zip":      []byte("not zip"),
		"crc":          corrupt,
		"traversal":    nativeZIP(t, "../lib.so"),
		"windows path": nativeZIP(t, `C:\lib.dll`),
		"absolute":     nativeZIP(t, "/lib.so"),
		"collision":    nativeZIP(t, "one/lib.so", "two/LIB.so"),
		"empty":        nativeZIP(t, "readme.txt"),
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			jar := filepath.Join(root, "test.jar")
			if err := os.WriteFile(jar, body, 0600); err != nil {
				t.Fatal(err)
			}
			if err := extractJar(jar, root, make(map[string]bool)); err == nil {
				t.Fatal("invalid archive accepted")
			}
		})
	}
}

func TestNativePlatformAndRules(t *testing.T) {
	for _, tc := range []struct{ os, arch, want string }{
		{"linux", "amd64", "natives-linux"}, {"darwin", "arm64", "natives-macos-arm64"},
		{"windows", "386", "natives-windows-x86"}, {"windows", "arm64", "natives-windows-arm64"},
	} {
		if got := nativeClassifier(tc.os, tc.arch); got != tc.want {
			t.Fatalf("%s/%s: %q", tc.os, tc.arch, got)
		}
	}
	var data VersionData
	if err := json.Unmarshal([]byte(fmt.Sprintf(`{"libraries":[{"rules":[{"action":"allow"},{"action":"disallow","os":{"name":%q}}]}]}`, osName())), &data); err != nil {
		t.Fatal(err)
	}
	if ShouldIncludeLibrary("test", data.Libraries[0].Rules) {
		t.Fatal("Mojang disallow rule ignored")
	}
	if ShouldIncludeLibrary("org.lwjgl:test:1:natives-unsupported", nil) {
		t.Fatal("wrong-platform native included")
	}
	if !strings.Contains(NativeLibDir(t.TempDir(), "test"), runtime.GOOS+"-"+runtime.GOARCH) {
		t.Fatal("native cache not platform-specific")
	}
}

func TestNativePathRejectsLinks(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "libraries")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if _, err := nativePath(root, "libraries/test.jar"); err == nil {
		t.Fatal("linked library directory accepted")
	}
}
