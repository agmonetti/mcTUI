package mods

import (
	"context"
	"crypto/sha512"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"mcTUI/internal/modrinth"
)

func TestInstallOptionalConflictsOnlyWhenSelected(t *testing.T) {
	s := store(t)
	old := filepath.Join(t.TempDir(), "a.jar")
	jar(t, old, nil)
	if err := s.Add(old); err != nil {
		t.Fatal(err)
	}
	r, err := s.load()
	if err != nil {
		t.Fatal(err)
	}
	r.Mods[0].ProjectID, r.Mods[0].VersionID = "A", "v1"
	if err = s.save(r); err != nil {
		t.Fatal(err)
	}
	plan := modrinth.InstallPlan{
		Targets:      []modrinth.PlanItem{{Version: modrinth.Version{ProjectID: "A", ID: "v1"}, AlreadyInstalled: true, InstalledVersion: "v1"}},
		OptionalDeps: []modrinth.PlanItem{{Version: modrinth.Version{ProjectID: "B"}, IncompatibleIDs: []string{"A"}}},
	}
	if err = s.Install(context.Background(), nil, plan, nil, false, nil); err != nil {
		t.Fatalf("omitted optional blocked installation: %v", err)
	}
	if err = s.Install(context.Background(), nil, plan, []string{"B"}, false, nil); err == nil || !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("selected optional conflict accepted: %v", err)
	}
	if _, err = os.Stat(filepath.Join(s.GameDir, "mods", "a.jar")); err != nil {
		t.Fatalf("conflict damaged installed mod: %v", err)
	}
	x := filepath.Join(t.TempDir(), "x.jar")
	jar(t, x, nil)
	if err = s.Add(x); err != nil {
		t.Fatal(err)
	}
	r, err = s.load()
	if err != nil {
		t.Fatal(err)
	}
	for i := range r.Mods {
		if r.Mods[i].File == "x.jar" {
			r.Mods[i].ProjectID, r.Mods[i].VersionID = "X", "v1"
		}
	}
	if err = s.save(r); err != nil {
		t.Fatal(err)
	}
	plan.OptionalDeps[0].IncompatibleIDs = []string{"X"}
	if err = s.Install(context.Background(), nil, plan, []string{"B"}, false, nil); err == nil || !strings.Contains(err.Error(), "installed mod X") {
		t.Fatalf("installed mod conflict accepted: %v", err)
	}
	plan.OptionalDeps[0].IncompatibleIDs = []string{"C"}
	plan.OptionalDeps = append(plan.OptionalDeps, modrinth.PlanItem{Version: modrinth.Version{ProjectID: "C"}})
	if err = s.Install(context.Background(), nil, plan, []string{"B", "C"}, false, nil); err == nil || !strings.Contains(err.Error(), "selected mod C") {
		t.Fatalf("conflicting optional pair accepted: %v", err)
	}
}

