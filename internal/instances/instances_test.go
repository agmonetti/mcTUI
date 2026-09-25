package instances

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateListRenameAndIsolate(t *testing.T) {
	root := t.TempDir()
	a, err := Create(root, "A", "1.20.4", "Fabric", "0.15.11", 2048)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Create(root, "B", "1.20.4", "Fabric", "0.15.11", 2048)
	if err != nil {
		t.Fatal(err)
	}
	dirA, err := GameDir(root, a)
	if err != nil {
		t.Fatal(err)
	}
	dirB, err := GameDir(root, b)
	if err != nil || dirA == dirB {
		t.Fatalf("game directories must differ: %q and %q (%v)", dirA, dirB, err)
	}
	runtimeA, _ := RuntimeDir(root, a)
	runtimeB, _ := RuntimeDir(root, b)
	if runtimeA != runtimeB || runtimeA == dirA {
		t.Fatal("managed instances must share runtime but not mutable game data")
	}
	if err := Rename(root, a.ID, "Renamed"); err != nil {
		t.Fatal(err)
	}
	found, err := List(root)
	if err != nil || len(found) != 3 || found[0].ID != LegacyID {
		t.Fatalf("list = %+v, %v", found, err)
	}
	updated, err := Read(root, a.ID)
	if err != nil || updated.Name != "Renamed" || updated.ID != a.ID {
		t.Fatalf("rename lost identity: %+v, %v", updated, err)
	}
	legacyDir, err := GameDir(root, found[0])
	if err != nil || legacyDir == dirA || legacyDir == dirB {
		t.Fatalf("legacy path changed: %q, %v", legacyDir, err)
	}
	if err := Rename(root, LegacyID, "nope"); err == nil {
		t.Fatal("legacy entry must not be renamed as managed metadata")
	}
	selected, err := Selected(root)
	if err != nil || selected.ID != LegacyID {
		t.Fatalf("default selection: %+v, %v", selected, err)
	}
	if err := Select(root, a.ID); err != nil {
		t.Fatal(err)
	}
	selected, err = Selected(root)
	if err != nil || selected.ID != a.ID {
		t.Fatalf("persisted selection: %+v, %v", selected, err)
	}
	if err := Select(root, "../outside"); err == nil {
		t.Fatal("invalid selection accepted")
	}
}

func TestRejectLinkedManagedGameAndRuntimeDirectories(t *testing.T) {
	root := t.TempDir()
	in, err := Create(root, "A", "1.20.4", "Vanilla", "", 1024)
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	gameDir := filepath.Join(root, "instances", in.ID, "minecraft")
	if err := os.Symlink(outside, gameDir); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := GameDir(root, in); err == nil {
		t.Fatal("linked game directory accepted")
	}
	if err := os.Remove(gameDir); err != nil {
		t.Fatal(err)
	}
	runtimeDir := filepath.Join(root, "runtime")
	if err := os.Symlink(outside, runtimeDir); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := RuntimeDir(root, in); err == nil {
		t.Fatal("linked runtime directory accepted")
	}
}

func TestRejectLinkedInstancesDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "instances")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := List(root); err == nil {
		t.Fatal("linked instances directory accepted for listing")
	}
	if _, err := Create(root, "A", "1.20.4", "Vanilla", "", 1024); err == nil {
		t.Fatal("linked instances directory accepted for creation")
	}
}

func TestRejectInvalidPathsAndMetadata(t *testing.T) {
	root := t.TempDir()
	for _, version := range []string{"../escape", "C:drive", "control\n"} {
		if _, err := Create(root, "A", version, "Vanilla", "", 1024); err == nil {
			t.Fatalf("invalid version %q accepted", version)
		}
	}
	if _, err := Create(root, "control\x1b[31m", "1.20.4", "Vanilla", "", 1024); err == nil {
		t.Fatal("control characters accepted in name")
	}
	if _, err := Create(root, "A", "1.20.4", "Fabric", "", 1024); err == nil {
		t.Fatal("Fabric without exact loader accepted")
	}
	if _, err := Read(root, "../outside"); err == nil {
		t.Fatal("invalid ID accepted")
	}
	in, err := Create(root, "A", "1.20.4", "Vanilla", "", 1024)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "instances", in.ID, "instance.json")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "outside"), path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Read(root, in.ID); err == nil {
		t.Fatal("linked metadata accepted")
	}
}
