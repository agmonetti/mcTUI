package ui

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha512"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"mcTUI/internal/i18n"
	"mcTUI/internal/instances"
	"mcTUI/internal/modrinth"
	"mcTUI/internal/packs"
)

func packFixture(t *testing.T, kind packs.Kind) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	entries := map[string]string{"shaders/example.vsh": "void main() {}"}
	if kind == packs.Resource {
		entries = map[string]string{
			"pack.mcmeta": `{"pack":{"description":"Test","min_format":[69,0],"max_format":[69,1]}}`,
			"assets/minecraft/textures/block/stone.png": "data",
		}
	}
	for name, data := range entries {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPacksNavigationInstanceIsolation(t *testing.T) {
	if err := i18n.Init("es"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = i18n.Init("en") })
	root := t.TempDir()
	first, err := instances.Create(root, "Primera", "1.21.9", "Vanilla", "", 2048)
	if err != nil {
		t.Fatal(err)
	}
	second, err := instances.Create(root, "Segunda", "1.21.9", "Vanilla", "", 2048)
	if err != nil {
		t.Fatal(err)
	}
	firstDir, err := instances.GameDir(root, first)
	if err != nil {
		t.Fatal(err)
	}
	secondDir, err := instances.GameDir(root, second)
	if err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "example.zip")
	if err := os.WriteFile(archive, packFixture(t, packs.Resource), 0600); err != nil {
		t.Fatal(err)
	}
	if err := (packs.Store{GameDir: firstDir}).Add(packs.Resource, archive); err != nil {
		t.Fatal(err)
	}
	m := Model{state: menuScreen, cursorMenu: 2, dataRoot: root, instance: first, width: 120, height: 35}
	var cmd tea.Cmd
	var next tea.Model
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.state != modrinthHubScreen || cmd != nil {
		t.Fatalf("Modrinth entry: state=%v", m.state)
	}
	if got := m.View(); !bytes.Contains([]byte(got), []byte("Paquetes de recursos")) || !bytes.Contains([]byte(got), []byte("Shaders")) {
		t.Fatal("hub omits pack categories")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.state != packWorkingScreen || cmd == nil {
		t.Fatalf("resource list did not load: %v", m.state)
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.state != packListScreen || len(m.packItems) != 1 || m.packItems[0].File != "example.zip" || m.packItems[0].Problem != "" {
		t.Fatalf("resource list: state=%v items=%+v err=%v", m.state, m.packItems, m.packErr)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = next.(Model)
	if m.state != packConfirmScreen {
		t.Fatalf("delete without consent: %v", m.state)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if _, err := os.Stat(filepath.Join(firstDir, "resourcepacks", "example.zip")); err != nil {
		t.Fatalf("cancel changed pack: %v", err)
	}
	m.instance = second
	next, cmd = m.openPacks(packs.Resource)
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)
	if len(m.packItems) != 0 {
		t.Fatalf("other instance shows packs: %+v", m.packItems)
	}
	if _, err := os.Stat(filepath.Join(secondDir, "resourcepacks", "example.zip")); !os.IsNotExist(err) {
		t.Fatalf("other instance modified: %v", err)
	}
}

func TestModrinthShaderDownloadPublishesOnlyVerifiedZIP(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = i18n.Init("en") })
	root := t.TempDir()
	instance, err := instances.Create(root, "Shaders", "1.21.9", "Fabric", "0.16.0", 2048)
	if err != nil {
		t.Fatal(err)
	}
	gameDir, err := instances.GameDir(root, instance)
	if err != nil {
		t.Fatal(err)
	}
	data := packFixture(t, packs.Shader)
	sha := sha512.Sum512(data)
	version := modrinth.Version{ID: "version", ProjectID: "shader-id", Channel: "release", GameVersions: []string{"1.21.9"}, Loaders: []string{"iris"}, Files: []modrinth.File{{Filename: "test-shaders.zip", Primary: true, Size: int64(len(data)), Hashes: modrinth.Hashes{SHA512: hex.EncodeToString(sha[:])}, URL: "https://cdn.modrinth.com/file.zip"}}}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/search":
			json.NewEncoder(w).Encode(modrinth.SearchResponse{Hits: []modrinth.SearchResult{{ProjectID: "shader-id", ProjectType: "shader", Title: "Test Shader"}}, Total: 1})
		case "/v2/project/shader-id":
			json.NewEncoder(w).Encode(modrinth.Project{ID: "shader-id", ProjectType: "shader", Title: "Test Shader"})
		case "/v2/project/shader-id/version":
			json.NewEncoder(w).Encode([]modrinth.Version{version})
		case "/file.zip":
			w.Write(data)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	prior := catalogAPI
	catalogAPI = &modrinth.Client{HTTP: &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}}}
	defer func() { catalogAPI = prior }()
	m := Model{state: packListScreen, packKind: packs.Shader, packStore: packs.Store{GameDir: gameDir}, instance: instance}
	m.packEngine = "iris"
	m.startPackRead()
	m.packBusy, m.packReturn, m.state = true, packSearchScreen, packWorkingScreen
	search := m.packSearch()()
	next, _ := m.Update(search)
	m = next.(Model)
	if m.state != packSearchScreen || len(m.packResults.Hits) != 1 {
		t.Fatalf("search: state=%v results=%+v err=%v", m.state, m.packResults, m.packErr)
	}
	m.startPackRead()
	m.packReturn, m.state = packDetailScreen, packWorkingScreen
	next, _ = m.Update(m.packDetail()())
	m = next.(Model)
	if m.state != packDetailScreen || m.packVersion.ID != "version" {
		t.Fatalf("detail: state=%v version=%+v err=%v", m.state, m.packVersion, m.packErr)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = next.(Model)
	if m.state != packWorkingScreen || cmd == nil {
		t.Fatalf("download not started: %v", m.state)
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.state != packListScreen || m.packErr != nil || len(m.packItems) != 1 {
		t.Fatalf("download: state=%v err=%v items=%+v", m.state, m.packErr, m.packItems)
	}
	got, err := os.ReadFile(filepath.Join(gameDir, "shaderpacks", "test-shaders.zip"))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("published contents: err=%v", err)
	}
}

