package instances

import (
	"os"
	"path/filepath"
	"testing"

	"mcTUI/internal/operationlock"
)

func TestRetireRestoreAndPermanentDelete(t *testing.T) {
	root := t.TempDir()
	in, err := Create(root, "Disposable", "1.21.1", "Vanilla", "", 2048)
	if err != nil {
		t.Fatal(err)
	}
	game, err := GameDir(root, in)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(game, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(game, "options.txt"), []byte("mine"), 0600); err != nil {
		t.Fatal(err)
	}
	backup, err := BackupDir(root, in)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(backup, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backup, "saved"), []byte("backup"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Select(root, in.ID); err != nil {
		t.Fatal(err)
	}
	release, err := operationlock.Acquire(game)
	if err != nil {
		t.Fatal(err)
	}
	if err := Retire(root, in.ID); err == nil {
		t.Fatal("retired a busy instance")
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	if err := Retire(root, in.ID); err != nil {
		t.Fatal(err)
	}
	if selected, err := Selected(root); err != nil || selected.ID != LegacyID {
		t.Fatalf("selection after retire = %+v, %v", selected, err)
	}
	if listed, err := List(root); err != nil || len(listed) != 1 {
		t.Fatalf("retired entry remained in picker: %+v, %v", listed, err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "retired", in.ID, "minecraft", "options.txt")); err != nil || string(data) != "mine" {
		t.Fatalf("retired data = %q, %v", data, err)
	}
	if err := RestoreRetired(root, in.ID); err != nil {
		t.Fatal(err)
	}
	if restored, err := Read(root, in.ID); err != nil || restored.ID != in.ID {
		t.Fatalf("restore = %+v, %v", restored, err)
	}
	if err := Retire(root, in.ID); err != nil {
		t.Fatal(err)
	}
	if err := PurgeRetired(root, in.ID); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(root, "retired", in.ID), backup} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("permanently deleted path still exists: %s: %v", path, err)
		}
	}
	if err := Retire(root, LegacyID); err == nil {
		t.Fatal("retired legacy installation")
	}
}
