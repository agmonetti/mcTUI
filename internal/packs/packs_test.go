package packs

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func makeGame(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return dir
}

type testEntry struct {
	name, text string
	mode       os.FileMode
}

func makeZIP(t *testing.T, name string, entries ...testEntry) string {
	t.Helper()
	f, err := os.Create(filepath.Join(t.TempDir(), name))
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	for _, entry := range entries {
		h, err := zip.FileInfoHeader(fakeInfo{entry.name, entry.mode})
		if err != nil {
			t.Fatal(err)
		}
		h.Name = entry.name
		w, err := z.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(entry.text)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

type fakeInfo struct {
	name string
	mode os.FileMode
}

func (f fakeInfo) Name() string           { return f.name }
func (f fakeInfo) Size() int64            { return 0 }
func (f fakeInfo) Mode() os.FileMode      { return f.mode }
func (f fakeInfo) ModTime() (t time.Time) { return }
func (f fakeInfo) IsDir() bool            { return f.mode.IsDir() }
func (f fakeInfo) Sys() any               { return nil }

var resourceEntries = []testEntry{
	{"pack.mcmeta", `{"pack":{"pack_format":34,"description":"Test pack"}}`, 0644},
	{"assets/minecraft/lang/en_us.json", `{}`, 0644},
}

func TestIsolatedInstancesAndImmutableInstall(t *testing.T) {
	first, second := Store{makeGame(t)}, Store{makeGame(t)}
	source := makeZIP(t, "pack.zip", resourceEntries...)
	if err := first.Install(Resource, source, "hello.zip"); err != nil {
		t.Fatal(err)
	}
	packs, err := first.List(Resource)
	if err != nil || len(packs) != 1 || packs[0].File != "hello.zip" || packs[0].Format != 34 || packs[0].Problem != "" {
		t.Fatalf("first instance: %+v, %v", packs, err)
	}
	other, err := second.List(Resource)
	if err != nil || len(other) != 0 {
		t.Fatalf("second instance: %+v, %v", other, err)
	}
	if err := first.Install(Resource, source, "hello.zip"); err == nil {
		t.Fatal("silent overwrite")
	}
	if err := second.Add(Resource, source); err != nil {
		t.Fatal(err)
	}
	if err := first.Remove(Resource, "hello.zip"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("staged source was modified: %v", err)
	}
	other, err = second.List(Resource)
	if err != nil || len(other) != 1 || other[0].Problem != "" {
		t.Fatalf("other instance changed: %+v, %v", other, err)
	}
}

func TestModernResourceFormatRange(t *testing.T) {
	s := Store{makeGame(t)}
	source := makeZIP(t, "modern.zip",
		testEntry{"pack.mcmeta", `{"pack":{"min_format":[65,0],"max_format":65,"description":"Modern resource pack"}}`, 0644},
		resourceEntries[1],
	)
	if err := s.Install(Resource, source, "modern.zip"); err != nil {
		t.Fatal(err)
	}
	packs, err := s.List(Resource)
	if err != nil || len(packs) != 1 || packs[0].Problem != "" || packs[0].Format != 0 ||
		packs[0].FormatRange != "65.0–65" || packs[0].Description != "Modern resource pack" {
		t.Fatalf("modern pack format incorrectly classified: %+v, %v", packs, err)
	}
}

func TestSupportedFormatsObjectAndJSONDescription(t *testing.T) {
	s := Store{makeGame(t)}
	// Case 1: Low On Fire style object format
	source := makeZIP(t, "lowonfire.zip",
		testEntry{"pack.mcmeta", `{"pack":{"pack_format":15,"min_format":15,"max_format":200,"supported_formats":{"min_inclusive":15,"max_inclusive":200},"description":{"fallback":"Low on fire"}}}`, 0644},
		resourceEntries[1],
	)
	if err := s.Install(Resource, source, "lowonfire.zip"); err != nil {
		t.Fatalf("install with supported_formats object failed: %v", err)
	}
	packs, err := s.List(Resource)
	if err != nil || len(packs) != 1 || packs[0].Problem != "" || packs[0].FormatRange != "15–200" || packs[0].Description != "Low on fire" {
		t.Fatalf("unexpected pack parse result: %+v, %v", packs, err)
	}

	// Case 2: Array supported_formats and list description
	source2 := makeZIP(t, "betterleaves.zip",
		testEntry{"pack.mcmeta", `{"pack":{"pack_format":15,"supported_formats":[15,255],"description":[{"text":"Better "},{"text":"Leaves"}]}}`, 0644},
		resourceEntries[1],
	)
	if err := s.Install(Resource, source2, "betterleaves.zip"); err != nil {
		t.Fatalf("install with supported_formats array failed: %v", err)
	}
}

func TestRejectsUnsafeArchiveAndLeavesNoPublishedPack(t *testing.T) {
	cases := []struct {
		name    string
		entries []testEntry
		kind    Kind
	}{
		{"traversal", append(slices.Clone(resourceEntries), testEntry{"../outside.txt", "bad", 0644}), Resource},
		{"duplicate", append(slices.Clone(resourceEntries), testEntry{"PACK.MCMETA", "other", 0644}), Resource},
		{"symlink", append(slices.Clone(resourceEntries), testEntry{"assets/link", "outside", os.ModeSymlink | 0777}), Resource},
		{"missing assets", []testEntry{resourceEntries[0]}, Resource},
		{"invalid metadata", []testEntry{{"pack.mcmeta", `{"pack":{"pack_format":"bad"}}`, 0644}, resourceEntries[1]}, Resource},
		{"inverted modern range", []testEntry{{"pack.mcmeta", `{"pack":{"min_format":[65,3],"max_format":[65,2]}}`, 0644}, resourceEntries[1]}, Resource},
		{"missing shaders", []testEntry{{"not-shaders/x", "x", 0644}}, Shader},
		{"file parent", []testEntry{{"shaders", "x", 0644}, {"shaders/a", "x", 0644}}, Shader},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := Store{makeGame(t)}
			source := makeZIP(t, "bad.zip", tc.entries...)
			if err := s.Install(tc.kind, source, "bad.zip"); err == nil {
				t.Fatal("invalid archive was installed")
			}
			packdir, _ := packDir(tc.kind)
			if _, err := os.Lstat(filepath.Join(s.GameDir, packdir, "bad.zip")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("archive published on failure: %v", err)
			}
			if _, err := os.Stat(source); err != nil {
				t.Fatalf("source lost on failure: %v", err)
			}
		})
	}
}

