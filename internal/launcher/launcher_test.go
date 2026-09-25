package launcher

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestPrepareSharedRuntimeWorksOfflineAndRejectsCorruptCache(t *testing.T) {
	root := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, r.URL.Path)
	}))
	version := "1.20.4"
	clientHash := "15a5f701524b9430ea1c1ee08841bfd846c1b11d"
	libHash := "d71c71eb256c0f5918043e198dc95327f552e205"
	meta := fmt.Sprintf(`{"assetIndex":{"id":"test"},"downloads":{"client":{"url":%q,"sha1":%q}},"libraries":[{"name":"group:lib:1","downloads":{"artifact":{"url":%q,"sha1":%q,"path":"group/lib/1/lib.jar"}}}]}`,
		server.URL+"/client", clientHash, server.URL+"/lib", libHash)
	cache := filepath.Join(root, "versions", version)
	if err := os.MkdirAll(cache, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "version.json"), []byte(meta), 0600); err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(root, "assets", "indexes")
	if err := os.MkdirAll(index, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(index, "test.json"), []byte(`{"objects":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	var checked atomic.Int32
	prepared, err := Prepare(root, version, "Vanilla", func(string) { checked.Add(1) })
	if err != nil || checked.Load() != 2 || !strings.Contains(prepared.classpath, "lib.jar") {
		t.Fatalf("initial preparation failed: %+v, %v, files=%d", prepared, err, checked.Load())
	}
	server.Close()
	if _, err := Prepare(root, version, "Vanilla", nil); err != nil {
		t.Fatal("cached preparation must work offline:", err)
	}
	if err := os.WriteFile(filepath.Join(cache, "client.jar"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(root, version, "Vanilla", nil); err == nil {
		t.Fatal("corrupt cached client must not be announced ready offline")
	}
	if err := os.WriteFile(filepath.Join(cache, "client.jar"), []byte("/client"), 0600); err != nil {
		t.Fatal(err)
	}
	bad := strings.Replace(meta, "group/lib/1/lib.jar", "../escape.jar", 1)
	if err := os.WriteFile(filepath.Join(cache, "version.json"), []byte(bad), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(root, version, "Vanilla", nil); err == nil {
		t.Fatal("library path traversal accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "escape.jar")); !os.IsNotExist(err) {
		t.Fatalf("escaped runtime: %v", err)
	}
}

func TestInvalidAssetIndexNeverQueuesDownloads(t *testing.T) {
	root := t.TempDir()
	index := filepath.Join(root, "index.json")
	for _, contents := range []string{"not json", `{}`, `{"objects":{"bad":{"hash":"x"}}}`} {
		if err := os.WriteFile(index, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
		called := false
		if err := downloadAssetObjects(root, index, &sync.WaitGroup{}, func(string, string, string) { called = true }); err == nil || called {
			t.Fatalf("invalid index accepted or downloads queued: %q", contents)
		}
	}
}

func TestRunGameUsesSeparateGameAndRuntimeDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell script")
	}
	root := t.TempDir()
	game := filepath.Join(root, "instance", "minecraft")
	runtimeDir := filepath.Join(root, "runtime")
	argsFile := filepath.Join(root, "args")
	pwdFile := filepath.Join(root, "pwd")
	java := filepath.Join(root, "java")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" + argsFile + "'\npwd > '" + pwdFile + "'\n"
	if err := os.WriteFile(java, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	runGame(game, runtimeDir, java, "fake-classpath", "fake.Main", "Player", "1.20.4", "assets", 1024, "")
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "--gameDir\n"+game+"\n") || !strings.Contains(string(args), "--assetsDir\n"+filepath.Join(runtimeDir, "assets")+"\n") {
		t.Fatalf("wrong game/runtime paths: %s", args)
	}
	pwd, err := os.ReadFile(pwdFile)
	if err != nil || strings.TrimSpace(string(pwd)) != game {
		t.Fatalf("working directory = %q, error = %v; want %q", pwd, err, game)
	}
}
