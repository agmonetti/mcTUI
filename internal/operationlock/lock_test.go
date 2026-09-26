package operationlock

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestExclusiveAcrossProcesses(t *testing.T) {
	dir := os.Getenv("MCTUI_LOCK_DIR")
	if dir == "" {
		dir = t.TempDir()
	}
	if os.Getenv("MCTUI_LOCK_HELPER") == "1" {
		release, err := Acquire(dir)
		if err != nil {
			os.Exit(2)
		}
		if err := release(); err != nil {
			os.Exit(3)
		}
		return
	}
	child := func() error {
		cmd := exec.Command(os.Args[0], "-test.run=^TestExclusiveAcrossProcesses$")
		cmd.Env = append(os.Environ(), "MCTUI_LOCK_HELPER=1", "MCTUI_LOCK_DIR="+dir)
		return cmd.Run()
	}
	release, err := Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := child(); err == nil {
		t.Fatal("another process acquired the held lock")
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	if err := child(); err != nil {
		t.Fatalf("released lock remained busy: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, ".mctui-operation.lock")); err != nil {
		t.Fatalf("stable lock path missing: %v", err)
	}
}
