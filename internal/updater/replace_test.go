package updater

import (
	"os"
	"testing"
)

func TestExecutablePath(t *testing.T) {
	p, err := ExecutablePath()
	if err != nil {
		t.Fatalf("ExecutablePath returned error: %v", err)
	}
	if p == "" {
		t.Fatal("ExecutablePath returned empty string")
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("ExecutablePath %q cannot be statted: %v", p, err)
	}
}
