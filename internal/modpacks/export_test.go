package modpacks

import (
	"archive/zip"
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mcTUI/internal/instances"
)

func exportFixture(t *testing.T) (Store, instances.Instance, string, string) {
	t.Helper()
	root := t.TempDir()
	in, err := instances.Create(root, "Renamed local instance", "1.20.1", "Fabric", "0.15.11", 2048)
	if err != nil {
		t.Fatal(err)
	}
	gameDir, err := instances.GameDir(root, in)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(gameDir, 0700); err != nil {
		t.Fatal(err)
	}
	mod := []byte("downloaded Modrinth file")
	for path, data := range map[string][]byte{
		"mods/example.jar":              mod,
		"config/example.toml":          []byte("theme = 'green'\n"),
		"resourcepacks/sample/pack.mcmeta": []byte(`{"pack":{"pack_format":15,"description":"Sample"}}`),
		"resourcepacks/sample/assets/minecraft/lang/en_us.json": []byte(`{"item.minecraft.diamond":"Diamond"}`),
		"shaderpacks/sample/shaders/main.fsh": []byte("void main() {}\n"),
		"options.txt":                 []byte("guiScale:2\n"),
		"saves/world/level.dat":         []byte("PRIVATE WORLD"),
		"config/credentials.json":      []byte(`{"access_token":"PRIVATE"}`),
	} {
		file := filepath.Join(gameDir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	one, five := sha1.Sum(mod), sha512.Sum512(mod)
	m := Manifest{
		SchemaVersion: 1, Source: Source{Type: "local"}, Name: "Original pack", Summary: "Original summary",
		IndexVersionID: "release-42", IndexSHA512: strings.Repeat("a", 128),
		MinecraftVersion: "1.20.1", Loader: "Fabric", LoaderVersion: "0.15.11",
		Optional: map[string]bool{"mods/example.jar": true},
		Files: map[string]BaseFile{"mods/example.jar": {
			SHA1: hex.EncodeToString(one[:]), SHA512: hex.EncodeToString(five[:]), Size: int64(len(mod)), Layer: "downloads",
			URLs: []string{"https://cdn.modrinth.com/data/ABCD/versions/EFGH/example.jar"},
		}},
	}
	if err := saveManifest(filepath.Join(root, "instances", in.ID), m); err != nil {
		t.Fatal(err)
	}
	return Store{Root: root}, in, gameDir, filepath.Join(root, "published.mrpack")
}

func TestExportInspectRoundTripPreservesIndexAndSelectedContent(t *testing.T) {
	s, in, _, output := exportFixture(t)
	selected := []string{"config/example.toml", "resourcepacks/sample/pack.mcmeta", "resourcepacks/sample/assets/minecraft/lang/en_us.json", "shaderpacks/sample/shaders/main.fsh", "options.txt"}
	if err := s.Export(context.Background(), in, output, selected); err != nil {
		t.Fatal(err)
	}
	archive, err := Inspect(output)
	if err != nil {
		t.Fatalf("export cannot be inspected: %v", err)
	}
	if archive.Index.FormatVersion != 1 || archive.Index.Game != "minecraft" || archive.Index.VersionID != "release-42" || archive.Index.Name != "Original pack" || archive.Index.Summary != "Original summary" ||
		archive.Index.Dependencies["minecraft"] != "1.20.1" || archive.Index.Dependencies["fabric-loader"] != "0.15.11" {
		t.Fatalf("incorrect exported index: %+v", archive.Index)
	}
	if len(archive.Index.Files) != 1 || archive.Index.Files[0].Path != "mods/example.jar" || archive.Index.Files[0].Env == nil || archive.Index.Files[0].Env.Client != "optional" || len(archive.Overrides) != len(selected) {
		t.Fatalf("incorrect exported inventory: %+v, %+v", archive.Index.Files, archive.Overrides)
	}
	z, err := zip.OpenReader(output)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	contents := make(map[string]string)
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, readErr := io.ReadAll(r)
		if err := r.Close(); err != nil || readErr != nil {
			t.Fatalf("read ZIP entry %s: %v / %v", f.Name, readErr, err)
		}
		contents[f.Name] = string(data)
	}
	if contents["overrides/config/example.toml"] != "theme = 'green'\n" || contents["overrides/shaderpacks/sample/shaders/main.fsh"] != "void main() {}\n" {
		t.Fatalf("selected override bytes differ: %+v", contents)
	}
	for name, content := range contents {
		if strings.Contains(name, "saves/") || strings.Contains(name, "credentials") || strings.Contains(content, "PRIVATE WORLD") || strings.Contains(content, "access_token") || name == "overrides/mods/example.jar" {
			t.Fatalf("sensitive or borrowed file leaked into %s", name)
		}
	}
}

func TestExportOverrideOnlyPackHasEmptyDownloadList(t *testing.T) {
	s, in, _, output := exportFixture(t)
	dir := filepath.Join(s.Root, "instances", in.ID)
	m, err := loadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	m.Files = map[string]BaseFile{}
	m.Optional = nil
	if err := saveManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	if err := s.Export(context.Background(), in, output, []string{"config/example.toml"}); err != nil {
		t.Fatal(err)
	}
	archive, err := Inspect(output)
	if err != nil {
		t.Fatal(err)
	}
	if archive.Index.Files == nil || len(archive.Index.Files) != 0 || len(archive.Overrides) != 1 {
		t.Fatalf("override-only archive has incorrect inventory: %+v", archive)
	}
}

func TestExportRejectsUninstalledOptionalDownloadInsteadOfDroppingIt(t *testing.T) {
	s, in, _, output := exportFixture(t)
	dir := filepath.Join(s.Root, "instances", in.ID)
	m, err := loadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	delete(m.Files, "mods/example.jar")
	m.Optional["mods/example.jar"] = false
	if err := saveManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	if err := s.Export(context.Background(), in, output, nil); err == nil || !strings.Contains(err.Error(), "unexportable optional download mods/example.jar") {
		t.Fatalf("missing optional download silently removed from exported inventory: %v", err)
	}
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatalf("partial export published: %v", err)
	}
}

