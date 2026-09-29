package modpacks

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"mcTUI/internal/instances"
)

func plainInstallInstance(t *testing.T, root string) (instances.Instance, string) {
	t.Helper()
	in, err := instances.Create(root, "Existing", "1.20.1", "Vanilla", "", 1536)
	if err != nil {
		t.Fatal(err)
	}
	game, err := instances.GameDir(root, in)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(game, 0700); err != nil {
		t.Fatal(err)
	}
	return in, game
}

func installFile(t *testing.T, game, path, body string) {
	t.Helper()
	dest, err := destination(game, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestInstallIntoManagedInstanceWithoutGameDirectory(t *testing.T) {
	root := t.TempDir()
	in, err := instances.Create(root, "Empty", "1.20.1", "Vanilla", "", 1536)
	if err != nil {
		t.Fatal(err)
	}
	game, err := instances.GameDir(root, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(game); !os.IsNotExist(err) {
		t.Fatalf("game directory already present: %v", err)
	}
	archive := updateArchive(t, root, "empty", map[string]string{"config/new.txt": "installed"})
	store := Store{Root: root}
	plan, err := store.PlanInstall(context.Background(), in, archive, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.InstallInto(context.Background(), in, archive, Source{Type: "local"}, nil, plan, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got := updateRead(t, game, "config/new.txt"); got != "installed" {
		t.Fatalf("pack not installed in empty instance: %q", got)
	}
}

func TestInstallIntoPreservesLocalFilesAndRequiresReview(t *testing.T) {
	root := t.TempDir()
	in, game := plainInstallInstance(t, root)
	original := in
	for path, body := range map[string]string{
		"config/preserved.txt": "mine", "config/replaced.txt": "older", "config/equal.txt": "same",
		"config/unrelated.txt": "untouched", "saves/world/level.dat": "world", "logs/latest.log": "log",
	} {
		installFile(t, game, path, body)
	}
	archive := updateArchive(t, root, "new", map[string]string{
		"config/preserved.txt": "incoming", "config/replaced.txt": "updated", "config/equal.txt": "same", "config/fresh.txt": "new",
	})
	store := Store{Root: root}
	plan, err := store.PlanInstall(context.Background(), in, archive, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 4 || len(plan.Conflicts) != 3 {
		t.Fatalf("missing incoming paths or occupied destinations: %+v", plan)
	}
	for _, p := range plan.Conflicts {
		if p.Local == "" || p.Upstream != "added" {
			t.Fatalf("incorrect conflict: %+v", p)
		}
	}
	if err := store.InstallInto(context.Background(), in, archive, Source{Type: "local"}, nil, plan, nil, nil); err == nil || !strings.Contains(err.Error(), "decision required") {
		t.Fatalf("missing decision allowed: %v", err)
	}
	if got := updateRead(t, game, "config/replaced.txt"); got != "older" {
		t.Fatalf("rejection modified local data: %s", got)
	}
	decisions := map[string]bool{"config/preserved.txt": false, "config/replaced.txt": true, "config/equal.txt": false}
	if err := store.InstallInto(context.Background(), in, archive, Source{Type: "local"}, nil, plan, decisions, nil); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"config/preserved.txt": "mine", "config/replaced.txt": "updated", "config/equal.txt": "same", "config/fresh.txt": "new",
		"config/unrelated.txt": "untouched", "saves/world/level.dat": "world", "logs/latest.log": "log",
	} {
		if got := updateRead(t, game, path); got != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
	m, err := loadManifest(filepath.Dir(game))
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := m.Files["config/preserved.txt"]; exists {
		t.Fatal("preserved local file acquired ownership")
	}
	if _, exists := m.Files["config/equal.txt"]; exists {
		t.Fatal("equal-but-preserved file acquired ownership")
	}
	if m.Files["config/replaced.txt"].SHA512 != updateSHA("updated") {
		t.Fatal("approved replacement missing from base")
	}
	stored, err := instances.Read(root, in.ID)
	if err != nil || !reflect.DeepEqual(stored, original) {
		t.Fatalf("instance metadata changed: %+v (%v)", stored, err)
	}
	if _, err := store.PlanInstall(context.Background(), in, archive, nil); err == nil || !strings.Contains(err.Error(), "update instead") {
		t.Fatalf("second install accepted: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(filepath.Dir(game), updateJournalName)); !os.IsNotExist(err) {
		t.Fatalf("journal remains: %v", err)
	}
}

func TestInstallIntoRejectsStaleAndIncompatibleWithoutMutation(t *testing.T) {
	root := t.TempDir()
	in, game := plainInstallInstance(t, root)
	installFile(t, game, "config/settings.txt", "first")
	archive := updateArchive(t, root, "fresh", map[string]string{"config/settings.txt": "new", "mods/added.jar": "mod"})
	store := Store{Root: root}
	plan, err := store.PlanInstall(context.Background(), in, archive, nil)
	if err != nil {
		t.Fatal(err)
	}
	installFile(t, game, "config/settings.txt", "second")
	if err := store.InstallInto(context.Background(), in, archive, Source{Type: "local"}, nil, plan, map[string]bool{"config/settings.txt": true}, nil); err == nil || !strings.Contains(err.Error(), "review again") {
		t.Fatalf("stale plan allowed: %v", err)
	}
	if got := updateRead(t, game, "config/settings.txt"); got != "second" {
		t.Fatalf("local file modified on stale plan: %s", got)
	}
	if _, err := os.Lstat(filepath.Join(game, "mods", "added.jar")); !os.IsNotExist(err) {
		t.Fatalf("stale install published file: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(filepath.Dir(game), "pack.json")); !os.IsNotExist(err) {
		t.Fatalf("stale install published manifest: %v", err)
	}
	incompatible := updateArchiveRuntime(t, root, "other", "1.21", map[string]string{"config/settings.txt": "other"})
	if _, err := store.PlanInstall(context.Background(), in, incompatible, nil); err == nil || !strings.Contains(err.Error(), "runtime") {
		t.Fatalf("incompatible runtime accepted: %v", err)
	}
	protected := updateArchive(t, root, "protected", map[string]string{"saves/world/level.dat": "bad"})
	if _, err := store.PlanInstall(context.Background(), in, protected, nil); err == nil || !strings.Contains(err.Error(), "worlds or logs") {
		t.Fatalf("world file accepted: %v", err)
	}
	if got := updateRead(t, game, "config/settings.txt"); got != "second" {
		t.Fatal("rejection modified local file")
	}
}

func TestInstallIntoMatchesExactFabricLoaderVersion(t *testing.T) {
	root := t.TempDir()
	in, err := instances.Create(root, "Fabric", "1.20.1", "Fabric", "0.15.11", 2048)
	if err != nil {
		t.Fatal(err)
	}
	game, err := instances.GameDir(root, in)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(game, 0700); err != nil {
		t.Fatal(err)
	}
	wrong := updateArchiveRuntime(t, root, "fabric-wrong", "1.20.1", map[string]string{"mods/new.jar": "bad"}, "0.15.10")
	store := Store{Root: root}
	if _, err := store.PlanInstall(context.Background(), in, wrong, nil); err == nil || !strings.Contains(err.Error(), "runtime") {
		t.Fatalf("loader version mismatch accepted: %v", err)
	}
	matching := updateArchiveRuntime(t, root, "fabric-right", "1.20.1", map[string]string{"mods/new.jar": "good"}, "0.15.11")
	plan, err := store.PlanInstall(context.Background(), in, matching, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.InstallInto(context.Background(), in, matching, Source{Type: "local"}, nil, plan, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got := updateRead(t, game, "mods/new.jar"); got != "good" {
		t.Fatalf("matching loader pack not installed: %q", got)
	}
}

func TestInstallIntoCanceledDuringStagingLeavesPlainInstanceUntouched(t *testing.T) {
	root := t.TempDir()
	in, game := plainInstallInstance(t, root)
	archive := updateArchive(t, root, "cancel", map[string]string{"config/new.txt": "incoming"})
	store := Store{Root: root}
	plan, err := store.PlanInstall(context.Background(), in, archive, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.InstallInto(ctx, in, archive, Source{Type: "local"}, nil, plan, nil, nil); err == nil {
		t.Fatal("canceled install succeeded")
	}
	if _, err := os.Lstat(filepath.Join(game, "config/new.txt")); !os.IsNotExist(err) {
		t.Fatalf("canceled install published file: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(filepath.Dir(game), "pack.json")); !os.IsNotExist(err) {
		t.Fatalf("canceled install published manifest: %v", err)
	}
}

func TestRecoverInterruptedPlainInstanceInstall(t *testing.T) {
	root := t.TempDir()
	in, game := plainInstallInstance(t, root)
	installFile(t, game, "config/old.txt", "my config")
	dir := filepath.Dir(game)
	stage, err := os.MkdirTemp(dir, ".mctui-pack-stage-")
	if err != nil {
		t.Fatal(err)
	}
	backup, err := destination(filepath.Join(stage, "backup"), "config/old.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := copyUpdateFile(filepath.Join(game, "config/old.txt"), backup); err != nil {
		t.Fatal(err)
	}
	archive := updateArchive(t, root, "recovery", map[string]string{"config/old.txt": "new", "mods/added.jar": "added"})
	_, _, next, err := (Store{Root: root}).installBase(context.Background(), in, archive, nil)
	if err != nil {
		t.Fatal(err)
	}
	next.Source = Source{Type: "local"}
	journal := updateJournal{Schema: 2, OldAbsent: true, Stage: filepath.Base(stage), Next: next, Changes: []updateChange{
		{Path: "config/old.txt", Before: updateSHA("my config"), After: updateSHA("new")},
		{Path: "mods/added.jar", After: updateSHA("added")},
	}}
	if err := saveJSON(dir, updateJournalName, journal); err != nil {
		t.Fatal(err)
	}
	installFile(t, game, "config/old.txt", "new")
	installFile(t, game, "mods/added.jar", "added")
	if err := saveManifest(dir, next); err != nil {
		t.Fatal(err)
	}
	if _, err := (Store{Root: root}).PlanInstall(context.Background(), in, archive, nil); err == nil || !strings.Contains(err.Error(), "recovery pending") {
		t.Fatalf("pending recovery ignored: %v", err)
	}
	if err := Recover(root, in); err != nil {
		t.Fatal(err)
	}
	if got := updateRead(t, game, "config/old.txt"); got != "my config" {
		t.Fatalf("original not restored: %q", got)
	}
	if _, err := os.Lstat(filepath.Join(game, "mods/added.jar")); !os.IsNotExist(err) {
		t.Fatalf("new file not removed: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "pack.json")); !os.IsNotExist(err) {
		t.Fatalf("new manifest not removed: %v", err)
	}
	if err := Recover(root, in); err != nil {
		t.Fatalf("recovery not repeatable: %v", err)
	}
}

func TestInstallIntoLegacyUsesDataRootForManifest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", home)
	root := t.TempDir()
	game := filepath.Join(home, ".minecraft")
	if err := os.Mkdir(game, 0700); err != nil {
		t.Fatal(err)
	}
	in, err := instances.Read(root, instances.LegacyID)
	if err != nil {
		t.Fatal(err)
	}
	if in.Loader != "Vanilla" || in.MinecraftVersion == "" {
		t.Fatalf("unexpected legacy runtime: %+v", in)
	}
	archive := updateArchiveRuntime(t, root, "legacy", in.MinecraftVersion, map[string]string{"config/legacy.txt": "value"})
	store := Store{Root: root}
	plan, err := store.PlanInstall(context.Background(), in, archive, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.InstallInto(context.Background(), in, archive, Source{Type: "local"}, nil, plan, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got := updateRead(t, game, "config/legacy.txt"); got != "value" {
		t.Fatalf("legacy install missing: %q", got)
	}
	if _, err := loadManifest(filepath.Join(root, "legacy-pack")); err != nil {
		t.Fatalf("legacy metadata not stored under data root: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, "pack.json")); !os.IsNotExist(err) {
		t.Fatalf("legacy manifest escaped data root: %v", err)
	}
	next := updateArchiveRuntime(t, root, "legacy-next", in.MinecraftVersion, map[string]string{"config/legacy.txt": "revised"})
	review, err := store.PlanUpdate(context.Background(), in, next, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(review.Conflicts) != 0 {
		t.Fatalf("unexpected legacy update conflict: %+v", review)
	}
	if err := store.Update(context.Background(), in, next, Source{Type: "local"}, nil, review, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got := updateRead(t, game, "config/legacy.txt"); got != "revised" {
		t.Fatalf("legacy update missing: %q", got)
	}
	out := filepath.Join(root, "legacy-export.mrpack")
	if err := store.Export(context.Background(), in, out, []string{"config/legacy.txt"}); err != nil {
		t.Fatal(err)
	}
	exported, err := Inspect(out)
	if err != nil || exported.Index.VersionID != "legacy-next" || len(exported.Overrides) != 1 {
		t.Fatalf("legacy export missing updated content: %+v (%v)", exported, err)
	}
}

func TestRecoverInterruptedLegacyInstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", home)
	root := t.TempDir()
	game := filepath.Join(home, ".minecraft")
	if err := os.Mkdir(game, 0700); err != nil {
		t.Fatal(err)
	}
	in, err := instances.Read(root, instances.LegacyID)
	if err != nil {
		t.Fatal(err)
	}
	if in.Loader != "Vanilla" {
		t.Fatalf("unexpected legacy runtime: %+v", in)
	}
	archive := updateArchiveRuntime(t, root, "legacy-crash", in.MinecraftVersion, map[string]string{"mods/new.jar": "staged"})
	store := Store{Root: root}
	_, dir, next, err := store.installBase(context.Background(), in, archive, nil)
	if err != nil {
		t.Fatal(err)
	}
	next.Source = Source{Type: "local"}
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	stage, err := os.MkdirTemp(dir, ".mctui-pack-stage-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(stage, "backup"), 0700); err != nil {
		t.Fatal(err)
	}
	journal := updateJournal{Schema: 2, OldAbsent: true, Stage: filepath.Base(stage), Next: next,
		Changes: []updateChange{{Path: "mods/new.jar", After: updateSHA("staged")}}}
	if err := saveJSON(dir, updateJournalName, journal); err != nil {
		t.Fatal(err)
	}
	installFile(t, game, "mods/new.jar", "staged")
	if err := saveManifest(dir, next); err != nil {
		t.Fatal(err)
	}
	if err := Recover(root, in); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(game, "mods/new.jar")); !os.IsNotExist(err) {
		t.Fatalf("published legacy file not rolled back: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "pack.json")); !os.IsNotExist(err) {
		t.Fatalf("legacy manifest not removed: %v", err)
	}
	if err := Recover(root, in); err != nil {
		t.Fatalf("legacy recovery not idempotent: %v", err)
	}
}