func TestLegacyBackupStaysUnderDataRoot(t *testing.T) {
	root := t.TempDir()
	s := Store{GameDir: filepath.Join(root, "external", ".minecraft"), Registry: filepath.Join(root, "data", "instances", "legacy", "mods.json")}
	if err := s.saveHistory([]HistoryEntry{{ProjectID: "example"}}); err != nil {
		t.Fatal(err)
	}
	if entries, err := s.History(); err != nil || len(entries) != 1 {
		t.Fatalf("legacy history unavailable: %+v %v", entries, err)
	}
	if _, err := os.Stat(filepath.Join(root, "data", "instances", "legacy", "backups", "mods_replaced", "history.json")); err != nil {
		t.Fatalf("legacy backup outside mcTUI data root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "external", "backups")); !os.IsNotExist(err) {
		t.Fatalf("backup leaked beside legacy game directory: %v", err)
	}
}

func TestInstallRollbackAndChangedFile(t *testing.T) {
	s := store(t)
	old := filepath.Join(t.TempDir(), "old.jar")
	jar(t, old, nil)
	if err := s.Add(old); err != nil {
		t.Fatal(err)
	}
	initial, err := s.load()
	if err != nil {
		t.Fatal(err)
	}
	initial.Mods[0].ProjectID = "project"
	initial.Mods[0].VersionID = "old"
	initial.Mods[0].Origin = "modrinth"
	if err = s.save(initial); err != nil {
		t.Fatal(err)
	}
	newer := filepath.Join(t.TempDir(), "new.jar")
	jar(t, newer, map[string]string{"fabric.mod.json": `{"id":"sample","version":"2"}`})
	body, err := os.ReadFile(newer)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha512.Sum512(body)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer server.Close()
	transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}
	c := &modrinth.Client{HTTP: &http.Client{Transport: transport}}
	file := modrinth.File{URL: "https://cdn.modrinth.net/new.jar", Filename: "new.jar", Primary: true, Size: int64(len(body)), Hashes: modrinth.Hashes{SHA512: hex.EncodeToString(sum[:])}}
	v := modrinth.Version{ID: "new", ProjectID: "project", Channel: "release", Files: []modrinth.File{file}}
	p := modrinth.InstallPlan{TargetMod: v, Targets: []modrinth.PlanItem{{Version: v, File: file, InstalledVersion: "old"}}}
	if err = s.Install(context.Background(), c, p, nil, false, nil); err != nil {
		t.Fatal(err)
	}
	entries, err := s.History()
	if err != nil || len(entries) != 1 {
		t.Fatalf("history %+v %v", entries, err)
	}
	records, err := s.load()
	if err != nil || records.SchemaVersion != 2 || records.Mods[0].VersionID != "new" {
		t.Fatalf("registry %+v %v", records, err)
	}
	if err = s.Rollback("project"); err != nil {
		t.Fatal(err)
	}
	records, err = s.load()
	if err != nil || records.Mods[0].VersionID != "old" {
		t.Fatalf("rollback %+v %v", records, err)
	}
	if _, err = os.Stat(filepath.Join(s.GameDir, "mods", "old.jar")); err != nil {
		t.Fatal(err)
	}
	// Changed file must not be overwritten without separate consent.
	if err = os.WriteFile(filepath.Join(s.GameDir, "mods", "old.jar"), []byte("edited"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = s.Install(context.Background(), c, p, nil, false, nil); err == nil || !strings.Contains(err.Error(), "modified") {
		t.Fatalf("local modification ignored: %v", err)
	}
	if err = s.Install(context.Background(), c, p, nil, true, nil); err != nil {
		t.Fatalf("explicit overwrite failed: %v", err)
	}
	if err = s.Rollback("project"); err != nil {
		t.Fatalf("approved local edit cannot be restored: %v", err)
	}
	if restored, err := os.ReadFile(filepath.Join(s.GameDir, "mods", "old.jar")); err != nil || string(restored) != "edited" {
		t.Fatalf("approved local edit lost: %q %v", restored, err)
	}
}
func TestMultiFilePublishFailureRestoresEarlierFiles(t *testing.T) {
	s := store(t)
	old := filepath.Join(t.TempDir(), "older.jar")
	jar(t, old, nil)
	if err := s.Add(old); err != nil {
		t.Fatal(err)
	}
	original, err := s.load()
	if err != nil {
		t.Fatal(err)
	}
	original.Mods[0].ProjectID, original.Mods[0].VersionID = "one.jar", "old"
	if err := s.save(original); err != nil {
		t.Fatal(err)
	}
	one, two := filepath.Join(t.TempDir(), "one.jar"), filepath.Join(t.TempDir(), "two.jar")
	jar(t, one, map[string]string{"fabric.mod.json": `{"id":"one","version":"1"}`})
	jar(t, two, map[string]string{"fabric.mod.json": `{"id":"two","version":"1"}`})
	bodies := map[string][]byte{}
	for name, path := range map[string]string{"one.jar": one, "two.jar": two} {
		bodies[name], _ = os.ReadFile(path)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(bodies[filepath.Base(r.URL.Path)]) }))
	defer server.Close()
	transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}
	client := &modrinth.Client{HTTP: &http.Client{Transport: transport}}
	versions := []modrinth.Version{}
	for _, name := range []string{"one.jar", "two.jar"} {
		digest := sha512.Sum512(bodies[name])
		f := modrinth.File{URL: "https://cdn.modrinth.net/" + name, Filename: name, Size: int64(len(bodies[name])), Hashes: modrinth.Hashes{SHA512: hex.EncodeToString(digest[:])}}
		versions = append(versions, modrinth.Version{ID: name, ProjectID: name, Channel: "release", Files: []modrinth.File{f}})
	}
	plan := modrinth.InstallPlan{TargetMod: versions[0], Targets: []modrinth.PlanItem{}}
	for _, v := range versions {
		f, _ := v.PrimaryFile()
		item := modrinth.PlanItem{Version: v, File: f}
		if v.ProjectID == "one.jar" {
			item.InstalledVersion = "old"
		}
		plan.Targets = append(plan.Targets, item)
	}
	completedFirst := false
	createdBlocker := false
	progress := func(downloaded, total int64) {
		if completedFirst && !createdBlocker && downloaded > 0 {
			_ = os.Mkdir(filepath.Join(s.GameDir, "mods", "two.jar"), 0700)
			createdBlocker = true
		}
		if downloaded == total {
			completedFirst = true
		}
	}
	if err := s.Install(context.Background(), client, plan, nil, false, progress); err == nil {
		t.Fatal("expected publication failure")
	}
	if _, err := os.Stat(filepath.Join(s.GameDir, "mods", "one.jar")); !os.IsNotExist(err) {
		t.Fatalf("earlier JAR not rolled back: %v", err)
	}
	if restored, err := s.load(); err != nil || len(restored.Mods) != 1 || restored.Mods[0].VersionID != "old" {
		t.Fatalf("registry not preserved: %+v %v", restored, err)
	}
	if _, err := os.Stat(filepath.Join(s.GameDir, "mods", "older.jar")); err != nil {
		t.Fatalf("previous JAR lost: %v", err)
	}
	if history, err := s.History(); err != nil || len(history) != 0 {
		t.Fatalf("uncommitted backup: %+v %v", history, err)
	}
	if info, err := os.Stat(filepath.Join(s.GameDir, "mods", "two.jar")); err != nil || !info.IsDir() {
		t.Fatalf("external destination changed: %v", err)
	}
}

