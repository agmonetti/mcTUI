package ui

import (
	"archive/zip"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"mcTUI/internal/modrinth"
	"mcTUI/internal/mods"
)

func TestCatalogReleaseUpdateBehindBeta(t *testing.T) {
	root := t.TempDir()
	store := mods.Store{GameDir: filepath.Join(root, "minecraft"), Registry: filepath.Join(root, "mods.json")}
	if err := os.MkdirAll(filepath.Join(store.GameDir, "mods"), 0700); err != nil {
		t.Fatal(err)
	}
	jar := filepath.Join(root, "old.jar")
	f, err := os.Create(jar)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	entry, err := z.Create("fabric.mod.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = entry.Write([]byte(`{"id":"old","version":"1"}`)); err != nil {
		t.Fatal(err)
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if err = store.Add(jar); err != nil {
		t.Fatal(err)
	}
	items, err := store.Scan()
	if err != nil {
		t.Fatal(err)
	}
	record := items[0].Record
	record.ProjectID, record.VersionID = "project", "old"
	registry, _ := json.Marshal(map[string]any{"schema_version": 2, "mods": []mods.Record{record}})
	if err = os.WriteFile(store.Registry, registry, 0600); err != nil {
		t.Fatal(err)
	}
	file := modrinth.File{Filename: "new.jar", Size: 10, Hashes: modrinth.Hashes{SHA512: strings.Repeat("a", 128)}}
	version := func(id, channel string) modrinth.Version {
		return modrinth.Version{ID: id, ProjectID: "project", Channel: channel, Loaders: []string{"fabric"}, GameVersions: []string{"1.21"}, Files: []modrinth.File{file}}
	}
	beta, release := version("beta", "beta"), version("release", "release")
	requests := 0
	var currentRelease atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/version_files/update":
			json.NewEncoder(w).Encode(map[string]modrinth.Version{record.Hashes.SHA512: beta})
		case "/v2/project/project/version":
			requests++
			if currentRelease.Load() {
				oldRelease := release
				oldRelease.ID = "old"
				json.NewEncoder(w).Encode([]modrinth.Version{beta, oldRelease})
			} else {
				json.NewEncoder(w).Encode([]modrinth.Version{beta, release})
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	previous := catalogAPI
	catalogAPI = &modrinth.Client{HTTP: &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}}}
	defer func() { catalogAPI = previous }()
	m := Model{modStore: store}
	m.instance.Loader, m.instance.MinecraftVersion = "Fabric", "1.21"
	m.catalogContext = context.Background()
	msg := m.catalogCheckUpdates()().(catalogUpdatesMsg)
	if msg.Err != nil || len(msg.Versions) != 1 || msg.Versions[0].ID != "release" || requests != 1 {
		t.Fatalf("release hidden behind beta: %+v; version lookups: %d", msg, requests)
	}
	m.instance.ModChannel = "beta"
	msg = m.catalogCheckUpdates()().(catalogUpdatesMsg)
	if msg.Err != nil || len(msg.Versions) != 1 || msg.Versions[0].ID != "beta" || requests != 1 {
		t.Fatalf("beta channel: %+v; version lookups: %d", msg, requests)
	}
	// The release fallback must not advertise the already installed version.
	currentRelease.Store(true)
	m.instance.ModChannel = "release"
	msg = m.catalogCheckUpdates()().(catalogUpdatesMsg)
	if msg.Err != nil || len(msg.Versions) != 0 {
		t.Fatalf("current release advertised as update: %+v", msg)
	}
}

func TestCatalogOptionalReplacementConsent(t *testing.T) {
	m := Model{state: modrinthReviewScreen, catalogOptional: map[string]bool{"optional": true},
		catalogPlan: modrinth.InstallPlan{OptionalDeps: []modrinth.PlanItem{{Version: modrinth.Version{ProjectID: "optional"}, File: modrinth.File{Filename: "new.jar"}}}},
		modItems:    []mods.Mod{{Record: mods.Record{ProjectID: "optional", File: "old.jar"}, Changed: true}},
	}
	if view := m.renderCatalog(); !strings.Contains(view, "old.jar") || !strings.Contains(view, "new.jar") {
		t.Fatalf("replacement missing from review: %s", view)
	}
	next, cmd := m.updateCatalogKey("y")
	m = next.(Model)
	if cmd != nil || m.catalogErr == nil || m.state != modrinthReviewScreen {
		t.Fatal("modified optional was installed without consent")
	}
	next, _ = m.updateCatalogKey("m")
	m = next.(Model)
	next, cmd = m.updateCatalogKey("y")
	m = next.(Model)
	if cmd == nil || m.state != modrinthWorkingScreen {
		t.Fatal("explicit consent did not permit installation")
	}
	m.catalogCancel()
}

