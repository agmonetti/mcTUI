package ui

import (
	"archive/zip"
	"bytes"
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
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"

	"mcTUI/internal/instances"
	"mcTUI/internal/modpacks"
	"mcTUI/internal/modrinth"
)

func TestModpacksUINavigationAndImport(t *testing.T) {
	root := t.TempDir()

	// Crear un mrpack fixture válido
	idx := modpacks.Index{
		FormatVersion: 1,
		Game:          "minecraft",
		VersionID:     "pack-v1",
		Name:          "Adventure Pack",
		Dependencies: map[string]string{
			"minecraft":     "1.21.1",
			"fabric-loader": "0.16.0",
		},
		Files: []modpacks.File{},
	}
	idxBytes, _ := json.Marshal(idx)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	iw, _ := zw.Create("modrinth.index.json")
	iw.Write(idxBytes)
	ow, _ := zw.Create("overrides/config/adventure.cfg")
	ow.Write([]byte("adventure=true\n"))
	zw.Close()

	packPath := filepath.Join(root, "test.mrpack")
	if err := os.WriteFile(packPath, buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}

	m := Model{
		state:    menuScreen,
		dataRoot: root,
		width:    100,
		height:   30,
		memoryMB: 2048,
		instance: instances.Instance{ID: "legacy", Kind: "legacy", Name: "Instalación existente", MinecraftVersion: "26.2", Loader: "Vanilla", MemoryMB: 2048},
	}

	// 1. Tecla 'm': entra a modrinthHubScreen
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = next.(Model)
	if m.state != modrinthHubScreen {
		t.Fatalf("expected state modrinthHubScreen, got %v", m.state)
	}

	// 2. Cursor en Modpacks (posición 3: Mods(0), Resource(1), Shader(2), Modpacks(3))
	m.hubCursor = 3
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.state != modpackScreen || m.mp.step != "home" {
		t.Fatalf("expected state modpackScreen step home, got %v / %v", m.state, m.mp.step)
	}

	// 3. Tecla 'l': pide ruta local de archivo
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = next.(Model)
	if m.mp.step != "input" || m.mp.action != "local" {
		t.Fatalf("expected input local, got %v / %v", m.mp.step, m.mp.action)
	}

	// 4. Ingresar ruta y dar Enter: carga e inspecciona
	m.mp.input.SetValue(packPath)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil || m.mp.step != "working" {
		t.Fatalf("expected working inspect, got %v", m.mp.step)
	}
	msg := cmd()
	next, _ = m.Update(msg)
	m = next.(Model)
	if m.mp.step != "review" || m.mp.archive.Index.Name != "Adventure Pack" {
		t.Fatalf("expected review Adventure Pack, got %v / %v", m.mp.step, m.mp.archive.Index.Name)
	}
	m.mp.store.Prepare = func(ctx context.Context, runtimeDir, version, loader, loaderVersion string) error {
		return nil
	}

	// 5. En review, presionar Enter para instalar en instancia nueva
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil || m.mp.step != "working" {
		t.Fatalf("expected working install, got %v", m.mp.step)
	}
	msg = cmd().(tea.BatchMsg)[0]()
	next, _ = m.Update(msg)
	m = next.(Model)
	if m.mp.step != "result" || m.mp.err != nil {
		t.Fatalf("expected success result, got step %v, err %v", m.mp.step, m.mp.err)
	}

	// Comprobar que la nueva instancia quedó seleccionada y persistida
	if m.instance.Kind != "managed" || m.instance.Name != "Adventure Pack" ||
		m.instance.MinecraftVersion != "1.21.1" || m.instance.Loader != "Fabric" {
		t.Fatalf("expected selected instance with pack runtime, got %+v", m.instance)
	}

	// Comprobar que renderModpacks muestra mensaje de éxito
	rendered := m.renderModpacks()
	if !strings.Contains(rendered, "Modpack instalado") {
		t.Fatalf("renderModpacks did not show success: %s", rendered)
	}
}