func TestModrinthResourcePackDownloadWithSupportedFormatsObject(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = i18n.Init("en") })
	root := t.TempDir()
	instance, err := instances.Create(root, "Resources", "1.21.1", "Vanilla", "", 2048)
	if err != nil {
		t.Fatal(err)
	}
	gameDir, err := instances.GameDir(root, instance)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	w, err := z.Create("pack.mcmeta")
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte(`{"pack":{"pack_format":15,"supported_formats":{"min_inclusive":15,"max_inclusive":200},"description":{"fallback":"Low on fire"}}}`))
	w2, err := z.Create("assets/minecraft/textures/block/fire.png")
	if err != nil {
		t.Fatal(err)
	}
	w2.Write([]byte("data"))
	z.Close()
	data := buf.Bytes()
	sha := sha512.Sum512(data)
	version := modrinth.Version{
		ID: "ver1", ProjectID: "rp-id", Channel: "release", GameVersions: []string{"1.21.1"}, Loaders: []string{"minecraft"},
		Files: []modrinth.File{{Filename: "low-on-fire.zip", Primary: true, Size: int64(len(data)), Hashes: modrinth.Hashes{SHA512: hex.EncodeToString(sha[:])}, URL: "https://cdn.modrinth.com/low-on-fire.zip"}},
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/low-on-fire.zip":
			w.Write(data)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	prior := catalogAPI
	catalogAPI = &modrinth.Client{HTTP: &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}}}
	defer func() { catalogAPI = prior }()

	m := Model{
		state:       packReviewScreen,
		packKind:    packs.Resource,
		packStore:   packs.Store{GameDir: gameDir},
		instance:    instance,
		packProject: modrinth.Project{ID: "rp-id", Title: "Low on Fire"},
		packVersion: version,
	}
	m.startPackRead()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = next.(Model)
	if m.state != packWorkingScreen || cmd == nil {
		t.Fatalf("install did not start: %v", m.state)
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.state != packListScreen || m.packErr != nil || len(m.packItems) != 1 {
		t.Fatalf("download failed: state=%v err=%v items=%+v", m.state, m.packErr, m.packItems)
	}
	if m.packItems[0].File != "low-on-fire.zip" || m.packItems[0].Problem != "" || m.packItems[0].FormatRange != "15–200" {
		t.Fatalf("installed pack metadata error: %+v", m.packItems[0])
	}
}

