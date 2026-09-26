package mods

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mcTUI/internal/operationlock"
)

func jar(t *testing.T, filename string, entries map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for name, body := range entries {
		f, err := w.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func store(t *testing.T) Store {
	t.Helper()
	root := t.TempDir()
	s := Store{filepath.Join(root, "minecraft"), filepath.Join(root, "mods.json")}
	if err := os.MkdirAll(filepath.Join(s.GameDir, "mods"), 0700); err != nil {
		t.Fatal(err)
	}
	return s
}

const fabricJSON = `{"id":"sample","name":"Sample","version":"1.0","environment":"client"}`

func TestInspect(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name    string
		entries map[string]string
		bad     bool
		id      string
	}{
		{"valid", map[string]string{"fabric.mod.json": fabricJSON}, false, "sample"},
		{"unknown", map[string]string{"META-INF/MANIFEST.MF": "test"}, false, ""},
		{"malformed", map[string]string{"fabric.mod.json": "{"}, true, ""},
		{"traversal", map[string]string{"../evil": "x"}, true, ""},
		{"absolute", map[string]string{"/evil": "x"}, true, ""},
		{"windows", map[string]string{"C:\\evil": "x"}, true, ""},
		{"metadataLimit", map[string]string{"fabric.mod.json": strings.Repeat("x", maxMetadata+1)}, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(dir, tc.name+".jar")
			jar(t, p, tc.entries)
			m, err := Inspect(p)
			if (err != nil) != tc.bad || m.ID != tc.id {
				t.Fatalf("%+v %v", m, err)
			}
		})
	}
	p := filepath.Join(dir, "crc.jar")
	b := jar(t, p, map[string]string{"asset": "UNIQUE-CONTENT"})
	at := bytes.Index(b, []byte("UNIQUE-CONTENT"))
	b[at] = 'X'
	os.WriteFile(p, b, 0600)
	if _, err := Inspect(p); err == nil {
		t.Fatal("CRC corruption accepted")
	}
	p = filepath.Join(dir, "symlink.jar")
	var b2 bytes.Buffer
	w := zip.NewWriter(&b2)
	h := &zip.FileHeader{Name: "link"}
	h.SetMode(os.ModeSymlink | 0777)
	f, _ := w.CreateHeader(h)
	f.Write([]byte("target"))
	w.Close()
	os.WriteFile(p, b2.Bytes(), 0600)
	if _, err := Inspect(p); err == nil {
		t.Fatal("ZIP symlink accepted")
	}
}
func TestLocalLifecycle(t *testing.T) {
	s := store(t)
	other := store(t)
	src := filepath.Join(t.TempDir(), "sample.jar")
	original := jar(t, src, map[string]string{"fabric.mod.json": fabricJSON})
	if err := s.Add(src); err != nil {
		t.Fatal(err)
	}
	items, err := s.Scan()
	if err != nil || len(items) != 1 || !items[0].Managed || items[0].Origin != "local" || items[0].SHA256 == "" || len(items[0].Hashes.SHA512) != 128 {
		t.Fatalf("%+v %v", items, err)
	}
	for i := 0; i < 4; i++ {
		if err = s.Toggle(items[0]); err != nil {
			t.Fatal(err)
		}
		items, err = s.Scan()
		if err != nil || len(items) != 1 || items[0].Changed {
			t.Fatalf("%+v %v", items, err)
		}
		got, err := os.ReadFile(filepath.Join(s.GameDir, "mods", items[0].File))
		if err != nil || !bytes.Equal(got, original) {
			t.Fatal("toggle changed content")
		}
	}
	if got, _ := other.Scan(); len(got) != 0 {
		t.Fatal("cross-instance mutation")
	}
	os.MkdirAll(filepath.Join(s.GameDir, "config"), 0700)
	config := filepath.Join(s.GameDir, "config", "keep.txt")
	os.WriteFile(config, []byte("keep"), 0600)
	if err = s.Remove(items[0], false); err == nil {
		t.Fatal("unconfirmed deletion")
	}
	if err = s.Remove(items[0], true); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Scan(); len(got) != 0 {
		t.Fatal("not removed")
	}
	if _, err = os.Stat(config); err != nil {
		t.Fatal("config removed")
	}
	if got, _ := os.ReadFile(src); !bytes.Equal(got, original) {
		t.Fatal("source modified")
	}
}
func TestScanAndChanges(t *testing.T) {
	s := store(t)
	dir := filepath.Join(s.GameDir, "mods")
	jar(t, filepath.Join(dir, "a.jar"), map[string]string{"fabric.mod.json": fabricJSON})
	jar(t, filepath.Join(dir, "b.jar.disabled"), map[string]string{"fabric.mod.json": fabricJSON})
	os.WriteFile(filepath.Join(dir, "c.jar"), []byte("broken"), 0600)
	jar(t, filepath.Join(dir, "d.jar"), nil)
	items, err := s.Scan()
	if err != nil || len(items) != 4 {
		t.Fatalf("%+v %v", items, err)
	}
	if items[0].Managed || !items[0].Duplicate || !items[1].Duplicate || items[1].Enabled || items[2].Problem == "" || items[3].Metadata.ID != "" || items[3].Problem != "" {
		t.Fatalf("%+v", items)
	}
	old := items[0]
	jar(t, filepath.Join(dir, "a.jar"), map[string]string{"asset": "changed"})
	if err = s.Toggle(old); err == nil {
		t.Fatal("stale hash accepted")
	}
	src := filepath.Join(t.TempDir(), "managed.jar")
	jar(t, src, map[string]string{"fabric.mod.json": fabricJSON})
	if err = s.Add(src); err != nil {
		t.Fatal(err)
	}
	items, _ = s.Scan()
	managed := items[len(items)-1]
	future := time.Now().Add(time.Hour)
	p := filepath.Join(dir, "managed.jar")
	if err = os.Chtimes(p, future, future); err != nil {
		t.Fatal(err)
	}
	if err = s.Remove(managed, true); err == nil {
		t.Fatal("stale mtime accepted")
	}
	items, _ = s.Scan()
	managed = items[len(items)-1]
	if !managed.Changed {
		t.Fatal("external mtime not marked")
	}
	if err = s.Toggle(managed); err == nil {
		t.Fatal("external managed change accepted")
	}
}
func TestCollisionsLockAndRegistry(t *testing.T) {
	s := store(t)
	src := filepath.Join(t.TempDir(), "mod.jar")
	jar(t, src, nil)
	release, err := operationlock.Acquire(s.GameDir)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Add(src); err == nil || !strings.Contains(err.Error(), "instance busy") {
		t.Fatalf("lock: %v", err)
	}
	release()
	if err = s.Add(src); err != nil {
		t.Fatal(err)
	}
	if err = s.Add(src); err == nil {
		t.Fatal("collision accepted")
	}
	items, _ := s.Scan()
	release, err = operationlock.Acquire(s.GameDir)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Toggle(items[0]); err == nil {
		t.Fatal("toggle ignored lock")
	}
	if err = s.Remove(items[0], true); err == nil {
		t.Fatal("remove ignored lock")
	}
	release()
	collision := filepath.Join(s.GameDir, "mods", "MOD.jar.disabled")
	os.WriteFile(collision, []byte("keep"), 0600)
	if err = s.Toggle(items[0]); err == nil {
		t.Fatal("case collision accepted")
	}
	if b, _ := os.ReadFile(collision); string(b) != "keep" {
		t.Fatal("collision overwritten")
	}
	os.WriteFile(s.Registry, []byte(`{"schema_version":99}`), 0600)
	if err = s.Remove(items[0], true); err == nil {
		t.Fatal("bad registry accepted")
	}
	if _, err = os.Stat(filepath.Join(s.GameDir, "mods", "mod.jar")); err != nil {
		t.Fatal("file removed on registry error")
	}
}
func TestRejectSymlinksAndUnsafeNames(t *testing.T) {
	for _, n := range []string{"../mod.jar", "C:mod.jar", "CON.jar", "x\\mod.jar", ".hidden.jar", "mod.jar\n"} {
		if validName(n) {
			t.Fatalf("accepted %q", n)
		}
	}
	s := store(t)
	src := filepath.Join(t.TempDir(), "source.jar")
	jar(t, src, nil)
	link := filepath.Join(s.GameDir, "mods", "link.jar")
	if err := os.Symlink(src, link); err != nil {
		t.Skip(err)
	}
	items, err := s.Scan()
	if err != nil || len(items) != 1 || items[0].Problem == "" {
		t.Fatalf("%+v %v", items, err)
	}
	if err = s.Remove(items[0], true); err == nil {
		t.Fatal("symlink removed")
	}
	if err = s.Add(link); err == nil {
		t.Fatal("symlink imported")
	}
}
func TestDeclaredZIPLimitAndInvalidImport(t *testing.T) {
	s := store(t)
	source := filepath.Join(t.TempDir(), "oversized.jar")
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	_, err := w.CreateRaw(&zip.FileHeader{Name: "large", Method: zip.Store, UncompressedSize64: maxEntry + 1})
	if err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(source, b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Inspect(source); err == nil {
		t.Fatal("oversized entry accepted")
	}
	if err = s.Add(source); err == nil {
		t.Fatal("invalid JAR installed")
	}
	if entries, _ := os.ReadDir(filepath.Join(s.GameDir, "mods")); len(entries) != 0 {
		t.Fatal("failed import left a file")
	}
	if err = os.Mkdir(s.Registry, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Scan(); err == nil {
		t.Fatal("directory registry accepted")
	}
}

func TestRequiredDependencyCannotBeRemoved(t *testing.T) {
	s := store(t)
	for _, name := range []string{"library.jar", "consumer.jar"} {
		p := filepath.Join(t.TempDir(), name)
		jar(t, p, nil)
		if err := s.Add(p); err != nil {
			t.Fatal(err)
		}
	}
	r, err := s.load()
	if err != nil {
		t.Fatal(err)
	}
	for i := range r.Mods {
		if r.Mods[i].File == "library.jar" {
			r.Mods[i].ProjectID = "library"
			r.Mods[i].RequiredBy = []string{"consumer"}
		}
		if r.Mods[i].File == "consumer.jar" {
			r.Mods[i].ProjectID = "consumer"
		}
	}
	if err := s.save(r); err != nil {
		t.Fatal(err)
	}
	items, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.File == "library.jar" {
			if err := s.Remove(item, true); err == nil || !strings.Contains(err.Error(), "required by") {
				t.Fatalf("dependency removal allowed: %v", err)
			}
			return
		}
	}
	t.Fatal("library not found")
}

func TestRegistryPersistsVersionedRecords(t *testing.T) {
	s := store(t)
	src := filepath.Join(t.TempDir(), "mod.jar")
	jar(t, src, nil)
	if err := s.Add(src); err != nil {
		t.Fatal(err)
	}
	var r registry
	b, _ := os.ReadFile(s.Registry)
	if err := json.Unmarshal(b, &r); err != nil || r.SchemaVersion != 2 || len(r.Mods) != 1 {
		t.Fatalf("%+v %v", r, err)
	}
}