func TestInstallCancelDuringDownloadLeavesNoJAR(t *testing.T) {
	s := store(t)
	path := filepath.Join(t.TempDir(), "cancel.jar")
	jar(t, path, nil)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(body) }))
	defer server.Close()
	transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}
	sum := sha512.Sum512(body)
	file := modrinth.File{URL: "https://cdn.modrinth.net/cancel.jar", Filename: "cancel.jar", Size: int64(len(body)), Hashes: modrinth.Hashes{SHA512: hex.EncodeToString(sum[:])}}
	version := modrinth.Version{ID: "v", ProjectID: "p", Channel: "release", Files: []modrinth.File{file}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	plan := modrinth.InstallPlan{Targets: []modrinth.PlanItem{{Version: version, File: file}}}
	err = s.Install(ctx, &modrinth.Client{HTTP: &http.Client{Transport: transport}}, plan, nil, false, func(n, _ int64) { cancel() })
	if err == nil {
		t.Fatal("canceled installation succeeded")
	}
	entries, err := os.ReadDir(filepath.Join(s.GameDir, "mods"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("cancel left files: %v %v", entries, err)
	}
	items, err := s.Scan()
	if err != nil || len(items) != 0 {
		t.Fatalf("cancel changed registry: %v %v", items, err)
	}
}

func TestResolveOptionalBranchInstallsWholeSelection(t *testing.T) {
	s := store(t)
	versions := map[string]modrinth.Version{}
	bodies := map[string][]byte{}
	for _, id := range []string{"A", "B", "C", "D"} {
		path := filepath.Join(t.TempDir(), id+".jar")
		jar(t, path, map[string]string{"fabric.mod.json": `{"id":"` + id + `","version":"1"}`})
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		bodies[id+".jar"] = body
		sum := sha512.Sum512(body)
		file := modrinth.File{URL: "https://cdn.modrinth.net/" + id + ".jar", Filename: id + ".jar", Primary: true, Size: int64(len(body)), Hashes: modrinth.Hashes{SHA512: hex.EncodeToString(sum[:])}}
		versions[id] = modrinth.Version{ID: id + "-v1", ProjectID: id, Channel: "release", Loaders: []string{"fabric"}, GameVersions: []string{"1.21"}, Files: []modrinth.File{file}}
	}
	a, b, c := versions["A"], versions["B"], versions["C"]
	a.Dependencies = []modrinth.Dependency{{ProjectID: "B", Type: "optional"}}
	b.Dependencies = []modrinth.Dependency{{ProjectID: "C", Type: "required"}}
	c.Dependencies = []modrinth.Dependency{{ProjectID: "D", Type: "required"}}
	versions["B"], versions["C"] = b, c
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v2/project/") {
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v2/project/"), "/version")
			_ = json.NewEncoder(w).Encode([]modrinth.Version{versions[id]})
			return
		}
		_, _ = w.Write(bodies[filepath.Base(r.URL.Path)])
	}))
	defer server.Close()
	transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}
	client := &modrinth.Client{HTTP: &http.Client{Transport: transport}}
	plan, err := client.Resolve(context.Background(), a, "fabric", "1.21", "release", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Install(context.Background(), client, plan, []string{"B"}, false, nil); err != nil {
		t.Fatal(err)
	}
	items, err := s.Scan()
	if err != nil || len(items) != 4 {
		t.Fatalf("optional branch not fully published: %+v %v", items, err)
	}
	for _, id := range []string{"B", "C", "D"} {
		var found bool
		for _, item := range items {
			if item.ProjectID == id {
				found = true
				if id != "B" && (len(item.RequiredBy) != 1 || item.RequiredBy[0] != map[string]string{"C": "B", "D": "C"}[id]) {
					t.Fatalf("missing required owner for %s: %+v", id, item.RequiredBy)
				}
			}
		}
		if !found {
			t.Fatalf("missing selected branch member %s", id)
		}
	}
}