func TestCatalogOptionalConflictReview(t *testing.T) {
	m := Model{state: modrinthWorkingScreen, height: 22, catalogOptional: map[string]bool{}, catalogCursor: 9,
		catalogPlan: modrinth.InstallPlan{
			Targets:      []modrinth.PlanItem{{Version: modrinth.Version{ProjectID: "A"}}},
			OptionalDeps: []modrinth.PlanItem{{Version: modrinth.Version{ProjectID: "B"}, File: modrinth.File{Filename: "b.jar"}, IncompatibleIDs: []string{"A"}}},
		},
	}
	m, _, _ = m.updateCatalog(catalogPlanMsg{Plan: m.catalogPlan})
	if m.catalogCursor != 0 {
		t.Fatal("review cursor inherited search position")
	}
	next, _ := m.updateCatalogKey(" ")
	m = next.(Model)
	if !strings.Contains(m.renderCatalog(), "⚠") {
		t.Fatal("selected optional conflict not shown")
	}
	next, cmd := m.updateCatalogKey("y")
	m = next.(Model)
	if cmd != nil || m.catalogErr == nil || m.state != modrinthReviewScreen {
		t.Fatal("conflicting optional was sent to installer")
	}
	next, _ = m.updateCatalogKey(" ")
	m = next.(Model)
	if m.catalogErr != nil || m.catalogSelectedConflict() != nil {
		t.Fatal("deselecting optional did not clear conflict")
	}
}

func TestCatalogReviewShowsEffectiveDownloadsAndDestination(t *testing.T) {
	m := Model{state: modrinthReviewScreen, height: 28, width: 100,
		modStore:        mods.Store{GameDir: "/instances/A/minecraft"},
		catalogOptional: map[string]bool{},
		catalogPlan: modrinth.InstallPlan{
			Targets:      []modrinth.PlanItem{{Version: modrinth.Version{ProjectID: "A", Number: "2"}, File: modrinth.File{Filename: "a.jar", Size: 1024}}},
			RequiredDeps: []modrinth.PlanItem{{Version: modrinth.Version{ProjectID: "B", Number: "1"}, File: modrinth.File{Filename: "b.jar", Size: 2048}, AlreadyInstalled: true}},
			OptionalDeps: []modrinth.PlanItem{{Version: modrinth.Version{ProjectID: "C", Number: "3"}, File: modrinth.File{Filename: "c.jar", Size: 4096}}},
		},
	}
	view := m.renderCatalog()
	if !strings.Contains(view, "/instances/A/minecraft/mods") || !strings.Contains(view, "a.jar") ||
		!strings.Contains(view, "b.jar") || !strings.Contains(view, "1.0 KiB (1 ") {
		t.Fatalf("review omitted destination, installed dependency or effective size: %s", view)
	}
	next, _ := m.updateCatalogKey(" ")
	m = next.(Model)
	if view = m.renderCatalog(); !strings.Contains(view, "[x] c.jar") || !strings.Contains(view, "5.0 KiB (2 ") {
		t.Fatalf("optional selection did not update visible plan: %s", view)
	}
}

func TestCatalogUpdatesViewportTracksSelectionAndVersions(t *testing.T) {
	m := Model{state: modrinthUpdatesScreen, height: 19, width: 100,
		modStore:        mods.Store{Registry: "/instances/A/mods.json"},
		catalogSelected: map[string]bool{}}
	for i := range 8 {
		id := fmt.Sprintf("project-%d", i)
		m.catalogUpdates = append(m.catalogUpdates, modrinth.Version{ProjectID: id, Number: "new", Channel: "release", Files: []modrinth.File{{Filename: id + ".jar", Size: 1024}}})
		m.catalogUpdateRecords = append(m.catalogUpdateRecords, mods.Record{File: id + ".jar", VersionID: "old"})
	}
	for range 7 {
		next, _ := m.updateCatalogKey("down")
		m = next.(Model)
	}
	view := m.renderCatalog()
	if !strings.Contains(view, "► [ ] project-7: old → new · 1.0 KiB") || !strings.Contains(view, "/instances/A/backups/mods_replaced") {
		t.Fatalf("selected update or backup policy off screen: %s", view)
	}
}
func TestCatalogSelectionChangeRevokesOverwriteConsent(t *testing.T) {
	m := Model{state: modrinthReviewScreen, catalogModifiedConfirm: true,
		catalogOptional: map[string]bool{"optional": true},
		catalogPlan:     modrinth.InstallPlan{OptionalDeps: []modrinth.PlanItem{{Version: modrinth.Version{ProjectID: "optional"}}}},
	}
	next, _ := m.updateCatalogKey(" ")
	m = next.(Model)
	if m.catalogModifiedConfirm {
		t.Fatal("consent for one file set survived selection change")
	}
}

