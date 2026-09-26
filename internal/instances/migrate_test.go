package instances

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCopyFromLegacyCopiesSelectedItemsWithoutChangingSource(t *testing.T) {
	root, source := t.TempDir(), t.TempDir()
	for path, content := range map[string]string{
		"mods/example.jar":       "mod",
		"config/options.txt":     "config",
		"saves/world/level.dat":  "world",
		"resourcepacks/pack.zip": "pack",
		"shaderpacks/shader.zip": "shader",
		"options.txt":            "video settings",
	} {
		fullPath := filepath.Join(source, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	items, err := InspectCopySource(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != len(copyableLegacyItems) {
		t.Fatalf("inspect returned %d items; want %d", len(items), len(copyableLegacyItems))
	}

	selected := []string{"mods", "saves", "options.txt"}
	result, err := CopyFromLegacy(root, source, "Imported", "1.21.1", selected, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if result.Instance.Name != "Imported" || result.Instance.MinecraftVersion != "1.21.1" || result.Instance.Loader != "Vanilla" || result.StagingPath != "" {
		t.Fatalf("unexpected published instance/result: %+v", result)
	}
	gameDir, err := GameDir(root, result.Instance)
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"mods/example.jar":      "mod",
		"saves/world/level.dat": "world",
		"options.txt":           "video settings",
	} {
		data, err := os.ReadFile(filepath.Join(gameDir, filepath.FromSlash(path)))
		if err != nil || string(data) != want {
			t.Fatalf("copied %s = %q, %v; want %q", path, data, err, want)
		}
	}
	for _, omitted := range []string{"config", "resourcepacks", "shaderpacks"} {
		if _, err := os.Lstat(filepath.Join(gameDir, omitted)); !os.IsNotExist(err) {
			t.Fatalf("unselected item %s was copied: %v", omitted, err)
		}
	}
	for path, want := range map[string]string{
		"mods/example.jar":      "mod",
		"saves/world/level.dat": "world",
		"options.txt":           "video settings",
	} {
		data, err := os.ReadFile(filepath.Join(source, filepath.FromSlash(path)))
		if err != nil || string(data) != want {
			t.Fatalf("source changed at %s: %q, %v", path, data, err)
		}
	}
	listed, err := List(root)
	if err != nil || len(listed) != 2 || listed[1].ID != result.Instance.ID {
		t.Fatalf("published instance list = %+v, %v", listed, err)
	}
}

func TestCopyInstancePreservesSettingsAndCopiesOnlySelectedData(t *testing.T) {
	root := t.TempDir()
	source, err := Create(root, "Original", "1.21.1", "Fabric", "0.16.0", 4096)
	if err != nil {
		t.Fatal(err)
	}
	source.JavaPath = filepath.Join(root, "java")
	if err := UpdateSettings(root, source); err != nil {
		t.Fatal(err)
	}
	sourceDir, err := GameDir(root, source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(sourceDir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"mods", "saves"} {
		if err := os.Mkdir(filepath.Join(sourceDir, dir), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sourceDir, dir, "data"), []byte(dir), 0600); err != nil {
			t.Fatal(err)
		}
	}
	copy, err := CopyInstance(root, source.ID, "Independent", []string{"mods"})
	if err != nil {
		t.Fatal(err)
	}
	if copy.Instance.ID == source.ID || copy.Instance.Name != "Independent" || copy.Instance.MinecraftVersion != source.MinecraftVersion || copy.Instance.Loader != source.Loader || copy.Instance.LoaderVersion != source.LoaderVersion || copy.Instance.JavaPath != source.JavaPath || copy.Instance.MemoryMB != source.MemoryMB {
		t.Fatalf("duplicated settings = %+v", copy.Instance)
	}
	destDir, err := GameDir(root, copy.Instance)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "saves")); !os.IsNotExist(err) {
		t.Fatalf("worlds copied without selection: %v", err)
	}
	if err := os.WriteFile(filepath.Join(destDir, "mods", "data"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(sourceDir, "mods", "data")); err != nil || string(data) != "mods" {
		t.Fatalf("source data changed with copy: %q, %v", data, err)
	}
}

func TestCopySourceRejectsLinksAndDoesNotPublish(t *testing.T) {
	root, source := t.TempDir(), t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "outside.jar"), []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(source, "mods")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "options.txt"), []byte("safe"), 0600); err != nil {
		t.Fatal(err)
	}
	items, err := InspectCopySource(source)
	if err != nil || items[0].Error == "" {
		t.Fatalf("linked source folder was not marked unavailable: %+v, %v", items, err)
	}
	if _, err := CopyFromLegacy(root, source, "Unsafe", "1.21.1", []string{"mods"}, 2048); err == nil {
		t.Fatal("copy accepted linked source folder")
	}
	listed, err := List(root)
	if err != nil || len(listed) != 1 || listed[0].ID != LegacyID {
		t.Fatalf("failed copy published an instance: %+v, %v", listed, err)
	}
	result, err := CopyFromLegacy(root, source, "Safe selection", "1.21.1", []string{"options.txt"}, 2048)
	if err != nil {
		t.Fatal("unselected linked item blocked a safe copy:", err)
	}
	gameDir, err := GameDir(root, result.Instance)
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(gameDir, "options.txt")); err != nil || string(data) != "safe" {
		t.Fatalf("safe selection not copied: %q, %v", data, err)
	}

	nestedSource := t.TempDir()
	if err := os.Mkdir(filepath.Join(nestedSource, "mods"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "outside.jar"), filepath.Join(nestedSource, "mods", "linked.jar")); err != nil {
		t.Fatal(err)
	}
	nestedItems, err := InspectCopySource(nestedSource)
	if err != nil || nestedItems[0].Error == "" {
		t.Fatalf("nested source link was not rejected: %+v, %v", nestedItems, err)
	}
}

func TestDiscardCopyStageOnlyRemovesValidatedTemporaryDirectory(t *testing.T) {
	root := t.TempDir()
	stage := filepath.Join(root, ".mctui-copy-test")
	if err := os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "partial"), []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := DiscardCopyStage(root, stage); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(stage); !os.IsNotExist(err) {
		t.Fatalf("staging directory remains: %v", err)
	}
	if err := DiscardCopyStage(root, filepath.Join(root, "instances")); err == nil || !strings.Contains(err.Error(), "invalid migration staging path") {
		t.Fatalf("accepted non-staging path: %v", err)
	}
}