func TestInstallCancelAfterFirstFileLeavesNoBatch(t *testing.T) {
	s := store(t)
	path := filepath.Join(t.TempDir(), "first.jar")
	jar(t, path, nil)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write(body)
	}))
	defer server.Close()
	transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}
	sum := sha512.Sum512(body)
	plan := modrinth.InstallPlan{}
	for _, id := range []string{"first", "second"} {
		file := modrinth.File{URL: "https://cdn.modrinth.net/" + id + ".jar", Filename: id + ".jar", Size: int64(len(body)), Hashes: modrinth.Hashes{SHA512: hex.EncodeToString(sum[:])}}
		plan.Targets = append(plan.Targets, modrinth.PlanItem{Version: modrinth.Version{ID: id, ProjectID: id, Channel: "release"}, File: file})
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err = s.Install(ctx, &modrinth.Client{HTTP: &http.Client{Transport: transport}}, plan, nil, false, func(n, _ int64) {
		if n >= int64(len(body)) {
			cancel()
		}
	})
	if err == nil || requests != 1 {
		t.Fatalf("canceled batch continued: %v, requests=%d", err, requests)
	}
	entries, err := os.ReadDir(filepath.Join(s.GameDir, "mods"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("canceled batch left files: %v %v", entries, err)
	}
	items, err := s.Scan()
	if err != nil || len(items) != 0 {
		t.Fatalf("canceled batch changed registry: %v %v", items, err)
	}
}

func TestInterruptedInstallRecoversOnScan(t *testing.T) {
	for _, sameName := range []bool{false, true} {
		t.Run(map[bool]string{false: "renamed", true: "same-name"}[sameName], func(t *testing.T) {
			s := store(t)
			oldSource := filepath.Join(t.TempDir(), "old.jar")
			jar(t, oldSource, nil)
			if err := s.Add(oldSource); err != nil {
				t.Fatal(err)
			}
			before, err := s.load()
			if err != nil {
				t.Fatal(err)
			}
			name := "new.jar"
			if sameName {
				name = "old.jar"
			}
			dir := filepath.Join(s.GameDir, "mods")
			temp := filepath.Join(dir, ".tmp-new.jar")
			jar(t, temp, map[string]string{"fabric.mod.json": `{"id":"example","version":"2"}`})
			newRecord, err := snapshot(temp)
			if err != nil {
				t.Fatal(err)
			}
			newRecord.File = name
			newRecord.Origin = "modrinth"
			newRecord.ProjectID = "project"
			backup := "20260926-0_old.jar"
			pending := installPending{Registry: before, Steps: []installStep{{New: newRecord, Old: &before.Mods[0], Backup: backup, Temp: filepath.Base(temp)}}}
			if err = s.savePending(pending); err != nil {
				t.Fatal(err)
			}
			if err = os.Rename(filepath.Join(dir, "old.jar"), filepath.Join(s.backupDir(), backup)); err != nil {
				t.Fatal(err)
			}
			if err = os.Rename(temp, filepath.Join(dir, name)); err != nil {
				t.Fatal(err)
			}
			// Simulate interruption after metadata was saved but before the journal was cleared.
			if err = s.save(registry{SchemaVersion: 2, Mods: []Record{newRecord}}); err != nil {
				t.Fatal(err)
			}
			if err = s.saveHistory([]HistoryEntry{{ProjectID: "project", Backup: backup}}); err != nil {
				t.Fatal(err)
			}
			items, err := s.Scan()
			if err != nil || len(items) != 1 || items[0].File != "old.jar" || items[0].Changed {
				t.Fatalf("interrupted update not recovered: %+v %v", items, err)
			}
			if _, err = os.Stat(s.pendingPath()); !os.IsNotExist(err) {
				t.Fatalf("journal not cleared: %v", err)
			}
			if entries, err := s.History(); err != nil || len(entries) != 0 {
				t.Fatalf("interrupted history not restored: %+v %v", entries, err)
			}
		})
	}
}

func TestRecoveryAfterProcessExitMidBatch(t *testing.T) {
	if root := os.Getenv("MCTUI_CRASH_FIXTURE"); root != "" {
		s := Store{GameDir: filepath.Join(root, "minecraft"), Registry: filepath.Join(root, "mods.json")}
		dir := filepath.Join(s.GameDir, "mods")
		var p installPending
		p.Registry.SchemaVersion = 2
		for _, name := range []string{"one", "two"} {
			old, err := snapshot(filepath.Join(dir, name+"-old.jar"))
			if err != nil {
				t.Fatal(err)
			}
			p.Registry.Mods = append(p.Registry.Mods, old)
			fresh, err := snapshot(filepath.Join(dir, ".tmp-"+name+".jar"))
			if err != nil {
				t.Fatal(err)
			}
			fresh.File = name + ".jar"
			p.Steps = append(p.Steps, installStep{Old: &old, New: fresh, Temp: ".tmp-" + name + ".jar", Backup: name + "_" + old.File})
		}
		if err := s.savePending(p); err != nil {
			t.Fatal(err)
		}
		first := p.Steps[0]
		if err := os.Rename(filepath.Join(dir, first.Old.File), filepath.Join(s.backupDir(), first.Backup)); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(filepath.Join(dir, first.Temp), filepath.Join(dir, first.New.File)); err != nil {
			t.Fatal(err)
		}
		os.Exit(0) // Real process exit: no cleanup defers or graceful shutdown.
	}
	s := store(t)
	root := filepath.Dir(s.GameDir)
	for _, name := range []string{"one", "two"} {
		jar(t, filepath.Join(s.GameDir, "mods", name+"-old.jar"), nil)
		jar(t, filepath.Join(s.GameDir, "mods", ".tmp-"+name+".jar"), map[string]string{"fabric.mod.json": `{"id":"` + name + `","version":"2"}`})
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRecoveryAfterProcessExitMidBatch$")
	cmd.Env = append(os.Environ(), "MCTUI_CRASH_FIXTURE="+root)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("interrupted child: %v %s", err, out)
	}
	items, err := s.Scan()
	if err != nil || len(items) != 2 {
		t.Fatalf("interrupted batch did not recover: %+v %v", items, err)
	}
	for _, name := range []string{"one", "two"} {
		if _, err := os.Stat(filepath.Join(s.GameDir, "mods", name+"-old.jar")); err != nil {
			t.Fatalf("original %s not restored: %v", name, err)
		}
		for _, path := range []string{filepath.Join(s.GameDir, "mods", name+".jar"), filepath.Join(s.GameDir, "mods", ".tmp-"+name+".jar")} {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("uncommitted file remains: %s: %v", path, err)
			}
		}
	}
	if _, err := os.Stat(s.pendingPath()); !os.IsNotExist(err) {
		t.Fatalf("journal not cleared: %v", err)
	}
	if entries, err := s.History(); err != nil || len(entries) != 0 {
		t.Fatalf("uncommitted history: %+v %v", entries, err)
	}
}