func TestRejectsSymlinkInUnpackedFolder(t *testing.T) {
	s := Store{makeGame(t)}
	source := filepath.Join(t.TempDir(), "unsafe")
	if err := os.MkdirAll(filepath.Join(source, "shaders"), 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(source, "shaders", "entry.vsh")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := s.Add(Shader, source); err == nil {
		t.Fatal("symlink in unpacked pack accepted")
	}
	if _, err := os.Lstat(filepath.Join(s.GameDir, "shaderpacks", "unsafe")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsafe pack published: %v", err)
	}
	data, err := os.ReadFile(outside)
	if err != nil || string(data) != "untouched" {
		t.Fatalf("outside file changed: %q, %v", data, err)
	}
}

func TestUnpackedFolderAndRemovalDoNotTouchSelection(t *testing.T) {
	s := Store{makeGame(t)}
	if err := os.WriteFile(filepath.Join(s.GameDir, "options.txt"), []byte("resourcePacks:[\"vanilla\"]"), 0600); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "user-pack")
	if err := os.MkdirAll(filepath.Join(source, "shaders"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "shaders", "gbuffers_terrain.vsh"), []byte("shader"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(Shader, source); err != nil {
		t.Fatal(err)
	}
	packs, err := s.List(Shader)
	if err != nil || len(packs) != 1 || packs[0].File != "user-pack" || !packs[0].Directory || packs[0].Problem != "" {
		t.Fatalf("folder import: %+v, %v", packs, err)
	}
	if err := s.Remove(Shader, "../options.txt"); err == nil {
		t.Fatal("path traversal removal succeeded")
	}
	if err := s.Remove(Shader, "user-pack"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(s.GameDir, "options.txt"))
	if err != nil || !strings.Contains(string(b), `resourcePacks:["vanilla"]`) {
		t.Fatalf("selection changed: %q, %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(source, "shaders", "gbuffers_terrain.vsh")); err != nil {
		t.Fatalf("source pack changed: %v", err)
	}
}
