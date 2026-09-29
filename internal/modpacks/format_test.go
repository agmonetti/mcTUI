package modpacks

import (
	"archive/zip"
	"bytes"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type zipEntry struct {
	name string
	data string
	mode os.FileMode
}

func makePack(t *testing.T, index []byte, entries ...zipEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.mrpack")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	all := append([]zipEntry{{name: "modrinth.index.json", data: string(index)}}, entries...)
	for _, e := range all {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		if e.mode != 0 {
			h.SetMode(e.mode)
		} else if strings.HasSuffix(e.name, "/") {
			h.SetMode(os.ModeDir | 0755)
		} else {
			h.SetMode(0644)
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(e.data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func testIndex(t *testing.T) Index {
	t.Helper()
	return Index{
		FormatVersion: 1, Game: "minecraft", VersionID: "1.0", Name: "Pack",
		Dependencies: map[string]string{"minecraft": "1.21.1", "fabric-loader": "0.16.14"},
		Files: []File{{Path: "mods/A.jar", Hashes: Hashes{SHA1: strings.Repeat("a", 40), SHA512: strings.Repeat("b", 128)}, Downloads: []string{"https://cdn.modrinth.com/data/A"}, FileSize: 0}},
	}
}

func indexJSON(t *testing.T, index Index) []byte {
	t.Helper()
	data, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestInspectFabricLayersAndClientEnvironments(t *testing.T) {
	index := testIndex(t)
	index.Files = append(index.Files,
		File{Path: "mods/optional.jar", Hashes: index.Files[0].Hashes, Downloads: index.Files[0].Downloads, FileSize: 17, Env: &Environment{Client: "optional", Server: "unsupported"}},
		File{Path: "mods/server-only.jar", Hashes: index.Files[0].Hashes, Downloads: index.Files[0].Downloads, FileSize: 3, Env: &Environment{Client: "unsupported", Server: "required"}},
	)
	p := makePack(t, indexJSON(t, index),
		zipEntry{name: "overrides/config/", mode: os.ModeDir | 0755},
		zipEntry{name: "overrides/config/settings.toml", data: "original"},
		zipEntry{name: "overrides/mods/A.jar", data: "override takes precedence"},
		zipEntry{name: "client-overrides/config/settings.toml", data: "client"},
		zipEntry{name: "server-overrides/config/settings.toml", data: "server"},
		zipEntry{name: "server-overrides/mods/server.jar", data: "not installed"},
	)
	archive, err := Inspect(p)
	if err != nil {
		t.Fatal(err)
	}
	if archive.Path != p || len(archive.Index.Files) != 3 || len(archive.Overrides) != 3 {
		t.Fatalf("lost index or included server overrides: %+v", archive)
	}
	if archive.Index.Files[1].Env.Client != "optional" || archive.Index.Files[2].Env.Client != "unsupported" {
		t.Fatalf("client environment selections lost: %+v", archive.Index.Files)
	}
	if archive.Overrides[1] != (Override{Path: "mods/A.jar", ZipName: "overrides/mods/A.jar", Layer: "overrides", Size: 25}) || archive.Overrides[2].Layer != "client-overrides" || archive.Overrides[2].Path != archive.Overrides[0].Path {
		t.Fatalf("incorrect layering/override source: %+v", archive.Overrides)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha512.Sum512(data)
	if archive.SHA512 != hex.EncodeToString(digest[:]) {
		t.Fatalf("archive digest does not protect ZIP bytes: %q", archive.SHA512)
	}
}

func TestInspectVanillaAndEmptyFile(t *testing.T) {
	idx := testIndex(t)
	idx.Dependencies = map[string]string{"minecraft": "1.20.4"}
	idx.Files = idx.Files[:1]
	got, err := Inspect(makePack(t, indexJSON(t, idx)))
	if err != nil || got.Index.Files[0].FileSize != 0 || len(got.Overrides) != 0 {
		t.Fatalf("valid Vanilla/empty file: %+v, %v", got, err)
	}
}

func TestValidPathRejectsTraversalAndWindowsAliases(t *testing.T) {
	for _, p := range []string{"", "/etc/passwd", `C:\drive`, `C:/drive`, `\\host\share`, "../evil", "a/../b", "a/./b", "a//b", "a/", "AUX.txt", "COM1.jar", "lpt9.cfg", "CON.aux", "a. ", "a/b ", "a\nb", "a\x00b", "a:b", "a?b", ".mctui-operation.lock", "config/.mctui-hidden", "mctui-backups/x", "Pack.json", "mctui_latest.log"} {
		if err := ValidPath(p); err == nil {
			t.Errorf("accepted unsafe path %q", p)
		}
	}
	for _, p := range []string{"mods/a.jar", "resourcepacks/élève.zip", "config/file name.txt", "options.txt"} {
		if err := ValidPath(p); err != nil {
			t.Errorf("rejected safe path %q: %v", p, err)
		}
	}
}

func TestInspectRejectsUnsafeZIP(t *testing.T) {
	idx := indexJSON(t, testIndex(t))
	cases := []struct {
		name string
		entry zipEntry
	}{
		{"traversal", zipEntry{name: "overrides/../escape"}},
		{"UNC path", zipEntry{name: `\\host\share`}},
		{"symlink", zipEntry{name: "overrides/link", data: "../../escape", mode: os.ModeSymlink | 0777}},
		{"special file", zipEntry{name: "overrides/pipe", mode: os.ModeNamedPipe | 0600}},
		{"unexpected root", zipEntry{name: "surprise.jar"}},
		{"root file without layer", zipEntry{name: "overrides", data: "not a directory"}},
		{"oversized index", zipEntry{name: "overrides/file", data: "okay"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := idx
			if tc.name == "oversized index" {
				data = bytes.Repeat([]byte(" "), int(maxIndexSize)+1)
			}
			if _, err := Inspect(makePack(t, data, tc.entry)); err == nil {
				t.Fatal("unsafe ZIP accepted")
			}
		})
	}
}

func TestInspectRejectsPathCollisions(t *testing.T) {
	idx := testIndex(t)
	for _, tc := range []struct {
		name    string
		files   []File
		entries []zipEntry
	}{
		{"download case fold", []File{{Path: "mods/a.JAR", Hashes: idx.Files[0].Hashes, Downloads: idx.Files[0].Downloads, FileSize: 1}}, nil},
		{"download ancestor", []File{{Path: "mods", Hashes: idx.Files[0].Hashes, Downloads: idx.Files[0].Downloads, FileSize: 1}}, nil},
		{"ZIP case fold", nil, []zipEntry{{name: "overrides/Config/A"}, {name: "overrides/config/B"}}},
		{"ZIP duplicate", nil, []zipEntry{{name: "overrides/config/A"}, {name: "overrides/config/A"}}},
		{"layers case fold", nil, []zipEntry{{name: "overrides/config/A"}, {name: "client-overrides/config/a"}}},
		{"layers file directory", nil, []zipEntry{{name: "overrides/config"}, {name: "client-overrides/config/A"}}},
		{"download overridden case fold", nil, []zipEntry{{name: "overrides/mods/a.jar"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pack := idx
			pack.Files = append(append([]File{}, idx.Files...), tc.files...)
			if _, err := Inspect(makePack(t, indexJSON(t, pack), tc.entries...)); err == nil {
				t.Fatal("colliding destinations accepted")
			}
		})
	}
}

func TestInspectRejectsUnsupportedAndIncompleteIndex(t *testing.T) {
	base := testIndex(t)
	for _, tc := range []struct {
		name   string
		mutate func(*Index)
	}{
		{"forge", func(i *Index) { i.Dependencies["forge"] = "45.0" }},
		{"quilt", func(i *Index) { i.Dependencies["quilt-loader"] = "0.26.0" }},
		{"unknown loader", func(i *Index) { i.Dependencies["future-loader"] = "1" }},
		{"missing minecraft", func(i *Index) { delete(i.Dependencies, "minecraft") }},
		{"invalid Fabric version", func(i *Index) { i.Dependencies["fabric-loader"] = "latest" + string(rune(10)) }},
		{"wrong game", func(i *Index) { i.Game = "other" }},
		{"wrong format", func(i *Index) { i.FormatVersion = 2 }},
		{"missing SHA1", func(i *Index) { i.Files[0].Hashes.SHA1 = "" }},
		{"bad SHA512", func(i *Index) { i.Files[0].Hashes.SHA512 = strings.Repeat("x", 128) }},
		{"negative size", func(i *Index) { i.Files[0].FileSize = -1 }},
		{"over budget", func(i *Index) { i.Files[0].FileSize = maxDownloads + 1 }},
		{"optional invalid", func(i *Index) { i.Files[0].Env = &Environment{Client: "maybe", Server: "required"} }},
		{"HTTP download", func(i *Index) { i.Files[0].Downloads = []string{"http://cdn.modrinth.com/a"} }},
		{"credentialed URL", func(i *Index) { i.Files[0].Downloads = []string{"https://user:pass@cdn.modrinth.com/a"} }},
		{"fragment URL", func(i *Index) { i.Files[0].Downloads = []string{"https://cdn.modrinth.com/a#secret"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idx := base
			idx.Dependencies = make(map[string]string, len(base.Dependencies))
			for k, v := range base.Dependencies {
				idx.Dependencies[k] = v
			}
			idx.Files = append([]File{}, base.Files...)
			tc.mutate(&idx)
			if _, err := Inspect(makePack(t, indexJSON(t, idx))); err == nil {
				t.Fatal("invalid index accepted")
			}
		})
	}
	for _, raw := range []string{
		`{"formatVersion":1,"game":"minecraft","name":"Pack","versionId":"1","dependencies":{"minecraft":"1.21"},"files":[{"path":"mods/a.jar","hashes":{"sha1":"` + strings.Repeat("a", 40) + `","sha512":"` + strings.Repeat("b", 128) + `"},"downloads":["https://cdn.modrinth.com/a"]}]}`,
		`{"formatVersion":1,"game":"minecraft","name":"Pack","versionId":"1","dependencies":{"minecraft":"1.21","minecraft":"1.20"},"files":[]}`,
	} {
		if _, err := Inspect(makePack(t, []byte(raw))); err == nil {
			t.Fatal("missing fileSize or duplicate JSON key accepted")
		}
	}
}

func TestInspectRejectsBadCRCIncludingServerOverride(t *testing.T) {
	p := makePack(t, indexJSON(t, testIndex(t)), zipEntry{name: "server-overrides/config/private.txt", data: "DO-NOT-CORRUPT-ME"})
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	// Deflate payload cannot be reliably located by the uncompressed string. Flip
	// the expected CRC in the central directory instead.
	needle := []byte("server-overrides/config/private.txt")
	pos := bytes.LastIndex(data, needle)
	if pos < 46 || !bytes.Equal(data[pos-46:pos-42], []byte{'P', 'K', 1, 2}) {
		t.Fatal("test ZIP central directory missing")
	}
	data[pos-30] ^= 0xff // CRC field begins 16 bytes after central directory header.
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(p); err == nil {
		t.Fatal("corrupt omitted server entry accepted")
	}
}