func TestInterruptedBatchPreflightPreservesAllFiles(t *testing.T) {
	for _, corrupt := range []string{"backup", "later JAR"} {
		t.Run(corrupt, func(t *testing.T) {
			s := store(t)
			dir := filepath.Join(s.GameDir, "mods")
			if err := os.MkdirAll(s.backupDir(), 0700); err != nil {
				t.Fatal(err)
			}
			var pending installPending
			pending.Registry.SchemaVersion = 2
			for _, name := range []string{"first", "second"} {
				oldPath := filepath.Join(dir, name+"-old.jar")
				if err := os.WriteFile(oldPath, []byte(name+" old"), 0600); err != nil {
					t.Fatal(err)
				}
				old, err := snapshot(oldPath)
				if err != nil {
					t.Fatal(err)
				}
				pending.Registry.Mods = append(pending.Registry.Mods, old)
				newPath := filepath.Join(dir, name+".jar")
				if err = os.WriteFile(newPath, []byte(name+" new"), 0600); err != nil {
					t.Fatal(err)
				}
				newRec, err := snapshot(newPath)
				if err != nil {
					t.Fatal(err)
				}
				backup := name + "_" + old.File
				if err = os.Rename(oldPath, filepath.Join(s.backupDir(), backup)); err != nil {
					t.Fatal(err)
				}
				pending.Steps = append(pending.Steps, installStep{New: newRec, Old: &old, Backup: backup, Temp: ".tmp-" + name + ".jar"})
			}
			if err := s.savePending(pending); err != nil {
				t.Fatal(err)
			}
			badPath := filepath.Join(dir, "first.jar")
			if corrupt == "backup" {
				badPath = filepath.Join(s.backupDir(), pending.Steps[1].Backup)
			}
			if err := os.WriteFile(badPath, []byte("external edit"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := s.recoverInstall(); err == nil {
				t.Fatal("recovery accepted changed file")
			}
			for _, name := range []string{"first", "second"} {
				for _, path := range []string{filepath.Join(dir, name+".jar"), filepath.Join(s.backupDir(), name+"_"+name+"-old.jar")} {
					if _, err := os.Stat(path); err != nil {
						t.Fatalf("recovery partially mutated %s: %v", path, err)
					}
				}
			}
			if _, err := os.Stat(s.pendingPath()); err != nil {
				t.Fatalf("journal removed on blocked recovery: %v", err)
			}
		})
	}
}

func TestInterruptedInstallPreservesExternallyChangedFile(t *testing.T) {
	s := store(t)
	dir := filepath.Join(s.GameDir, "mods")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "new.jar")
	if err := os.WriteFile(file, []byte("expected download"), 0600); err != nil {
		t.Fatal(err)
	}
	rec, err := snapshot(file)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.savePending(installPending{Registry: registry{SchemaVersion: 2}, Steps: []installStep{{New: rec, Temp: ".tmp-new.jar"}}}); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(file, []byte("user changed this"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Scan(); err == nil {
		t.Fatal("recovery should refuse to overwrite changed file")
	}
	if content, err := os.ReadFile(file); err != nil || string(content) != "user changed this" {
		t.Fatalf("changed file lost: %q %v", content, err)
	}
	if _, err = os.Stat(s.pendingPath()); err != nil {
		t.Fatalf("recovery journal lost: %v", err)
	}
}

func TestInstallLinksAlreadyInstalledDependency(t *testing.T) {
	s := store(t)
	for _, id := range []string{"parent", "library"} {
		path := filepath.Join(t.TempDir(), id+".jar")
		jar(t, path, nil)
		if err := s.Add(path); err != nil {
			t.Fatal(err)
		}
	}
	r, err := s.load()
	if err != nil {
		t.Fatal(err)
	}
	for i := range r.Mods {
		r.Mods[i].ProjectID = strings.TrimSuffix(r.Mods[i].File, ".jar")
		r.Mods[i].VersionID = r.Mods[i].ProjectID
	}
	if err := s.save(r); err != nil {
		t.Fatal(err)
	}
	version := func(id string) modrinth.Version {
		return modrinth.Version{ID: id, ProjectID: id, Channel: "release", Files: []modrinth.File{{Filename: id + ".jar"}}}
	}
	plan := modrinth.InstallPlan{TargetMod: version("parent"), Targets: []modrinth.PlanItem{{Version: version("parent"), AlreadyInstalled: true, InstalledVersion: "parent"}}, RequiredDeps: []modrinth.PlanItem{{Version: version("library"), AlreadyInstalled: true, InstalledVersion: "library", RequiredBy: []string{"parent"}}}}
	if err := s.Install(context.Background(), nil, plan, nil, false, nil); err != nil {
		t.Fatal(err)
	}
	items, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.ProjectID == "library" {
			if len(item.RequiredBy) != 1 || item.RequiredBy[0] != "parent" {
				t.Fatalf("missing owner: %+v", item.Record)
			}
			if err := s.Remove(item, true); err == nil {
				t.Fatal("removed required library")
			}
			return
		}
	}
	t.Fatal("library missing")
}

func TestInstallOnlyLinksSelectedOptionalOwners(t *testing.T) {
	for _, choose := range []bool{false, true} {
		t.Run(map[bool]string{false: "omitted", true: "selected"}[choose], func(t *testing.T) {
			s := store(t)
			for _, id := range []string{"A", "B", "library"} {
				path := filepath.Join(t.TempDir(), id+".jar")
				jar(t, path, nil)
				if err := s.Add(path); err != nil {
					t.Fatal(err)
				}
			}
			r, err := s.load()
			if err != nil {
				t.Fatal(err)
			}
			for i := range r.Mods {
				r.Mods[i].ProjectID = strings.TrimSuffix(r.Mods[i].File, ".jar")
				r.Mods[i].VersionID = r.Mods[i].ProjectID
			}
			if err = s.save(r); err != nil {
				t.Fatal(err)
			}
			item := func(id string) modrinth.PlanItem {
				return modrinth.PlanItem{Version: modrinth.Version{ID: id, ProjectID: id}, AlreadyInstalled: true, InstalledVersion: id}
			}
			library := item("library")
			library.RequiredBy = []string{"A", "B"} // B is an unselected optional unless chosen.
			plan := modrinth.InstallPlan{Targets: []modrinth.PlanItem{item("A")}, RequiredDeps: []modrinth.PlanItem{library}, OptionalDeps: []modrinth.PlanItem{item("B")}}
			var optional []string
			if choose {
				optional = []string{"B"}
			}
			if err = s.Install(context.Background(), nil, plan, optional, false, nil); err != nil {
				t.Fatal(err)
			}
			r, err = s.load()
			if err != nil {
				t.Fatal(err)
			}
			for _, rec := range r.Mods {
				if rec.ProjectID == "library" && (!slices.Contains(rec.RequiredBy, "A") || slices.Contains(rec.RequiredBy, "B") != choose) {
					t.Fatalf("incorrect optional ownership: %+v", rec.RequiredBy)
				}
			}
		})
	}
}

func TestInstallRejectsStaleSelectedGraph(t *testing.T) {
	for _, tc := range []struct {
		name, project, version string
		selected               bool
	}{
		{"required version changed", "library", "other", false},
		{"optional version changed", "extra", "other", true},
		{"new incompatible", "blocked", "current", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := store(t)
			v := func(id string) modrinth.Version { return modrinth.Version{ID: "new", ProjectID: id} }
			plan := modrinth.InstallPlan{
				Targets:         []modrinth.PlanItem{{Version: v("main")}},
				RequiredDeps:    []modrinth.PlanItem{{Version: v("library"), InstalledVersion: "current"}},
				OptionalDeps:    []modrinth.PlanItem{{Version: v("extra"), InstalledVersion: "current"}},
				IncompatibleIDs: []string{"blocked"},
			}
			for _, id := range []string{"library", "extra"} {
				path := filepath.Join(t.TempDir(), id+".jar")
				jar(t, path, nil)
				if err := s.Add(path); err != nil {
					t.Fatal(err)
				}
			}
			r, err := s.load()
			if err != nil {
				t.Fatal(err)
			}
			for i := range r.Mods {
				r.Mods[i].ProjectID = strings.TrimSuffix(r.Mods[i].File, ".jar")
				r.Mods[i].VersionID = "current"
			}
			if tc.project == "blocked" {
				path := filepath.Join(t.TempDir(), "blocked.jar")
				jar(t, path, nil)
				if err := s.Add(path); err != nil {
					t.Fatal(err)
				}
				r, err = s.load()
				if err != nil {
					t.Fatal(err)
				}
				r.Mods[len(r.Mods)-1].ProjectID = "blocked"
				r.Mods[len(r.Mods)-1].VersionID = "current"
			} else {
				for i := range r.Mods {
					if r.Mods[i].ProjectID == tc.project {
						r.Mods[i].VersionID = tc.version
					}
				}
			}
			if err := s.save(r); err != nil {
				t.Fatal(err)
			}
			var optional []string
			if tc.selected {
				optional = []string{"extra"}
			}
			if err := s.Install(context.Background(), nil, plan, optional, false, nil); err == nil || !strings.Contains(err.Error(), "stale") {
				t.Fatalf("stale plan accepted: %v", err)
			}
			if _, err := os.Stat(filepath.Join(s.GameDir, "mods", "main.jar")); !os.IsNotExist(err) {
				t.Fatalf("published despite stale plan: %v", err)
			}
		})
	}
}

func TestBatchUpdateReconcilesSharedDependencyOwners(t *testing.T) {
	s := store(t)
	bodies := map[string][]byte{}
	versions := map[string]modrinth.Version{}
	for _, id := range []string{"A", "B", "library"} {
		path := filepath.Join(t.TempDir(), id+".jar")
		jar(t, path, map[string]string{"fabric.mod.json": `{"id":"` + id + `","version":"2"}`})
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		bodies[id+".jar"] = body
		sum := sha512.Sum512(body)
		file := modrinth.File{URL: "https://cdn.modrinth.net/" + id + ".jar", Filename: id + ".jar", Size: int64(len(body)), Hashes: modrinth.Hashes{SHA512: hex.EncodeToString(sum[:])}}
		versions[id] = modrinth.Version{ID: id + "-new", ProjectID: id, Channel: "release", Loaders: []string{"fabric"}, GameVersions: []string{"1.21"}, Files: []modrinth.File{file}}
		old := filepath.Join(t.TempDir(), id+"-old.jar")
		jar(t, old, nil)
		if err := s.Add(old); err != nil {
			t.Fatal(err)
		}
	}
	r, err := s.load()
	if err != nil {
		t.Fatal(err)
	}
	for i := range r.Mods {
		id := strings.TrimSuffix(r.Mods[i].File, "-old.jar")
		r.Mods[i].ProjectID, r.Mods[i].VersionID = id, id+"-old"
		if id == "library" {
			r.Mods[i].RequiredBy = []string{"A", "B", "outside"}
		}
	}
	if err = s.save(r); err != nil {
		t.Fatal(err)
	}
	a := versions["A"]
	a.Dependencies = []modrinth.Dependency{{ProjectID: "library", Type: "required"}}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/v2/project/") {
			_ = json.NewEncoder(w).Encode([]modrinth.Version{versions["library"]})
			return
		}
		_, _ = w.Write(bodies[filepath.Base(req.URL.Path)])
	}))
	defer server.Close()
	transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}
	client := &modrinth.Client{HTTP: &http.Client{Transport: transport}}
	plan, err := client.ResolveBatch(context.Background(), []modrinth.Version{a, versions["B"]}, "fabric", "1.21", "release", map[string]string{"A": "A-old", "B": "B-old", "library": "library-old"})
	if err != nil || len(plan.Targets) != 2 || len(plan.RequiredDeps) != 1 {
		t.Fatalf("batch graph: %+v %v", plan, err)
	}
	if err = s.Install(context.Background(), client, plan, nil, false, nil); err != nil {
		t.Fatal(err)
	}
	items, err := s.Scan()
	if err != nil || len(items) != 3 {
		t.Fatalf("incomplete update: %+v %v", items, err)
	}
	for _, item := range items {
		if item.VersionID != item.ProjectID+"-new" || item.Changed {
			t.Fatalf("wrong published version: %+v", item)
		}
		if item.ProjectID == "library" && (!slices.Contains(item.RequiredBy, "A") || !slices.Contains(item.RequiredBy, "outside") || slices.Contains(item.RequiredBy, "B")) {
			t.Fatalf("stale shared owners: %+v", item.RequiredBy)
		}
	}
	if history, err := s.History(); err != nil || len(history) != 3 {
		t.Fatalf("missing rollback history: %+v %v", history, err)
	}
}