func TestExportOverrideOnlyActualImportRoundTrip(t *testing.T) {
	s, in, _, output := exportFixture(t)
	dir := filepath.Join(s.Root, "instances", in.ID)
	m, err := loadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	m.Files = map[string]BaseFile{}
	m.Optional = nil
	if err := saveManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	if err := s.Export(context.Background(), in, output, []string{"config/example.toml", "options.txt"}); err != nil {
		t.Fatal(err)
	}
	archive, err := Inspect(output)
	if err != nil {
		t.Fatal(err)
	}
	s.Prepare = func(context.Context, string, string, string, string) error { return nil }
	imported, err := s.Import(context.Background(), archive, Source{Type: "local"}, "Reimported pack", 2048, nil, nil)
	if err != nil {
		t.Fatalf("actual import of exported archive failed: %v", err)
	}
	gameDir, err := instances.GameDir(s.Root, imported)
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{"config/example.toml": "theme = 'green'\n", "options.txt": "guiScale:2\n"} {
		got, err := os.ReadFile(filepath.Join(gameDir, filepath.FromSlash(path)))
		if err != nil || string(got) != want {
			t.Fatalf("reimported %s = %q, %v; want %q", path, got, err, want)
		}
	}
}

func TestExportRejectsUnexportableSourcesWithoutPublishing(t *testing.T) {
	cases := []struct {
		name string
		change func(*testing.T, Store, instances.Instance, string)
		want string
	}{
		{"modified downloaded bytes", func(t *testing.T, s Store, in instances.Instance, game string) {
			t.Helper()
			if err := os.WriteFile(filepath.Join(game, "mods", "example.jar"), []byte("changed by player"), 0600); err != nil { t.Fatal(err) }
		}, "mods/example.jar"},
		{"missing verified URL", func(t *testing.T, s Store, in instances.Instance, game string) {
			m, err := loadManifest(filepath.Join(s.Root, "instances", in.ID)); if err != nil { t.Fatal(err) }
			base := m.Files["mods/example.jar"]; base.URLs = nil; m.Files["mods/example.jar"] = base
			if err := saveManifest(filepath.Join(s.Root, "instances", in.ID), m); err != nil { t.Fatal(err) }
		}, "mods/example.jar"},
		{"nonpublication CDN", func(t *testing.T, s Store, in instances.Instance, game string) {
			m, err := loadManifest(filepath.Join(s.Root, "instances", in.ID)); if err != nil { t.Fatal(err) }
			base := m.Files["mods/example.jar"]; base.URLs = []string{"https://cdn.modrinth.net/data/ABCD/version.jar"}; m.Files["mods/example.jar"] = base
			if err := saveManifest(filepath.Join(s.Root, "instances", in.ID), m); err != nil { t.Fatal(err) }
		}, "mods/example.jar"},
		{"github mirror lacks publication permission", func(t *testing.T, s Store, in instances.Instance, game string) {
			m, err := loadManifest(filepath.Join(s.Root, "instances", in.ID))
			if err != nil {
				t.Fatal(err)
			}
			base := m.Files["mods/example.jar"]
			base.URLs = []string{"https://github.com/example/project/releases/download/v1/example.jar"}
			m.Files["mods/example.jar"] = base
			if err := saveManifest(filepath.Join(s.Root, "instances", in.ID), m); err != nil {
				t.Fatal(err)
			}
		}, "mods/example.jar"},
		{"sensitive borrowed download", func(t *testing.T, s Store, in instances.Instance, game string) {
			m, err := loadManifest(filepath.Join(s.Root, "instances", in.ID)); if err != nil { t.Fatal(err) }
			m.Files["saves/world/level.dat"] = m.Files["mods/example.jar"]
			if err := saveManifest(filepath.Join(s.Root, "instances", in.ID), m); err != nil { t.Fatal(err) }
		}, "saves/world/level.dat"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, in, gameDir, output := exportFixture(t)
			tc.change(t, s, in, gameDir)
			err := s.Export(context.Background(), in, output, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want per-file unexportable error for %s, got %v", tc.want, err)
			}
			if _, err := os.Lstat(output); !os.IsNotExist(err) {
				t.Fatalf("partial output published: %v", err)
			}
		})
	}
}

