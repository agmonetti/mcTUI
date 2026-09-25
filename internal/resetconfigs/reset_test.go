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

func TestCustomBackupDirectoryAndHistory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".minecraft")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(t.TempDir(), "my backups")
	if err := os.Mkdir(base, 0700); err != nil {
		t.Fatal(err)
	}
	makeFolder(t, dir, "mods")
	result, err := runAtBase(dir, Backup, base)
	if err != nil || len(result.Errors) != 0 || filepath.Dir(result.BackupPath) != base {
		t.Fatalf("custom backup: %+v %v", result, err)
	}
	backups, err := listBackupsAt(dir)
	if err != nil || len(backups) != 1 || backups[0].Path != result.BackupPath {
		t.Fatalf("external backup not in history: %+v %v", backups, err)
	}
	if _, conflicts, err := restorePlanAt(dir, backups[0].Path); err != nil || len(conflicts) != 0 {
		t.Fatalf("cannot plan external restore: %v %v", conflicts, err)
	}
	restored, err := restoreAt(dir, backups[0].Path, ConsumeBackup, nil)
	if err != nil || len(restored.Errors) != 0 {
		t.Fatalf("external restore: %+v %v", restored, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "mods", "example.jar")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(result.BackupPath); !os.IsNotExist(err) {
		t.Fatalf("consumed backup remains: %v", err)
	}
}

func TestMissingExternalBackupDoesNotHideLocalHistory(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(t.TempDir(), "external")
	if err := os.Mkdir(base, 0700); err != nil {
		t.Fatal(err)
	}
	makeFolder(t, dir, "mods")
	local, err := runAt(dir, Backup)
	if err != nil {
		t.Fatal(err)
	}
	makeFolder(t, dir, "mods")
	if _, err := runAtBase(dir, Backup, base); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(base, base+"-offline"); err != nil {
		t.Fatal(err)
	}
	backups, err := listBackupsAt(dir)
	if err != nil || len(backups) != 1 || backups[0].Path != local.BackupPath {
		t.Fatalf("local backup hidden by missing external location: %+v %v", backups, err)
	}
}

func TestCustomBackupRefusesNestedBackup(t *testing.T) {
	dir := t.TempDir()
	base := t.TempDir()
	makeFolder(t, dir, "mods")
	first, err := runAtBase(dir, Backup, base)
	if err != nil {
		t.Fatal(err)
	}
	makeFolder(t, dir, "mods")
	if _, err := runAtBase(dir, Backup, first.BackupPath); err == nil {
		t.Fatal("backup nested inside existing external backup")
	}
	if _, err := os.Stat(filepath.Join(dir, "mods", "example.jar")); err != nil {
		t.Fatal("rejected backup touched active data:", err)
	}
	backups, err := listBackupsAt(dir)
	if err != nil || len(backups) != 1 || backups[0].Path != first.BackupPath {
		t.Fatalf("history corrupted: %+v %v", backups, err)
	}
}

func TestCustomBackupRefusesTargetAndLinkedAncestor(t *testing.T) {
	dir := t.TempDir()
	makeFolder(t, dir, "mods")
	if _, err := runAtBase(dir, Backup, filepath.Join(dir, "mods")); err == nil {
		t.Fatal("backup placed inside the folder being moved")
	}
	outside := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(outside, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlinks unavailable: %v", err)
		}
		t.Fatal(err)
	}
	if _, err := runAtBase(dir, Backup, link); err == nil {
		t.Fatal("backup placed through a symlink")
	}
	if _, err := os.Stat(filepath.Join(dir, "mods", "example.jar")); err != nil {
		t.Fatal("failed backup touched mods:", err)
	}
}