func TestCatalogSearchBackspaceRemovesWholeRune(t *testing.T) {
	m := Model{state: modrinthSearchScreen, catalogQuery: "café"}
	next, _ := m.updateCatalogKey("backspace")
	m = next.(Model)
	if m.catalogQuery != "caf" {
		t.Fatalf("backspace split unicode rune: %q", m.catalogQuery)
	}
	m.stopCatalogRead()
}

func TestCatalogDownloadProgressAndCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := Model{state: modrinthWorkingScreen, catalogCancel: cancel, catalogEvents: make(chan tea.Msg, 1), catalogTotal: 100}
	m, cmd, _ := m.updateCatalog(catalogProgressMsg{Downloaded: 42})
	if cmd == nil || !strings.Contains(m.renderCatalog(), "42 / 100 bytes") {
		t.Fatal("download progress was not displayed or polled")
	}
	next, _ := m.updateCatalogKey("esc")
	m = next.(Model)
	if ctx.Err() == nil || m.state != modrinthWorkingScreen {
		t.Fatal("cancel must wait for worker cleanup")
	}
	m.catalogEvents <- catalogInstalledMsg{Err: ctx.Err()}
	m, _, _ = m.updateCatalog(cmd())
	if m.state != modsScreen || m.modErr != context.Canceled || m.catalogEvents != nil {
		t.Fatal("cancel completion was not reported")
	}
}

func TestCatalogReadCancellationAndStaleMessages(t *testing.T) {
	m := Model{state: modrinthSearchScreen, catalogResults: modrinth.SearchResponse{Hits: []modrinth.SearchResult{{ProjectID: "first"}}}}
	m.startCatalogRead()
	old := m.catalogContext
	next, cmd := m.updateCatalogKey("enter")
	m = next.(Model)
	if cmd == nil || old.Err() != context.Canceled || m.catalogContext.Err() != nil {
		t.Fatal("opening detail must cancel search without canceling detail")
	}
	request := m.catalogRequest
	next, _ = m.updateCatalogKey("esc")
	m = next.(Model)
	if m.state != modsScreen || m.catalogContext != nil {
		t.Fatal("leaving working screen did not cancel read")
	}
	m.startCatalogRead()
	m.state = modrinthWorkingScreen
	m, _, _ = m.updateCatalog(catalogDetailMsg{Request: request, Project: modrinth.Project{Title: "stale"}})
	m, _, _ = m.updateCatalog(catalogPlanMsg{Request: request})
	m, _, _ = m.updateCatalog(catalogUpdatesMsg{Request: request})
	if m.state != modrinthWorkingScreen || m.catalogProject.Title != "" || m.catalogContext.Err() != nil {
		t.Fatal("stale read changed a newer operation")
	}
	m.stopCatalogRead()
	if got := cmd().(catalogDetailMsg); got.Err == nil || !strings.Contains(got.Err.Error(), "canceled") {
		t.Fatalf("canceled detail contacted API or lost cancellation: %v", got.Err)
	}
}

func TestCatalogPaginationAndStaleResults(t *testing.T) {
	m := Model{state: modrinthSearchScreen, catalogRequest: 3, height: 18}
	first := modrinth.SearchResponse{Hits: []modrinth.SearchResult{{Title: "first"}, {Title: "second"}}, Total: 4, Limit: 2}
	m, _, _ = m.updateCatalog(catalogSearchMsg{Request: 3, Results: first})
	if m.catalogOffset != 2 {
		t.Fatalf("first offset %d", m.catalogOffset)
	}
	next, cmd := m.updateCatalogKey("down")
	m = next.(Model)
	if cmd == nil || !m.catalogLoading {
		t.Fatal("page request missing")
	}
	next, cmd = m.updateCatalogKey("down")
	m = next.(Model)
	if cmd != nil {
		t.Fatal("duplicate page request")
	}
	page := modrinth.SearchResponse{Hits: []modrinth.SearchResult{{Title: "third"}, {Title: "fourth"}}, Total: 4, Limit: 2}
	m, _, _ = m.updateCatalog(catalogSearchMsg{Request: 3, Offset: 2, Results: page})
	if len(m.catalogResults.Hits) != 4 || m.catalogOffset != 4 || m.catalogLoading {
		t.Fatalf("bad page: %+v", m)
	}
	m.catalogCursor = 3
	if !strings.Contains(m.renderCatalog(), "fourth") {
		t.Fatal("selected item outside viewport")
	}
	m, _, _ = m.updateCatalog(catalogSearchMsg{Request: 2, Results: first})
	if len(m.catalogResults.Hits) != 4 {
		t.Fatal("stale search replaced results")
	}
}