func TestExportKeepsProvenCDNOnlyAmongInstallMirrors(t *testing.T) {
	s, in, _, output := exportFixture(t)
	manifestDir := filepath.Join(s.Root, "instances", in.ID)
	m, err := loadManifest(manifestDir)
	if err != nil {
		t.Fatal(err)
	}
	base := m.Files["mods/example.jar"]
	base.URLs = append([]string{
		"https://github.com/example/project/releases/download/v1/example.jar",
		"https://cdn.modrinth.net/data/ABCD/versions/EFGH/example.jar",
	}, base.URLs...)
	m.Files["mods/example.jar"] = base
	if err := saveManifest(manifestDir, m); err != nil {
		t.Fatal(err)
	}
	if err := s.Export(context.Background(), in, output, nil); err != nil {
		t.Fatal(err)
	}
	archive, err := Inspect(output)
	if err != nil {
		t.Fatal(err)
	}
	got := archive.Index.Files[0].Downloads
	if len(got) != 1 || got[0] != "https://cdn.modrinth.com/data/ABCD/versions/EFGH/example.jar" {
		t.Fatalf("non-proven installation mirror published: %v", got)
	}
}

func TestExportRejectsUnsafeOverrideAndNeverReplacesDestination(t *testing.T) {
	cases := []struct{ name, path string; prepare func(*testing.T, string) }{
		{"world", "saves/world/level.dat", nil},
		{"secret path", "config/credentials.json", nil},
		{"borrowed JAR", "mods/example.jar", nil},
		{"binary zip", "resourcepacks/sample.zip", func(t *testing.T, game string) { t.Helper(); if err := os.WriteFile(filepath.Join(game, "resourcepacks", "sample.zip"), []byte("binary"), 0600); err != nil { t.Fatal(err) } }},
		{"secret content", "config/example.toml", func(t *testing.T, game string) { t.Helper(); if err := os.WriteFile(filepath.Join(game, "config", "example.toml"), []byte("access_token = 'private'"), 0600); err != nil { t.Fatal(err) } }},
		{"symlink", "config/example.toml", func(t *testing.T, game string) { t.Helper(); path := filepath.Join(game, "config", "example.toml"); if err := os.Remove(path); err != nil { t.Fatal(err) }; if err := os.Symlink(filepath.Join(game, "saves", "world", "level.dat"), path); err != nil { t.Fatal(err) } }},
		{"symlink parent", "config/example.toml", func(t *testing.T, game string) {
			t.Helper()
			parent := filepath.Join(game, "config")
			if err := os.Rename(parent, filepath.Join(game, "real-config")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(game, "real-config"), parent); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, in, gameDir, output := exportFixture(t)
			if tc.prepare != nil { tc.prepare(t, gameDir) }
			if err := s.Export(context.Background(), in, output, []string{tc.path}); err == nil {
				t.Fatalf("unsafe override accepted: %s", tc.path)
			}
			if _, err := os.Lstat(output); !os.IsNotExist(err) {
				t.Fatalf("partial output published: %v", err)
			}
		})
	}
	s, in, _, output := exportFixture(t)
	if err := os.WriteFile(output, []byte("leave existing file intact"), 0600); err != nil { t.Fatal(err) }
	if err := s.Export(context.Background(), in, output, nil); err == nil { t.Fatal("existing destination overwritten") }
	data, err := os.ReadFile(output)
	if err != nil || string(data) != "leave existing file intact" { t.Fatalf("destination changed: %q, %v", data, err) }
}
