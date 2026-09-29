package modpacks

import (
	"archive/zip"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mcTUI/internal/instances"
)

func updateSHA(data string) string {
	h := sha512.Sum512([]byte(data))
	return hex.EncodeToString(h[:])
}

func updateArchive(t *testing.T, dir, version string, files map[string]string) Archive {
	return updateArchiveRuntime(t, dir, version, "1.20.1", files)
}

func updateArchiveRuntime(t *testing.T, dir, version, minecraft string, files map[string]string, fabricVersion ...string) Archive {
	t.Helper()
	path := filepath.Join(dir, version+".mrpack")
	f, err := os.Create(path)
	if err != nil { t.Fatal(err) }
	z := zip.NewWriter(f)
	dependencies := map[string]string{"minecraft": minecraft}
	if len(fabricVersion) > 0 { dependencies["fabric-loader"] = fabricVersion[0] }
	index := map[string]any{"formatVersion": 1, "game": "minecraft", "name": "Pack " + version, "versionId": version, "dependencies": dependencies, "files": []any{}}
	data, err := json.Marshal(index)
	if err != nil { t.Fatal(err) }
	w, err := z.Create("modrinth.index.json")
	if err != nil { t.Fatal(err) }
	if _, err := w.Write(data); err != nil { t.Fatal(err) }
	for name, body := range files {
		w, err := z.Create("overrides/"+name)
		if err != nil { t.Fatal(err) }
		if _, err := w.Write([]byte(body)); err != nil { t.Fatal(err) }
	}
	if err := z.Close(); err != nil { t.Fatal(err) }
	if err := f.Close(); err != nil { t.Fatal(err) }
	archive, err := Inspect(path)
	if err != nil { t.Fatal(err) }
	return archive
}

func updateInstance(t *testing.T, root string, old map[string]string) (instances.Instance, string) {
	t.Helper()
	in, err := instances.Create(root, "Test Pack", "1.20.1", "Vanilla", "", 2048)
	if err != nil { t.Fatal(err) }
	game, err := instances.GameDir(root, in)
	if err != nil { t.Fatal(err) }
	if err := os.Mkdir(game, 0700); err != nil { t.Fatal(err) }
	manifest := Manifest{SchemaVersion: 1, Name: "Pack one", Source: Source{Type: "local"}, IndexVersionID: "one", IndexSHA512: updateSHA("old archive"), MinecraftVersion: "1.20.1", Loader: "Vanilla", Files: map[string]BaseFile{}}
	for path, body := range old {
		file, err := destination(game, path)
		if err != nil { t.Fatal(err) }
		if err := os.WriteFile(file, []byte(body), 0600); err != nil { t.Fatal(err) }
		manifest.Files[path] = BaseFile{SHA512: updateSHA(body), Size: int64(len(body)), Layer: "overrides"}
	}
	if err := saveManifest(filepath.Dir(game), manifest); err != nil { t.Fatal(err) }
	return in, game
}

func updateRead(t *testing.T, game, path string) string {
	t.Helper()
	bytes, err := os.ReadFile(filepath.Join(game, filepath.FromSlash(path)))
	if err != nil { t.Fatal(err) }
	return string(bytes)
}