func TestInstallRejectsExternalEditDuringDownload(t *testing.T) {
	s := store(t)
	old := filepath.Join(t.TempDir(), "old.jar")
	jar(t, old, nil)
	if err := s.Add(old); err != nil {
		t.Fatal(err)
	}
	r, err := s.load()
	if err != nil {
		t.Fatal(err)
	}
	r.Mods[0].ProjectID, r.Mods[0].VersionID = "project", "old"
	if err = s.save(r); err != nil {
		t.Fatal(err)
	}
	newJar := filepath.Join(t.TempDir(), "new.jar")
	jar(t, newJar, nil)
	body, err := os.ReadFile(newJar)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { _, _ = w.Write(body) }))
	defer server.Close()
	transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}
	sum := sha512.Sum512(body)
	file := modrinth.File{URL: "https://cdn.modrinth.net/new.jar", Filename: "new.jar", Size: int64(len(body)), Hashes: modrinth.Hashes{SHA512: hex.EncodeToString(sum[:])}}
	version := modrinth.Version{ID: "new", ProjectID: "project", Channel: "release", Files: []modrinth.File{file}}
	plan := modrinth.InstallPlan{Targets: []modrinth.PlanItem{{Version: version, File: file, InstalledVersion: "old"}}}
	changed := []byte("edited outside the lock during download")
	err = s.Install(context.Background(), &modrinth.Client{HTTP: &http.Client{Transport: transport}}, plan, nil, true, func(n, _ int64) {
		if n == int64(len(body)) {
			if writeErr := os.WriteFile(filepath.Join(s.GameDir, "mods", "old.jar"), changed, 0600); writeErr != nil {
				t.Error(writeErr)
			}
		}
	})
	if err == nil || !strings.Contains(err.Error(), "changed during download") {
		t.Fatalf("external change overwritten: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(s.GameDir, "mods", "old.jar"))
	if err != nil || string(got) != string(changed) {
		t.Fatalf("external file lost: %q %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(s.GameDir, "mods", "new.jar")); !os.IsNotExist(err) {
		t.Fatalf("new version published: %v", err)
	}
	if history, err := s.History(); err != nil || len(history) != 0 {
		t.Fatalf("spurious backup: %+v %v", history, err)
	}
}

func TestLegacyRegistryUpgrade(t *testing.T) {
	s := store(t)
	src := filepath.Join(t.TempDir(), "old.jar")
	jar(t, src, nil)
	if err := s.Add(src); err != nil {
		t.Fatal(err)
	}
	r, err := s.load()
	if err != nil {
		t.Fatal(err)
	}
	r.SchemaVersion = 1
	data := []byte(`{"schema_version":1,"mods":`)
	// Existing 5B metadata is accepted and migrated only on the next write.
	entries, err := json.Marshal(r.Mods)
	if err != nil {
		t.Fatal(err)
	}
	data = append(append(data, entries...), '}')
	if err = os.WriteFile(s.Registry, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Scan(); err != nil {
		t.Fatal(err)
	}
	if err = s.save(r); err != nil {
		t.Fatal(err)
	}
	upgraded, err := s.load()
	if err != nil || upgraded.SchemaVersion != 2 {
		t.Fatalf("upgrade %+v %v", upgraded, err)
	}
}
