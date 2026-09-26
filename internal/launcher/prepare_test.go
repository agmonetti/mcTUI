package launcher

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareContextRejectsCanceledAndIncompletePreparation(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := PrepareContext(ctx, root, "1.20.4", "Vanilla", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled preparation error = %v", err)
	}

	versionDir := filepath.Join(root, "versions", "1.20.4")
	if err := os.MkdirAll(versionDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "version.json"), []byte(`{"downloads":{"client":{"url":"https://example.invalid/client","sha1":"0000000000000000000000000000000000000000"}},"assetIndex":{"id":"assets"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := PrepareContext(context.Background(), root, "1.20.4", "Vanilla", nil)
	var prepareErr *PrepareError
	if !errors.As(err, &prepareErr) || prepareErr.Stage != "client" {
		t.Fatalf("incomplete metadata error = %#v, want typed client error", err)
	}
}
