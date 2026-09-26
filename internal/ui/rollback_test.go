package ui

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"mcTUI/internal/instances"
	"mcTUI/internal/mods"
)

func TestRollbackReviewCancelAndRestore(t *testing.T) {
	root := t.TempDir()
	in, err := instances.Create(root, "Rollback A", "1.20.4", "Fabric", "0.16.0", 2048)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := instances.GameDir(root, in)
	if err != nil {
		t.Fatal(err)
	}
	modDir := filepath.Join(dir, "mods")
	if err := os.MkdirAll(modDir, 0700); err != nil {
		t.Fatal(err)
	}
	backupDir := filepath.Join(root, "instances", in.ID, "backups", "mods_replaced")
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		t.Fatal(err)
	}
	makeJar := func(path, version string) mods.Record {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		z := zip.NewWriter(f)
		w, err := z.Create("fabric.mod.json")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(`{"id":"sample","version":"` + version + `","name":"Sample"}`)); err != nil {
			t.Fatal(err)
		}
		if err := z.Close(); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(b)
		return mods.Record{File: filepath.Base(path), SHA256: hex.EncodeToString(h[:]), Enabled: true, Origin: "modrinth", ProjectID: "sample-project", VersionID: version}
	}
	old := makeJar(filepath.Join(backupDir, "saved-old.jar"), "old")
	old.File = "old.jar"
	current := makeJar(filepath.Join(modDir, "new.jar"), "new")
	entry := mods.HistoryEntry{Date: "2026-09-26", ProjectID: "sample-project", Previous: old, New: current, Backup: "saved-old.jar"}
	registry := filepath.Join(root, "instances", in.ID, "mods.json")
	for path, value := range map[string]any{
		registry: struct {
			SchemaVersion int           `json:"schema_version"`
			Mods          []mods.Record `json:"mods"`
		}{2, []mods.Record{current}},
		filepath.Join(backupDir, "history.json"): []mods.HistoryEntry{entry},
	} {
		b, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	m := Model{state: menuScreen, dataRoot: root, instance: in, width: 120, height: 32}
	updated, cmd := m.openMods()
	m = updated.(Model)
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.modErr != nil || len(m.modItems) != 1 {
		t.Fatalf("scan: %v %+v", m.modErr, m.modItems)
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	m = updated.(Model)
	if m.state != modsWorkingScreen || cmd == nil {
		t.Fatal("history not loaded asynchronously")
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.state != modsRollbackScreen || !strings.Contains(m.View(), "old.jar") || !strings.Contains(m.View(), "new.jar") || !strings.Contains(m.View(), "saved-old.jar") {
		t.Fatalf("missing review: %s", m.View())
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if cmd != nil || m.state != modsScreen {
		t.Fatal("cancel did not return to list")
	}
	if _, err := os.Stat(filepath.Join(modDir, "new.jar")); err != nil {
		t.Fatal("cancel mutated current JAR", err)
	}
	updated, cmd = m.updateModsKey("b")
	m = updated.(Model)
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	original, err := os.ReadFile(filepath.Join(modDir, "new.jar"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modDir, "new.jar"), []byte("changed externally"), 0600); err != nil {
		t.Fatal(err)
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = updated.(Model)
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.modErr == nil || !strings.Contains(m.modErr.Error(), "changed") {
		t.Fatalf("external edit was overwritten: %v", m.modErr)
	}
	if b, err := os.ReadFile(filepath.Join(modDir, "new.jar")); err != nil || string(b) != "changed externally" {
		t.Fatalf("external edit lost: %v", err)
	}
	if err := os.WriteFile(filepath.Join(modDir, "new.jar"), original, 0600); err != nil {
		t.Fatal(err)
	}
	updated, cmd = m.updateModsKey("b")
	m = updated.(Model)
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = updated.(Model)
	if m.state != modsWorkingScreen || cmd == nil {
		t.Fatal("confirmation did not start rollback")
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.state != modsScreen || m.modErr != nil || len(m.modItems) != 1 || m.modItems[0].VersionID != "old" {
		t.Fatalf("rollback failed: %v %+v", m.modErr, m.modItems)
	}
	if _, err := os.Stat(filepath.Join(modDir, "old.jar")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(modDir, "new.jar")); !os.IsNotExist(err) {
		t.Fatal("new JAR still present", err)
	}
	updated, cmd = m.updateModsKey("b")
	m = updated.(Model)
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.state != modsScreen || m.modErr == nil {
		t.Fatal("missing backup not reported")
	}
}