func TestUpdateThreeWayPreservesEditsAndRequiresDecisions(t *testing.T) {
	root := t.TempDir()
	in, game := updateInstance(t, root, map[string]string{"config/edit.txt": "old", "config/missing.txt": "old", "config/removed.txt": "old", "config/normal.txt": "old", "config/unchanged.txt": "old"})
	if err := os.WriteFile(filepath.Join(game, "config", "edit.txt"), []byte("my edit"), 0600); err != nil { t.Fatal(err) }
	if err := os.Remove(filepath.Join(game, "config", "missing.txt")); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(game, "config", "added.txt"), []byte("my new file"), 0600); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(game, "config", "same.txt"), []byte("same bytes"), 0600); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(game, "config", "unowned.txt"), []byte("do not touch"), 0600); err != nil { t.Fatal(err) }
	archive := updateArchive(t, root, "two", map[string]string{"config/edit.txt": "upstream edit", "config/missing.txt": "upstream missing", "config/normal.txt": "upstream normal", "config/unchanged.txt": "old", "config/added.txt": "upstream new", "config/same.txt": "same bytes"})
	s := Store{Root: root}
	plan, err := s.PlanUpdate(context.Background(), in, archive, nil)
	if err != nil { t.Fatal(err) }
	conflicts := map[string]bool{}
	for _, item := range plan.Conflicts { conflicts[item.Path] = true }
	for _, path := range []string{"config/edit.txt", "config/missing.txt", "config/added.txt", "config/same.txt"} {
		if !conflicts[path] { t.Errorf("missing conflict for %s: %+v", path, plan) }
	}
	if len(conflicts) != 4 { t.Fatalf("unexpected conflicts: %+v", conflicts) }
	if err := s.Update(context.Background(), in, archive, Source{Type: "local"}, nil, plan, nil, nil); err == nil || !strings.Contains(err.Error(), "decision required") { t.Fatalf("expected consent error, got %v", err) }
	if got := updateRead(t, game, "config/normal.txt"); got != "old" { t.Fatalf("modified without consent: %q", got) }
	decisions := map[string]bool{"config/edit.txt": false, "config/missing.txt": false, "config/added.txt": false, "config/same.txt": false}
	if err := s.Update(context.Background(), in, archive, Source{Type: "local"}, nil, plan, decisions, nil); err != nil { t.Fatal(err) }
	for path, want := range map[string]string{"config/edit.txt": "my edit", "config/added.txt": "my new file", "config/same.txt": "same bytes", "config/normal.txt": "upstream normal", "config/unchanged.txt": "old", "config/unowned.txt": "do not touch"} {
		if got := updateRead(t, game, path); got != want { t.Errorf("%s: got %q, want %q", path, got, want) }
	}
	for _, path := range []string{"config/missing.txt", "config/removed.txt"} {
		if _, err := os.Lstat(filepath.Join(game, path)); !os.IsNotExist(err) { t.Errorf("%s unexpectedly present: %v", path, err) }
	}
	manifest, err := loadManifest(filepath.Dir(game))
	if err != nil { t.Fatal(err) }
	if manifest.IndexVersionID != "two" || manifest.Name != "Pack two" || manifest.Files["config/edit.txt"].SHA512 != updateSHA("upstream edit") { t.Fatalf("new base not committed: %+v", manifest) }
	for _, path := range []string{"config/added.txt", "config/same.txt"} {
		if _, owned := manifest.Files[path]; owned { t.Errorf("preserved local extra acquired pack ownership: %s", path) }
	}
	if _, err := os.Lstat(filepath.Join(filepath.Dir(game), updateJournalName)); !os.IsNotExist(err) { t.Fatalf("journal not cleared: %v", err) }
}

func TestUpdateExplicitOverwriteAndRuntimeRejection(t *testing.T) {
	root := t.TempDir()
	in, game := updateInstance(t, root, map[string]string{"config/edit.txt": "old"})
	if err := os.WriteFile(filepath.Join(game, "config", "edit.txt"), []byte("user edit"), 0600); err != nil { t.Fatal(err) }
	archive := updateArchive(t, root, "two", map[string]string{"config/edit.txt": "new"})
	s := Store{Root: root}
	plan, err := s.PlanUpdate(context.Background(), in, archive, nil)
	if err != nil { t.Fatal(err) }
	if err := s.Update(context.Background(), in, archive, Source{Type: "local"}, nil, plan, map[string]bool{"config/edit.txt": true}, nil); err != nil { t.Fatal(err) }
	if got := updateRead(t, game, "config/edit.txt"); got != "new" { t.Fatalf("explicit replacement failed: %q", got) }
	incompatible := updateArchiveRuntime(t, root, "three", "1.21", map[string]string{"config/edit.txt": "next"})
	if _, err := s.PlanUpdate(context.Background(), in, incompatible, nil); err == nil || !strings.Contains(err.Error(), "new instance") { t.Fatalf("runtime change not rejected: %v", err) }
}

func TestUpdateRejectsStaleReviewedConflict(t *testing.T) {
	root := t.TempDir()
	in, game := updateInstance(t, root, map[string]string{"config/edit.txt": "original"})
	path := filepath.Join(game, "config", "edit.txt")
	if err := os.WriteFile(path, []byte("first edit"), 0600); err != nil { t.Fatal(err) }
	archive := updateArchive(t, root, "two", map[string]string{"config/edit.txt": "upstream"})
	store := Store{Root: root}
	reviewed, err := store.PlanUpdate(context.Background(), in, archive, nil)
	if err != nil { t.Fatal(err) }
	if len(reviewed.Conflicts) != 1 { t.Fatalf("expected edited-file conflict: %+v", reviewed) }
	if err := os.WriteFile(path, []byte("second edit"), 0600); err != nil { t.Fatal(err) }
	decisions := map[string]bool{"config/edit.txt": true}
	if err := store.Update(context.Background(), in, archive, Source{Type: "local"}, nil, reviewed, decisions, nil); err == nil || !strings.Contains(err.Error(), "review again") {
		t.Fatalf("stale overwrite decision accepted: %v", err)
	}
	if got := updateRead(t, game, "config/edit.txt"); got != "second edit" { t.Fatalf("new local edit overwritten: %q", got) }
	manifest, err := loadManifest(filepath.Dir(game))
	if err != nil { t.Fatal(err) }
	if manifest.IndexVersionID != "one" { t.Fatalf("manifest changed on rejected review: %s", manifest.IndexVersionID) }
	if _, err := os.Lstat(filepath.Join(filepath.Dir(game), updateJournalName)); !os.IsNotExist(err) { t.Fatalf("rejected update left journal: %v", err) }
	fresh, err := store.PlanUpdate(context.Background(), in, archive, nil)
	if err != nil { t.Fatal(err) }
	if err := store.Update(context.Background(), in, archive, Source{Type: "local"}, nil, fresh, decisions, nil); err != nil { t.Fatalf("freshly reviewed edit failed: %v", err) }
	if got := updateRead(t, game, "config/edit.txt"); got != "upstream" { t.Fatalf("approved update not applied: %q", got) }
}

