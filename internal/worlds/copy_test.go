package worlds

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mcTUI/internal/instances"
)

func copyFixtures(t *testing.T) (string, instances.Instance, instances.Instance, string, string) {
	t.Helper()
	root := t.TempDir()
	source := instances.Instance{ID: strings.Repeat("a", 32), Kind: "managed", MinecraftVersion: "1.12"}
	destination := instances.Instance{ID: strings.Repeat("b", 32), Kind: "managed", MinecraftVersion: "1.21"}
	src := filepath.Join(root, "instances", source.ID, "minecraft")
	dst := filepath.Join(root, "instances", destination.ID, "minecraft")
	for _, path := range []string{src, dst} {
		if err := os.MkdirAll(filepath.Join(path, "saves"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	return root, source, destination, src, dst
}

func putCopyFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCopyPreservesCompleteWorldAcrossVersionMismatch(t *testing.T) {
	root, source, destination, src, dst := copyFixtures(t)
	world := filepath.Join(src, "saves", "My World")
	files := map[string]string{
		"level.dat":             "world metadata",
		"region/r.0.0.mca":      "region bytes\x00\xff",
		"data/map_0.dat":        "map data",
		"playerdata/player.dat": "inventory",
	}
	for name, data := range files {
		putCopyFile(t, filepath.Join(world, name), data)
	}
	if err := os.Mkdir(filepath.Join(world, "empty"), 0700); err != nil {
		t.Fatal(err)
	}
	name, err := Copy(root, source, destination, "My World")
	if err != nil || name != "My World" {
		t.Fatalf("Copy = %q, %v", name, err)
	}
	for filename, data := range files {
		copied, err := os.ReadFile(filepath.Join(dst, "saves", name, filename))
		if err != nil || string(copied) != data {
			t.Fatalf("copied %s = %q, %v", filename, copied, err)
		}
	}
	if info, err := os.Stat(filepath.Join(dst, "saves", name, "empty")); err != nil || !info.IsDir() {
		t.Fatalf("empty directory not copied: %v", err)
	}
	putCopyFile(t, filepath.Join(dst, "saves", name, "region", "r.0.0.mca"), "changed")
	original, err := os.ReadFile(filepath.Join(world, "region", "r.0.0.mca"))
	if err != nil || string(original) != files["region/r.0.0.mca"] {
		t.Fatalf("copy modified source: %q, %v", original, err)
	}
}

func TestCopyCollisionsNeverOverwriteExistingWorlds(t *testing.T) {
	root, source, destination, src, dst := copyFixtures(t)
	putCopyFile(t, filepath.Join(src, "saves", "world", "level.dat"), "source")
	putCopyFile(t, filepath.Join(dst, "saves", "world", "level.dat"), "existing")
	putCopyFile(t, filepath.Join(dst, "saves", "world (copy)", "level.dat"), "first copy")
	name, err := Copy(root, source, destination, "world")
	if err != nil || name != "world (copy 2)" {
		t.Fatalf("Copy = %q, %v", name, err)
	}
	for _, entry := range []struct{ name, content string }{
		{"world", "existing"}, {"world (copy)", "first copy"}, {name, "source"},
	} {
		data, err := os.ReadFile(filepath.Join(dst, "saves", entry.name, "level.dat"))
		if err != nil || string(data) != entry.content {
			t.Fatalf("%s = %q, %v", entry.name, data, err)
		}
	}
}

func TestCopyRejectsLinksAndLeavesNoPartialWorld(t *testing.T) {
	root, source, destination, src, dst := copyFixtures(t)
	world := filepath.Join(src, "saves", "world")
	putCopyFile(t, filepath.Join(world, "a.txt"), "already copied before link")
	if err := os.Symlink(filepath.Join(world, "a.txt"), filepath.Join(world, "z-link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Copy(root, source, destination, "world"); err == nil {
		t.Fatal("expected rejection of symlink in save")
	}
	entries, err := os.ReadDir(filepath.Join(dst, "saves"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("partial save visible: %v, %v", entries, err)
	}
	gameEntries, err := os.ReadDir(dst)
	if err != nil || len(gameEntries) != 1 || gameEntries[0].Name() != "saves" {
		t.Fatalf("staging directory not cleaned: %v, %v", gameEntries, err)
	}
	if err := os.Remove(filepath.Join(world, "z-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(world); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dst, world); err != nil {
		t.Fatal(err)
	}
	if _, err := Copy(root, source, destination, "world"); err == nil {
		t.Fatal("expected rejection of symlink world root")
	}
}

func TestCopyRejectsLinkedDestinationSaves(t *testing.T) {
	root, source, destination, src, dst := copyFixtures(t)
	putCopyFile(t, filepath.Join(src, "saves", "world", "level.dat"), "source")
	external := filepath.Join(t.TempDir(), "outside")
	if err := os.Mkdir(external, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dst, "saves")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(dst, "saves")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Copy(root, source, destination, "world"); err == nil {
		t.Fatal("accepted linked destination saves")
	}
	entries, err := os.ReadDir(external)
	if err != nil || len(entries) != 0 {
		t.Fatalf("wrote outside destination: %v, %v", entries, err)
	}
}

func TestCopyRejectsSameInstanceAndTraversal(t *testing.T) {
	root, source, destination, src, dst := copyFixtures(t)
	putCopyFile(t, filepath.Join(src, "saves", "world", "level.dat"), "source")
	for _, folder := range []string{"", ".", "..", "../world", "x/y", `x\y`} {
		if _, err := Copy(root, source, destination, folder); err == nil {
			t.Errorf("Copy accepted folder %q", folder)
		}
	}
	if _, err := Copy(root, source, source, "world"); err == nil {
		t.Fatal("Copy accepted identical instances")
	}
	entries, err := os.ReadDir(filepath.Join(dst, "saves"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("unexpected destination save: %v, %v", entries, err)
	}
}

func TestCopyFromLegacyInstance(t *testing.T) {
	root, _, destination, _, dst := copyFixtures(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", home)
	legacy := instances.Instance{ID: instances.LegacyID, Kind: "legacy", MinecraftVersion: "old"}
	putCopyFile(t, filepath.Join(home, ".minecraft", "saves", "Legacy Save", "level.dat"), "legacy data")
	name, err := Copy(root, legacy, destination, "Legacy Save")
	if err != nil || name != "Legacy Save" {
		t.Fatalf("Copy legacy = %q, %v", name, err)
	}
	data, err := os.ReadFile(filepath.Join(dst, "saves", name, "level.dat"))
	if err != nil || string(data) != "legacy data" {
		t.Fatalf("legacy content = %q, %v", data, err)
	}
}
