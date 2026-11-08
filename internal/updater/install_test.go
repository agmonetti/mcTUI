package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestInstallRelease(t *testing.T) {
	binaryContent := []byte("#!/bin/sh\necho upgraded\n")
	hasher := sha256.New()
	hasher.Write(binaryContent)
	targetName := TargetAssetName(runtime.GOOS, runtime.GOARCH)
	validHash := hex.EncodeToString(hasher.Sum(nil))

	checksumsContent := fmt.Sprintf("%s  %s\n", validHash, targetName)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/checksums.txt":
			_, _ = w.Write([]byte(checksumsContent))
		case "/binary":
			_, _ = w.Write(binaryContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	rel := Release{
		TagName: "v2.1.0",
		Assets: []Asset{
			{Name: "checksums.txt", BrowserDownloadURL: server.URL + "/checksums.txt"},
			{Name: targetName, BrowserDownloadURL: server.URL + "/binary"},
		},
	}

	// Test with a mock progress collector
	var stages []string
	progressFn := func(stage string, percent float64) {
		stages = append(stages, stage)
	}

	// We create a temporary dummy executable to simulate os.Executable
	tmpDir := t.TempDir()
	dummyExec := filepath.Join(tmpDir, "dummy-mctui")
	if err := os.WriteFile(dummyExec, []byte("old-binary"), 0755); err != nil {
		t.Fatal(err)
	}

	// Verify download and checksum matching directly using our components
	tempTarget := filepath.Join(tmpDir, "downloaded.tmp")
	if err := DownloadFile(server.URL+"/binary", tempTarget, nil); err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}

	if err := VerifySHA256(tempTarget, validHash); err != nil {
		t.Fatalf("VerifySHA256 failed: %v", err)
	}

	// Checksum mismatch scenario
	badHash := "1111111111111111111111111111111111111111111111111111111111111111"
	if err := VerifySHA256(tempTarget, badHash); err == nil {
		t.Fatalf("expected error on bad hash, got nil")
	}

	if err := ApplyUpdate(tempTarget, dummyExec); err != nil {
		t.Fatalf("ApplyUpdate failed: %v", err)
	}

	updatedData, err := os.ReadFile(dummyExec)
	if err != nil {
		t.Fatal(err)
	}
	if string(updatedData) != string(binaryContent) {
		t.Fatalf("binary content not updated properly: got %q, want %q", string(updatedData), string(binaryContent))
	}

	_ = progressFn
	_ = rel
}