func TestRecoverBeforeManifestCommitRestoresOriginalAndLocalExtras(t *testing.T) {
	root := t.TempDir()
	in, game := updateInstance(t, root, map[string]string{"config/first.txt": "original"})
	dir := filepath.Dir(game)
	first, err := destination(game, "config/first.txt")
	if err != nil { t.Fatal(err) }
	extra, err := destination(game, "config/extra.txt")
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(extra, []byte("my extra"), 0600); err != nil { t.Fatal(err) }
	old, err := loadManifest(dir)
	if err != nil { t.Fatal(err) }
	stage, err := os.MkdirTemp(dir, ".mctui-pack-stage-")
	if err != nil { t.Fatal(err) }
	backup, err := destination(filepath.Join(stage, "backup"), "config/first.txt")
	if err != nil { t.Fatal(err) }
	if err := copyUpdateFile(first, backup); err != nil { t.Fatal(err) }
	next := old
	next.IndexVersionID, next.IndexSHA512 = "two", updateSHA("new archive")
	next.Files = map[string]BaseFile{
		"config/first.txt": {SHA512: updateSHA("updated"), Size: int64(len("updated")), Layer: "overrides"},
		"config/added.txt": {SHA512: updateSHA("new file"), Size: int64(len("new file")), Layer: "overrides"},
	}
	journal := updateJournal{Schema: 1, Stage: filepath.Base(stage), Old: old, Next: next, Changes: []updateChange{
		{Path: "config/first.txt", Before: updateSHA("original"), After: updateSHA("updated")},
		{Path: "config/added.txt", After: updateSHA("new file")},
	}}
	if err := saveJSON(dir, updateJournalName, journal); err != nil { t.Fatal(err) }
	if err := os.WriteFile(first, []byte("updated"), 0600); err != nil { t.Fatal(err) }
	added, err := destination(game, "config/added.txt")
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(added, []byte("new file"), 0600); err != nil { t.Fatal(err) }
	archive := updateArchive(t, root, "two", map[string]string{"config/first.txt": "updated", "config/added.txt": "new file"})
	if _, err := (Store{Root: root}).PlanUpdate(context.Background(), in, archive, nil); err == nil || !strings.Contains(err.Error(), "recovery pending") {
		t.Fatalf("plan proceeded during interrupted publication: %v", err)
	}
	if err := Recover(root, in); err != nil { t.Fatal(err) }
	if got := updateRead(t, game, "config/first.txt"); got != "original" { t.Fatalf("owned original not restored: %q", got) }
	if _, err := os.Lstat(added); !os.IsNotExist(err) { t.Fatalf("newly published file not removed: %v", err) }
	if got := updateRead(t, game, "config/extra.txt"); got != "my extra" { t.Fatalf("unowned extra changed: %q", got) }
	restored, err := loadManifest(dir)
	if err != nil { t.Fatal(err) }
	if restored.IndexVersionID != "one" { t.Fatalf("manifest changed during rollback: %s", restored.IndexVersionID) }
}