func TestPackPickerImportsUnpackedFolder(t *testing.T) {
	for _, tc := range []struct {
		kind packs.Kind
		name, entry, body string
	}{
		{packs.Shader, "my-shaders", "shaders/main.vsh", "void main() {}"},
		{packs.Resource, "my-resources", "assets/minecraft/lang/en_us.json", `{}`},
	} {
		t.Run(string(tc.kind), func(t *testing.T) {
			gameDir := t.TempDir()
			sourceRoot := t.TempDir()
			source := filepath.Join(sourceRoot, tc.name)
			if err := os.MkdirAll(filepath.Dir(filepath.Join(source, tc.entry)), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(source, tc.entry), []byte(tc.body), 0600); err != nil {
				t.Fatal(err)
			}
			if tc.kind == packs.Resource {
				if err := os.WriteFile(filepath.Join(source, "pack.mcmeta"), []byte(`{"pack":{"pack_format":34}}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			m := Model{
				state: packPickerScreen, packKind: tc.kind,
				packStore: packs.Store{GameDir: gameDir},
				pickerForPacks: true, pickerPath: sourceRoot,
			}
			m.loadPickerDir()
			next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
			m = next.(Model)
			if m.state != packWorkingScreen || cmd == nil {
				t.Fatalf("folder import did not start: state=%v err=%v", m.state, m.packErr)
			}
			next, _ = m.Update(cmd())
			m = next.(Model)
			if m.state != packListScreen || m.packErr != nil || len(m.packItems) != 1 || m.packItems[0].File != tc.name || m.packItems[0].Problem != "" {
				t.Fatalf("folder import failed: state=%v items=%+v err=%v", m.state, m.packItems, m.packErr)
			}
			if _, err := os.Stat(filepath.Join(source, tc.entry)); err != nil {
				t.Fatalf("source folder modified: %v", err)
			}
		})
	}
}

func TestPackListSearchTriggers(t *testing.T) {
	for _, key := range []string{"s", "b", "/", "enter"} {
		t.Run("key_"+key, func(t *testing.T) {
			m := Model{
				state:     packListScreen,
				packKind:  packs.Resource,
				packItems: nil, // empty list
			}
			next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
			if key == "enter" {
				next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			}
			m = next.(Model)
			if m.state != packWorkingScreen || m.packReturn != packSearchScreen || cmd == nil {
				t.Fatalf("key %q did not start search: state=%v return=%v cmd=%v", key, m.state, m.packReturn, cmd)
			}
		})
	}
}

func TestPackVisualLayoutMatchesMods(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = i18n.Init("en") })
	root := t.TempDir()
	instance, err := instances.Create(root, "MiInstancia", "1.21.1", "Fabric", "0.16.0", 2048)
	if err != nil {
		t.Fatal(err)
	}
	gameDir, err := instances.GameDir(root, instance)
	if err != nil {
		t.Fatal(err)
	}
	m := Model{
		state:     packListScreen,
		packKind:  packs.Shader,
		packStore: packs.Store{GameDir: gameDir},
		instance:  instance,
		packItems: []packs.Pack{
			{Name: "Complementary Shaders", File: "Complementary.zip"},
		},
		width:  100,
		height: 30,
	}
	view := m.View()
	for _, expected := range []string{
		"LOCAL SHADERS",
		"INSTANCE",
		"MiInstancia",
		"VERSION",
		"1.21.1",
		"PATH",
		"minecraft",
		"Installed",
		"Complementary Shaders",
		"1 files installed",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("pack view missing expected element %q:\n%s", expected, view)
		}
	}

	// Test resource packs (English)
	m.packKind = packs.Resource
	view = m.View()
	if !strings.Contains(view, "LOCAL RESOURCE PACKS") {
		t.Fatalf("resource pack view missing expected title:\n%s", view)
	}

	// Test Spanish locale
	if err := i18n.Init("es"); err != nil {
		t.Fatal(err)
	}
	viewES := m.View()
	for _, expected := range []string{
		"PAQUETES DE RECURSOS LOCALES",
		"INSTANCIA",
		"VERSIÓN",
		"RUTA",
		"Instalado",
		"1 archivos instalados",
	} {
		if !strings.Contains(viewES, expected) {
			t.Fatalf("Spanish pack view missing expected element %q:\n%s", expected, viewES)
		}
	}
	m.packKind = packs.Shader
	viewES = m.View()
	if !strings.Contains(viewES, "SHADERS LOCALES") {
		t.Fatalf("Spanish shader view missing expected title:\n%s", viewES)
	}
}

func TestPackReviewKeepsConfirmationVisibleWithLongDescription(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}
	m := Model{
		state: packReviewScreen, packKind: packs.Shader, width: 120, height: 32,
		instance:    instances.Instance{Name: "Test", MinecraftVersion: "1.21.1", Loader: "Fabric"},
		packProject: modrinth.Project{Title: "Test Shader", Description: strings.Repeat("Long shader description that wraps across the panel. ", 30)},
		packVersion: modrinth.Version{Number: "1.0", Files: []modrinth.File{{Filename: "shader.zip", Primary: true}}},
	}
	for _, state := range []screenState{packDetailScreen, packReviewScreen} {
		m.state = state
		view := m.View()
		if !strings.Contains(view, "shader.zip") {
			t.Fatalf("state %d lost file details:\n%s", state, view)
		}
		if state == packReviewScreen && (!strings.Contains(view, "Download Test Shader") || !strings.Contains(view, "[y] Confirm")) {
			t.Fatalf("review clipped the download decision:\n%s", view)
		}
	}
}