func TestRestoreRequiresConfirmationAndOffersKeepOrConsume(t *testing.T) {
	dir := t.TempDir()
	makeFolder(t, dir, "mods")
	original, err := runAt(dir, Backup)
	if err != nil {
		t.Fatal(err)
	}
	backupName := filepath.Base(original.BackupPath)
	makeFolder(t, dir, "mods")
	if _, conflicts, err := restorePlanAt(dir, backupName); err != nil || len(conflicts) != 1 || conflicts[0] != "mods" {
		t.Fatalf("occupied folder not detected: %v %v", conflicts, err)
	}
	if _, err := restoreAt(dir, backupName, KeepBackup, nil); err == nil {
		t.Fatal("restore replaced data without confirmation")
	}
	if _, err := restoreAt(dir, backupName, KeepBackup, []string{"mods"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "mods", "example.jar")); err != nil {
		t.Fatal("restored file missing:", err)
	}
	if _, err := os.Stat(filepath.Join(original.BackupPath, "mods", "example.jar")); err != nil {
		t.Fatal("kept backup missing:", err)
	}
	if err := os.RemoveAll(filepath.Join(dir, "mods")); err != nil {
		t.Fatal(err)
	}
	if _, err := restoreAt(dir, backupName, ConsumeBackup, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(original.BackupPath); !os.IsNotExist(err) {
		t.Fatalf("consumed backup still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "mods", "example.jar")); err != nil {
		t.Fatal("consumed restore missing file:", err)
	}
}

func TestKeepRestoreRejectsLinksBeforeReplacingActiveData(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("protected"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "mods"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "mods", "linked")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlinks unavailable: %v", err)
		}
		t.Fatal(err)
	}
	backup, err := runAt(dir, Backup)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "mods"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mods", "active.jar"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := rejectLinks(filepath.Join(backup.BackupPath, "mods")); err == nil {
		t.Fatal("link not detected in source backup")
	}
	result, err := restoreAt(dir, filepath.Base(backup.BackupPath), KeepBackup, []string{"mods"})
	if err != nil || len(result.Errors) == 0 {
		t.Fatalf("copy restore accepted a link: %+v %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "mods", "active.jar")); err != nil {
		t.Fatal("active data was replaced after copy validation failed:", err)
	}
	if _, err := os.Stat(filepath.Join(backup.BackupPath, "mods", "linked")); err != nil {
		t.Fatal("backup link unexpectedly removed:", err)
	}
	if data, err := os.ReadFile(outside); err != nil || string(data) != "protected" {
		t.Fatalf("outside target changed: %q, %v", data, err)
	}
}

func TestRestoreRefusesUnapprovedChangesAfterInspection(t *testing.T) {
	dir := t.TempDir()
	makeFolder(t, dir, "mods")
	makeFolder(t, dir, "config")
	backup, err := runAt(dir, Backup)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "mods"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mods", "active.jar"), []byte("active"), 0600); err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(backup.BackupPath)
	_, approved, err := restorePlanAt(dir, name)
	if err != nil || len(approved) != 1 || approved[0] != "mods" {
		t.Fatalf("unexpected plan: %v %v", approved, err)
	}
	if err := os.Mkdir(filepath.Join(dir, "config"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config", "new.json"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := restoreAt(dir, name, KeepBackup, approved); err == nil {
		t.Fatal("restore replaced a folder that was not shown for confirmation")
	}
	for _, file := range []string{"mods/active.jar", "config/new.json"} {
		if _, err := os.Stat(filepath.Join(dir, file)); err != nil {
			t.Fatalf("preflight failure changed %s: %v", file, err)
		}
	}
}

func TestRestoreIntoEmptyFolders(t *testing.T) {
	dir := t.TempDir()
	makeFolder(t, dir, "config")
	result, err := runAt(dir, Backup)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "config"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, conflicts, err := restorePlanAt(dir, filepath.Base(result.BackupPath)); err != nil || len(conflicts) != 0 {
		t.Fatalf("empty directory treated as occupied: %v %v", conflicts, err)
	}
	if _, err := restoreAt(dir, filepath.Base(result.BackupPath), KeepBackup, nil); err != nil {
		t.Fatal(err)
	}
}

func TestListBackupsIncludesDateAndFolders(t *testing.T) {
	dir := t.TempDir()
	makeFolder(t, dir, "config")
	created, err := runAt(dir, Backup)
	if err != nil {
		t.Fatal(err)
	}
	backups, err := listBackupsAt(dir)
	if err != nil || len(backups) != 1 {
		t.Fatalf("backup listing failed: %+v %v", backups, err)
	}
	if backups[0].Name != filepath.Base(created.BackupPath) || backups[0].Created.IsZero() || len(backups[0].Folders) != 1 || backups[0].Folders[0] != "config" {
		t.Fatalf("backup metadata missing: %+v", backups[0])
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