func TestRecoverInterruptedUpdateAndRejectCorruptBackup(t *testing.T) {
	root := t.TempDir()
	in, game := updateInstance(t, root, map[string]string{"config/first.txt": "first", "config/second.txt": "second"})
	dir := filepath.Dir(game)
	old, err := loadManifest(dir)
	if err != nil { t.Fatal(err) }
	stage, err := os.MkdirTemp(dir, ".mctui-pack-stage-")
	if err != nil { t.Fatal(err) }
	backup := filepath.Join(stage, "backup")
	if err := os.Mkdir(backup, 0700); err != nil { t.Fatal(err) }
	for _, path := range []string{"config/first.txt", "config/second.txt"} {
		target, err := destination(backup, path)
		if err != nil { t.Fatal(err) }
		if err := copyUpdateFile(filepath.Join(game, path), target); err != nil { t.Fatal(err) }
	}
	next := old
	next.IndexVersionID = "two"
	next.IndexSHA512 = updateSHA("new archive")
	next.Name = "Pack two"
	next.Files = map[string]BaseFile{"config/first.txt": {SHA512: updateSHA("replacement"), Size: int64(len("replacement")), Layer: "overrides"}}
	journal := updateJournal{Schema: 1, Stage: filepath.Base(stage), Old: old, Next: next, Changes: []updateChange{{Path: "config/first.txt", Before: updateSHA("first"), After: updateSHA("replacement")}, {Path: "config/second.txt", Before: updateSHA("second"), After: ""}}}
	if err := saveJSON(dir, updateJournalName, journal); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(game, "config/first.txt"), []byte("replacement"), 0600); err != nil { t.Fatal(err) }
	if err := os.Remove(filepath.Join(game, "config/second.txt")); err != nil { t.Fatal(err) }
	if err := saveManifest(dir, next); err != nil { t.Fatal(err) }
	oldFile := filepath.Join(backup, "config/second.txt")
	if err := os.WriteFile(oldFile, []byte("corrupt"), 0600); err != nil { t.Fatal(err) }
	if err := Recover(root, in); err == nil { t.Fatal("corrupt backup accepted") }
	if got := updateRead(t, game, "config/first.txt"); got != "replacement" { t.Fatalf("partial recovery despite invalid backup: %q", got) }
	if err := os.WriteFile(oldFile, []byte("second"), 0600); err != nil { t.Fatal(err) }
	if err := Recover(root, in); err != nil { t.Fatal(err) }
	for path, want := range map[string]string{"config/first.txt": "first", "config/second.txt": "second"} {
		if got := updateRead(t, game, path); got != want { t.Fatalf("%s: %q", path, got) }
	}
	restored, err := loadManifest(dir)
	if err != nil { t.Fatal(err) }
	if restored.IndexVersionID != "one" { t.Fatalf("old pack version not restored: %s", restored.IndexVersionID) }
	if err := Recover(root, in); err != nil { t.Fatalf("recovery is not idempotent: %v", err) }
	if _, err := os.Stat(stage); !os.IsNotExist(err) { t.Fatalf("staging still present: %v", err) }
}

func TestRecoverOrdinaryManagedInstanceWithoutPack(t *testing.T) {
	root := t.TempDir()
	in, err := instances.Create(root, "Regular Instance", "1.20.1", "Vanilla", "", 1024)
	if err != nil { t.Fatal(err) }
	if err := Recover(root, in); err != nil { t.Fatalf("ordinary instance blocked: %v", err) }
}

func TestRecoverBlocksInvalidJournal(t *testing.T) {
	root := t.TempDir()
	in, game := updateInstance(t, root, map[string]string{"config/settings.txt": "owned"})
	journal := filepath.Join(filepath.Dir(game), updateJournalName)
	if err := os.WriteFile(journal, []byte("{not json"), 0600); err != nil { t.Fatal(err) }
	if err := Recover(root, in); err == nil { t.Fatal("invalid recovery journal accepted") }
	if got := updateRead(t, game, "config/settings.txt"); got != "owned" { t.Fatalf("recovery changed game file: %q", got) }
}

func TestRecoverRejectsJournalTargetingUnownedLocalExtra(t *testing.T) {
	root := t.TempDir()
	in, game := updateInstance(t, root, map[string]string{"config/settings.txt": "owned"})
	dir := filepath.Dir(game)
	extra, err := destination(game, "config/local.txt")
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(extra, []byte("my local file"), 0600); err != nil { t.Fatal(err) }
	manifest, err := loadManifest(dir)
	if err != nil { t.Fatal(err) }
	stage, err := os.MkdirTemp(dir, ".mctui-pack-stage-")
	if err != nil { t.Fatal(err) }
	if err := os.Mkdir(filepath.Join(stage, "backup"), 0700); err != nil { t.Fatal(err) }
	journal := updateJournal{
		Schema: 1, Stage: filepath.Base(stage), Old: manifest, Next: manifest,
		Changes: []updateChange{{Path: "config/local.txt", After: updateSHA("my local file")}},
	}
	if err := saveJSON(dir, updateJournalName, journal); err != nil { t.Fatal(err) }
	if err := Recover(root, in); err == nil || !strings.Contains(err.Error(), "invalid pack recovery entry") {
		t.Fatalf("unowned file accepted by journal: %v", err)
	}
	if got := updateRead(t, game, "config/local.txt"); got != "my local file" { t.Fatalf("local extra removed during recovery: %q", got) }
	if got := updateRead(t, game, "config/settings.txt"); got != "owned" { t.Fatalf("owned file changed during rejected recovery: %q", got) }
	if _, err := os.Lstat(filepath.Join(dir, updateJournalName)); err != nil { t.Fatalf("invalid journal unexpectedly cleared: %v", err) }
}
