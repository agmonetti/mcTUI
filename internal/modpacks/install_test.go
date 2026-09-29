package modpacks

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"mcTUI/internal/instances"
)

func TestImportLocalMrpackAtomicCreationAndMetadata(t *testing.T) {
	root := t.TempDir()

	idx := Index{
		FormatVersion: 1,
		Game:          "minecraft",
		VersionID:     "pack-1.0.0",
		Name:          "Test Pack",
		Summary:       "A test modpack",
		Dependencies: map[string]string{
			"minecraft":     "1.21.1",
			"fabric-loader": "0.16.0",
		},
		Files: []File{},
	}
	indexBytes, err := json.Marshal(idx)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	iw, err := zw.Create("modrinth.index.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := iw.Write(indexBytes); err != nil {
		t.Fatal(err)
	}

	ow1, err := zw.Create("overrides/config/test.cfg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ow1.Write([]byte("config=initial\n")); err != nil {
		t.Fatal(err)
	}

	ow2, err := zw.Create("client-overrides/options.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ow2.Write([]byte("fov:90\n")); err != nil {
		t.Fatal(err)
	}

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	packFile := filepath.Join(root, "pack.mrpack")
	if err := os.WriteFile(packFile, buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}

	arc, err := Inspect(packFile)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	prepared := false
	store := Store{
		Root: root,
		Prepare: func(ctx context.Context, runtimeDir, version, loader, loaderVersion string) error {
			if version != "1.21.1" || loader != "Fabric" || loaderVersion != "0.16.0" {
				t.Fatalf("unexpected prepare params: %s %s %s", version, loader, loaderVersion)
			}
			prepared = true
			return nil
		},
	}

	in, err := store.Import(context.Background(), arc, Source{Type: "local"}, "Instancia Pack", 2048, nil, nil)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	if !prepared {
		t.Fatal("runtime prepare was not called before publishing instance")
	}

	if in.Name != "Instancia Pack" || in.MinecraftVersion != "1.21.1" || in.Loader != "Fabric" || in.LoaderVersion != "0.16.0" {
		t.Fatalf("unexpected instance metadata: %+v", in)
	}

	// Comprobar que instances.List ve la instancia recién publicada
	list, err := instances.List(root)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, item := range list {
		if item.ID == in.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("imported instance %s not found in instances.List", in.ID)
	}

	// Comprobar archivos en gameDir
	gameDir, err := instances.GameDir(root, in)
	if err != nil {
		t.Fatal(err)
	}
	cfgBytes, err := os.ReadFile(filepath.Join(gameDir, "config", "test.cfg"))
	if err != nil || string(cfgBytes) != "config=initial\n" {
		t.Fatalf("missing or wrong config/test.cfg: %v, %s", err, string(cfgBytes))
	}
	optBytes, err := os.ReadFile(filepath.Join(gameDir, "options.txt"))
	if err != nil || string(optBytes) != "fov:90\n" {
		t.Fatalf("missing or wrong options.txt: %v, %s", err, string(optBytes))
	}

	// Comprobar pack.json (D07)
	manifest, err := loadManifest(filepath.Dir(gameDir))
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}
	if manifest.Name != "Test Pack" || manifest.IndexVersionID != "pack-1.0.0" || manifest.IndexSHA512 != arc.SHA512 {
		t.Fatalf("wrong manifest values: %+v", manifest)
	}
	if len(manifest.Files) != 2 {
		t.Fatalf("expected 2 override files in manifest, got %d", len(manifest.Files))
	}

	// Comprobar que no quedaron directorios temporales .mctui-modpack-* huérfanos
	entries, _ := os.ReadDir(root)
	for _, entry := range entries {
		if filepath.HasPrefix(entry.Name(), ".mctui-modpack-") {
			t.Fatalf("orphan staging directory retained: %s", entry.Name())
		}
	}
}