func TestModpackVersionCancellationReturnsToReleasesWithoutError(t *testing.T) {
	m := Model{
		state:    modpackScreen,
		dataRoot: t.TempDir(),
		width:    80,
		mp: modpackState{
			step:      "releases",
			projectID: "selected-pack",
			versions:  []modrinth.Version{{ID: "selected-release", Name: "Selected release"}},
			store:     modpacks.Store{Root: t.TempDir()},
		},
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil || m.mp.step != "working" || !strings.Contains(m.renderModpacks(), "Descargando e inspeccionando") {
		t.Fatalf("selecting a version did not show staging: %s", m.renderModpacks())
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if !strings.Contains(m.renderModpacks(), "Cancelando") {
		t.Fatalf("cancellation was not visible: %s", m.renderModpacks())
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.mp.step != "releases" || m.mp.err != nil || strings.Contains(m.renderModpacks(), "Error") {
		t.Fatalf("cancelled staging did not restore releases: %s", m.renderModpacks())
	}
}

func TestModpackInstallCancellationKeepsReviewAndCleansTemporaryStage(t *testing.T) {
	root := t.TempDir()
	index, err := json.Marshal(modpacks.Index{
		FormatVersion: 1, Game: "minecraft", VersionID: "version-1", Name: "Test pack",
		Dependencies: map[string]string{"minecraft": "1.21.1"}, Files: []modpacks.File{},
	})
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	zipWriter := zip.NewWriter(&archive)
	entry, err := zipWriter.Create("modrinth.index.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(index); err != nil {
		t.Fatal(err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "test.mrpack")
	if err := os.WriteFile(path, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	pack, err := modpacks.Inspect(path)
	if err != nil {
		t.Fatal(err)
	}
	preparing := make(chan struct{})
	m := Model{
		state: modpackScreen, dataRoot: root, width: 80, memoryMB: 2048,
		mp: modpackState{
			step: "review", mode: "import", name: "Test pack", archive: pack,
			source: modpacks.Source{Type: "local"},
			store: modpacks.Store{Root: root, Prepare: func(ctx context.Context, _, _, _, _ string) error {
				close(preparing)
				<-ctx.Done()
				return ctx.Err()
			}},
		},
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil || !strings.Contains(m.renderModpacks(), "Instalando modpack") {
		t.Fatalf("install did not render working state: %s", m.renderModpacks())
	}
	work := cmd().(tea.BatchMsg)[0]
	result := make(chan tea.Msg, 1)
	go func() { result <- work() }()
	select {
	case <-preparing:
	case <-time.After(5 * time.Second):
		t.Fatal("installation did not reach runtime preparation")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	next, _ = m.Update(<-result)
	m = next.(Model)
	if m.mp.step != "review" || m.mp.err != nil || !strings.Contains(m.renderModpacks(), "Test pack") {
		t.Fatalf("cancelled install did not restore review: %s", m.renderModpacks())
	}
	if stages, err := filepath.Glob(filepath.Join(root, ".mctui-modpack-*")); err != nil || len(stages) != 0 {
		t.Fatalf("cancelled install left staging directories: %v, %v", stages, err)
	}
	list, err := instances.List(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range list {
		if in.Kind == "managed" {
			t.Fatalf("cancelled install published an instance: %+v", in)
		}
	}
}

func TestModpackUpdateReviewShowsEveryChangeAndRequiresExplicitConflictChoice(t *testing.T) {
	m := Model{
		state:  modpackScreen,
		width:  100,
		height: 26,
		mp: modpackState{
			step:      "working",
			mode:      "update",
			request:   1,
			decisions: map[string]bool{},
		},
	}
	plan := modpacks.UpdatePlan{
		Changes: []modpacks.UpdatePath{
			{Path: "config/removed.cfg", Upstream: "removed", LocalChange: "unchanged"},
			{Path: "config/custom.cfg", Upstream: "changed", LocalChange: "changed", Conflict: true},
		},
		Conflicts: []modpacks.UpdatePath{{Path: "config/custom.cfg", Conflict: true}},
	}
	next, _ := m.Update(modpackMsg{request: 1, kind: "plan", plan: plan})
	m = next.(Model)
	if m.mp.step != "conflicts" {
		t.Fatalf("plan was not shown for review: %s", m.mp.step)
	}
	if view := m.renderModpacks(); !strings.Contains(view, "config/removed.cfg") || !strings.Contains(view, "config/custom.cfg") {
		t.Fatalf("update review omitted a changed path: %s", view)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(Model)
	if m.mp.decisions["config/custom.cfg"] {
		t.Fatal("space on an automatic change overwrote a different conflict")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(Model)
	if !m.mp.decisions["config/custom.cfg"] || !strings.Contains(m.renderModpacks(), "[Sobrescribir]") {
		t.Fatal("selected conflict did not receive a visible overwrite decision")
	}
}

func TestModpackRecoveryReportsCorruptJournalWithoutDeletingIt(t *testing.T) {
	root := t.TempDir()
	in, err := instances.Create(root, "Pack en prueba", "1.20.4", "Vanilla", "", 2048)
	if err != nil {
		t.Fatal(err)
	}
	journal := filepath.Join(root, "instances", in.ID, ".mctui-pack-update.json")
	if err := os.WriteFile(journal, []byte("{invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	m := Model{state: modpackScreen, dataRoot: root, instance: in, mp: modpackState{step: "home", store: modpacks.Store{Root: root}}}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = next.(Model)
	if cmd == nil || m.mp.step != "working" {
		t.Fatalf("recover action not available from TUI: %s", m.mp.step)
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.mp.step != "result" || m.mp.err == nil || !strings.Contains(m.renderModpacks(), "Error") {
		t.Fatalf("corrupt journal was reported as recovered: %s", m.renderModpacks())
	}
	if _, err := os.Stat(journal); err != nil {
		t.Fatalf("recovery destroyed evidence of failure: %v", err)
	}
}

func TestModpacksHomeMenuAndSearchAppearance(t *testing.T) {
	root := t.TempDir()
	m := Model{
		state:    modpackScreen,
		dataRoot: root,
		width:    100,
		height:   30,
		instance: instances.Instance{ID: "legacy", Kind: "legacy", Name: "prueba con fabric", MinecraftVersion: "26.1", Loader: "Fabric", LoaderVersion: "0.19.5"},
		mp: modpackState{
			step:       "home",
			homeCursor: 0,
		},
	}

	// Comprobar apariencia de la pantalla inicial (idéntica a LOCAL MODS en Image #2)
	view := m.renderModpacks()
	if !strings.Contains(view, "MODPACKS") {
		t.Fatalf("missing title MODPACKS: %s", view)
	}
	if !strings.Contains(view, "INSTANCE") || !strings.Contains(view, "prueba con fabric") {
		t.Fatalf("missing INSTANCE metadata: %s", view)
	}
	if !strings.Contains(view, "VERSION") || !strings.Contains(view, "26.1 / Fabric 0.19.5") {
		t.Fatalf("missing VERSION metadata: %s", view)
	}
	if !strings.Contains(view, "►") || !strings.Contains(view, "Importar archivo .mrpack local") {
		t.Fatalf("missing cursor or first option: %s", view)
	}

	// Bajar cursor con tecla 'down'
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	if m.mp.homeCursor != 1 {
		t.Fatalf("expected homeCursor 1, got %d", m.mp.homeCursor)
	}

	// Pasar a la pantalla de búsqueda con resultados simulados (idéntica a Image #3)
	m.mp.step = "results"
	m.mp.query = "fabric"
	m.mp.results = modrinth.SearchResponse{
		Total: 10436,
		Hits: []modrinth.SearchResult{
			{Title: "Fabric API", Author: "modmuss50", Downloads: 262000000, Description: "Lightweight and modular..."},
			{Title: "Sodium", Author: "jellysquid3", Downloads: 231700000, Description: "A high-performance rend..."},
		},
	}
	searchView := m.renderModpacks()
	if !strings.Contains(searchView, "Name") || !strings.Contains(searchView, "Author") || !strings.Contains(searchView, "Downloads") || !strings.Contains(searchView, "Short description") {
		t.Fatalf("missing table headers: %s", searchView)
	}
	if !strings.Contains(searchView, "Fabric API") || !strings.Contains(searchView, "modmuss50") || !strings.Contains(searchView, "262.0M") {
		t.Fatalf("missing row content: %s", searchView)
	}
	if !strings.Contains(searchView, "Showing 2 of 10436 modpacks") {
		t.Fatalf("missing result count: %s", searchView)
	}
}

func TestModpacksReleasesCompactAndExpandable(t *testing.T) {
	root := t.TempDir()
	versions := make([]modrinth.Version, 28)
	for i := range 28 {
		versions[i] = modrinth.Version{
			ID:      fmt.Sprintf("v-%02d", i),
			Name:    fmt.Sprintf("Release 3.%d.0", 28-i),
			Channel: "release",
		}
	}

	m := Model{
		state:    modpackScreen,
		dataRoot: root,
		width:    100,
		height:   30,
		instance: instances.Instance{ID: "managed-1", Kind: "managed", Name: "prueba con fabric", MinecraftVersion: "26.1", Loader: "Fabric"},
		mp: modpackState{
			step:            "releases",
			projectID:       "fabulously-optimized",
			versions:        versions,
			showAllVersions: false,
			cursor:          0,
		},
	}

	// 1. Modo compacto inicial: debe mostrar exactamente las 3 primeras y la opción de expandir
	view := m.renderModpacks()
	if !strings.Contains(view, "Release 3.28.0") || !strings.Contains(view, "Release 3.27.0") || !strings.Contains(view, "Release 3.26.0") {
		t.Fatalf("missing top 3 releases: %s", view)
	}
	if strings.Contains(view, "Release 3.10.0") {
		t.Fatalf("unexpected deep release in compact view: %s", view)
	}
	if !strings.Contains(view, "Ver todo el historial") || !strings.Contains(view, "28 disponibles") {
		t.Fatalf("missing expandable row hint: %s", view)
	}

	// 2. Presionar 'v' o 'Tab' para expandir el desplegable completo
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	m = next.(Model)
	if !m.mp.showAllVersions {
		t.Fatal("showAllVersions was not toggled by 'v'")
	}

	// En vista expandida: paginada con catalogPage (no más de 8 filas impresas a la vez)
	expView := m.renderModpacks()
	if !strings.Contains(expView, "Historial completo") || !strings.Contains(expView, "Showing 8 of 28 versions") {
		t.Fatalf("expected paginated full history: %s", expView)
	}

	// 3. Presionar 'v' o 'Tab' nuevamente: vuelve al modo compacto
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	m = next.(Model)
	if m.mp.showAllVersions {
		t.Fatal("showAllVersions was not collapsed by 'v'")
	}
}

func TestModpackSelectMinecraftIndependentOfCurrentInstance(t *testing.T) {
	var searchFiltered, releasesFiltered bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/search":
			searchFiltered = strings.Contains(r.URL.Query().Get("facets"), "versions:")
			fmt.Fprint(w, `{"hits":[{"project_id":"pack","title":"Pack"}],"total_hits":1}`)
		case "/v2/project/pack":
			fmt.Fprint(w, `{"id":"pack","project_type":"modpack"}`)
		case "/v2/project/pack/version":
			releasesFiltered = r.URL.Query().Has("game_versions") || r.URL.Query().Has("loaders")
			fmt.Fprint(w, `[{"id":"old","name":"Old release","game_versions":["1.21"],"loaders":["fabric"]},{"id":"new","name":"New release","game_versions":["26.2"],"loaders":["neoforge"]}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	previous := catalogAPI
	catalogAPI = &modrinth.Client{HTTP: &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		},
	}}}
	defer func() { catalogAPI = previous }()

	m := Model{state: modpackScreen, dataRoot: t.TempDir(), width: 100, height: 30,
		instance: instances.Instance{Name: "Current", MinecraftVersion: "1.21", Loader: "Fabric"},
		mp:       modpackState{step: "home"}}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)
	if searchFiltered || m.mp.step != "results" || len(m.mp.results.Hits) != 1 {
		t.Fatalf("search excluded packs outside current version: %s", m.renderModpacks())
	}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)
	if releasesFiltered || m.mp.step != "minecraft" || len(m.mp.minecraftVersions) != 2 {
		t.Fatalf("versions restricted to current runtime: %s", m.renderModpacks())
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.mp.step != "releases" || m.mp.selectedMinecraft != "26.2" ||
		len(m.modpackVisibleVersions()) != 1 || m.modpackVisibleVersions()[0].ID != "new" ||
		!strings.Contains(m.renderModpacks(), "neoforge") {
		t.Fatalf("wrong Minecraft release selected: %s", m.renderModpacks())
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.mp.step != "minecraft" {
		t.Fatalf("could not change Minecraft version: %s", m.mp.step)
	}
}

func TestModpacksTableCJKWidthNoWrap(t *testing.T) {
	root := t.TempDir()
	m := Model{
		state:    modpackScreen,
		dataRoot: root,
		width:    100,
		height:   30,
		instance: instances.Instance{ID: "inst-1", Kind: "managed", Name: "Fabulously Optimized", MinecraftVersion: "26.3", Loader: "Fabric"},
		mp: modpackState{
			step:  "results",
			query: "redstone",
			results: modrinth.SearchResponse{
				Total: 1,
				Hits: []modrinth.SearchResult{
					{
						Title:       "红石生电优化【Redstone Survival Optimization】",
						Author:      "moonboss666",
						Downloads:   1300000,
						Description: "一个红石玩家必备的整合包，轻，快，小，是他的特点！A must-have integration package for a Redstone player, light, fast, and small, are his characteristics!",
					},
				},
			},
		},
	}

	view := m.renderModpacks()
	maxAllowed := m.catalogWidth()
	for _, line := range strings.Split(view, "\n") {
		w := runewidth.StringWidth(line)
		if w > maxAllowed {
			t.Fatalf("line visual width %d exceeds catalog width %d: %q", w, maxAllowed, line)
		}
	}
}

func TestModpackInstallIntoCurrentReviewsAndAppliesPerPathDecisions(t *testing.T) {
	root := t.TempDir()
	in, err := instances.Create(root, "Actual", "1.21.1", "Fabric", "0.16.0", 2048)
	if err != nil {
		t.Fatal(err)
	}
	gameDir, err := instances.GameDir(root, in)
	if err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string]string{
		"config/keep.cfg": "my settings", "config/replace.cfg": "old settings",
	} {
		dest := filepath.Join(gameDir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dest, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	idx := modpacks.Index{
		FormatVersion: 1, Game: "minecraft", Name: "New Pack", VersionID: "pack-v1",
		Dependencies: map[string]string{"minecraft": "1.21.1", "fabric-loader": "0.16.0"},
		Files:        []modpacks.File{},
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	index, err := zw.Create("modrinth.index.json")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(idx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := index.Write(data); err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string]string{
		"config/keep.cfg": "pack settings", "config/replace.cfg": "new settings", "config/added.cfg": "new file",
	} {
		entry, err := zw.Create("overrides/" + path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	packPath := filepath.Join(root, "pack.mrpack")
	if err := os.WriteFile(packPath, buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	pack, err := modpacks.Inspect(packPath)
	if err != nil {
		t.Fatal(err)
	}
	m := Model{state: modpackScreen, dataRoot: root, width: 100, height: 30, instance: in,
		mp: modpackState{step: "review", mode: "import", name: "New Pack", archive: pack,
			source: modpacks.Source{Type: "local"}, optional: map[string]bool{}, store: modpacks.Store{Root: root}}}
	if view := m.renderModpacks(); !strings.Contains(view, "Actual") || !strings.Contains(view, "Fabric 0.16.0") ||
		!strings.Contains(view, "[i] Comparar rutas") || !strings.Contains(view, "[Enter] Instalar en instancia nueva") {
		t.Fatalf("destination choices or runtime missing: %s", view)
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = next.(Model)
	if cmd == nil || m.mp.step != "working" || m.mp.busy != "plan" || m.mp.mode != "into" {
		t.Fatalf("current-instance selection did not plan: %s", m.mp.step)
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.mp.step != "conflicts" || len(m.mp.plan.Changes) != 3 || len(m.mp.conflicts) != 2 {
		t.Fatalf("incoming paths and conflicts not reviewable: %+v; err=%v", m.mp.plan, m.mp.err)
	}
	if view := m.renderModpacks(); !strings.Contains(view, "config/keep.cfg") ||
		!strings.Contains(view, "config/replace.cfg") || !strings.Contains(view, "config/added.cfg") ||
		!strings.Contains(view, "Conservar local") || !strings.Contains(view, "Destino: Actual") {
		t.Fatalf("path review incomplete: %s", view)
	}
	for i, change := range m.mp.plan.Changes {
		if change.Path == "config/replace.cfg" {
			m.mp.cursor = i
			next, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
			m = next.(Model)
		}
	}
	if !m.mp.decisions["config/replace.cfg"] || m.mp.decisions["config/keep.cfg"] {
		t.Fatalf("overwrite decision leaked into preserved path: %+v", m.mp.decisions)
	}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil || m.mp.step != "working" {
		t.Fatalf("approved installation not started: %s", m.mp.step)
	}
	next, _ = m.Update(cmd().(tea.BatchMsg)[0]())
	m = next.(Model)
	if m.mp.step != "result" || m.mp.err != nil || m.instance.ID != in.ID {
		t.Fatalf("current installation failed or changed selection: step=%s err=%v instance=%+v", m.mp.step, m.mp.err, m.instance)
	}
	for path, want := range map[string]string{
		"config/keep.cfg": "my settings", "config/replace.cfg": "new settings", "config/added.cfg": "new file",
	} {
		got, err := os.ReadFile(filepath.Join(gameDir, filepath.FromSlash(path)))
		if err != nil || string(got) != want {
			t.Fatalf("%s: got %q, want %q, err=%v", path, got, want, err)
		}
	}
	list, err := instances.List(root)
	if err != nil || len(list) != 2 {
		t.Fatalf("install into current created an instance: %v, %v", list, err)
	}
}

func TestModpackCurrentTargetRequiresExactRuntimeAndNoExistingPack(t *testing.T) {
	root := t.TempDir()
	in, err := instances.Create(root, "Actual", "1.21.1", "Fabric", "0.16.0", 2048)
	if err != nil {
		t.Fatal(err)
	}
	m := Model{state: modpackScreen, dataRoot: root, instance: in,
		mp: modpackState{step: "review", mode: "import", archive: modpacks.Archive{Index: modpacks.Index{
			Dependencies: map[string]string{"minecraft": "1.21.1", "fabric-loader": "0.16.0"},
		}}}}
	for _, changed := range []instances.Instance{
		func() instances.Instance { c := in; c.MinecraftVersion = "1.20.4"; return c }(),
		func() instances.Instance { c := in; c.Loader = "Vanilla"; c.LoaderVersion = ""; return c }(),
		func() instances.Instance { c := in; c.LoaderVersion = "0.15.0"; return c }(),
		func() instances.Instance { c := in; c.LoaderVersion = ""; return c }(),
	} {
		m.instance = changed
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
		m = next.(Model)
		if cmd != nil || m.mp.step != "review" || m.mp.mode != "import" ||
			!strings.Contains(m.renderModpacks(), "No disponible:") {
			t.Fatalf("runtime mismatch offered current destination: %+v", changed)
		}
	}
	m.instance = in
	gameDir, err := instances.GameDir(root, in)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(gameDir), "pack.json"), []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = next.(Model)
	if cmd != nil || m.mp.mode != "import" || !strings.Contains(m.renderModpacks(), "[u] Actualizar") {
		t.Fatal("existing pack offered an install instead of the update flow")
	}
}

func TestModpackLegacyVanillaCanChooseCurrentWithoutAssumingFabricVersion(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", home)
	gameDir := filepath.Join(home, ".minecraft")
	if err := os.MkdirAll(gameDir, 0700); err != nil {
		t.Fatal(err)
	}
	m := Model{state: modpackScreen, dataRoot: root,
		instance: instances.Instance{ID: instances.LegacyID, Kind: "legacy", Name: "Existente",
			MinecraftVersion: "1.21.1", Loader: "Vanilla"},
		mp: modpackState{step: "review", mode: "import", archive: modpacks.Archive{Index: modpacks.Index{
			Dependencies: map[string]string{"minecraft": "1.21.1"},
		}}}}
	if reason := m.modpackCurrentTargetReason(); reason != "" {
		t.Fatalf("matching Vanilla legacy target unavailable: %s", reason)
	}
	m.instance.Loader = "Fabric"
	if reason := m.modpackCurrentTargetReason(); reason == "" {
		t.Fatal("legacy Fabric target without loader version was treated as an exact match")
	}
	manifestDir := filepath.Join(root, "legacy-pack")
	if err := os.Mkdir(manifestDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(manifestDir, "pack.json"), []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	m.instance.Loader = "Vanilla"
	m.mp.step = "home"
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	m = next.(Model)
	if m.mp.step != "input" || m.mp.mode != "update" {
		t.Fatalf("legacy pack could not enter existing update flow: %s", m.mp.step)
	}
}

func TestModpackLocalFileInputDisplaysEditablePath(t *testing.T) {
	m := Model{state: modpackScreen, mp: modpackState{step: "home"}}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = next.(Model)
	m.mp.input.SetValue("/tmp/sample.mrpack")
	if m.mp.step != "input" || !strings.Contains(m.renderModpacks(), "/tmp/sample.mrpack") {
		t.Fatalf("local pack path is not visible while editing: %s", m.renderModpacks())
	}
}
