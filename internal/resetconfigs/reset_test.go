package resetconfigs

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func makeFolder(t *testing.T, dir, name string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "example.jar"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestReset(t *testing.T) {
	for _, mode := range []Mode{Backup, Purge} {
		t.Run(string(mode), func(t *testing.T) {
			dir := t.TempDir()
			makeFolder(t, dir, "mods")
			makeFolder(t, dir, "config")
			makeFolder(t, dir, "saves")
			result, err := runAt(dir, mode)
			if err != nil || len(result.Errors) != 0 || result.NothingToReset {
				t.Fatalf("unexpected result: %+v, %v", result, err)
			}
			for _, folder := range result.Folders {
				if _, err := os.Lstat(folder.Path); !os.IsNotExist(err) {
					t.Fatalf("source remains: %s: %v", folder.Path, err)
				}
				want := Deleted
				if mode == Backup {
					want = Moved
					if _, err := os.Stat(filepath.Join(result.BackupPath, folder.Name, "example.jar")); err != nil {
						t.Fatal(err)
					}
				}
				if folder.Status != want {
					t.Fatalf("%s: %s, want %s", folder.Name, folder.Status, want)
				}
			}
			if _, err := os.Stat(filepath.Join(dir, "saves", "example.jar")); err != nil {
				t.Fatal("world was modified:", err)
			}
			again, err := runAt(dir, mode)
			if err != nil || !again.NothingToReset || again.BackupPath != "" {
				t.Fatalf("empty reset: %+v, %v", again, err)
			}
		})
	}
}

func TestBackupNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	makeFolder(t, dir, "mods")
	first, err := runAt(dir, Backup)
	if err != nil {
		t.Fatal(err)
	}
	makeFolder(t, dir, "mods")
	second, err := runAt(dir, Backup)
	if err != nil || first.BackupPath == second.BackupPath {
		t.Fatalf("backup collision: %+v %+v %v", first, second, err)
	}
	if _, err := os.Stat(filepath.Join(first.BackupPath, "mods", "example.jar")); err != nil {
		t.Fatal(err)
	}
}

func TestRejectUnsafePaths(t *testing.T) {
	dir := t.TempDir()
	if _, err := runAt("relative/path", Purge); err == nil {
		t.Fatal("relative path accepted")
	}
	if _, err := runAt(filepath.Join(dir, "missing"), Purge); err == nil {
		t.Fatal("missing installation accepted")
	}
	makeFolder(t, dir, "saves")
	if err := os.Symlink(filepath.Join(dir, "saves"), filepath.Join(dir, "mods")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlinks unavailable: %v", err)
		}
		t.Fatal(err)
	}
	if _, err := runAt(dir, Purge); err == nil {
		t.Fatal("linked mods accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "saves", "example.jar")); err != nil {
		t.Fatal("linked world affected:", err)
	}
	if err := os.Remove(filepath.Join(dir, "mods")); err != nil {
		t.Fatal(err)
	}
	makeFolder(t, dir, "mods")
	if err := os.Symlink(filepath.Join(dir, "saves"), filepath.Join(dir, "mctui-backups")); err != nil {
		t.Fatal(err)
	}
	if _, err := runAt(dir, Backup); err == nil {
		t.Fatal("linked backups accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "mods", "example.jar")); err != nil {
		t.Fatal("mods affected:", err)
	}
}

func TestPartialPurgeStops(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires Unix directory permissions and unprivileged user")
	}
	dir := t.TempDir()
	makeFolder(t, dir, "mods")
	makeFolder(t, dir, "config")
	locked := filepath.Join(dir, "mods", "locked")
	if err := os.Mkdir(locked, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "file"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0500); err != nil {
		t.Fatal(err)
	}
	// Restore permissions so TempDir cleanup works after the attempted purge.
	t.Cleanup(func() { _ = os.Chmod(locked, 0700) })
	result, err := runAt(dir, Purge)
	if err != nil || len(result.Errors) == 0 || result.Folders[0].Status != PartiallyDeleted || result.Folders[1].Status != Untouched {
		t.Fatalf("partial purge: %+v, %v", result, err)
	}
	if !strings.Contains(strings.Join(result.Folders[0].Removed, ","), "example.jar") {
		t.Fatalf("missing removed-file report: %+v", result.Folders[0])
	}
	if _, err := os.Stat(filepath.Join(dir, "config", "example.jar")); err != nil {
		t.Fatal("next folder affected:", err)
	}
}
