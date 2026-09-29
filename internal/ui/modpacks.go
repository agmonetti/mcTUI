package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"mcTUI/internal/i18n"
	"mcTUI/internal/instances"
	"mcTUI/internal/modpacks"
	"mcTUI/internal/modrinth"
)

type modpackHomeOption struct {
	key   string
	label string
}

var modpackHomeOptions = []modpackHomeOption{
	{"l", "Importar archivo .mrpack local"},
	{"h", "Importar desde URL HTTPS"},
	{"s", "Buscar modpacks en Modrinth"},
	{"u", "Actualizar modpack de esta instancia"},
	{"e", "Exportar instancia como .mrpack"},
	{"r", "Recuperar actualización interrumpida"},
}

type modpackState struct {
	step, action, mode string
	input              textinput.Model
	request            uint64
	cancel             context.CancelFunc
	cancelRequested    bool
	returnStep         string
	progress           *modpackProgress
	archive            modpacks.Archive
	cleanup            func()
	source             modpacks.Source
	name               string
	optional           map[string]bool
	cursor             int
	homeCursor         int
	query              string
	results            modrinth.SearchResponse
	versions           []modrinth.Version
	minecraftVersions  []string
	selectedMinecraft  string
	selectedVersions   []modrinth.Version
	showAllVersions    bool
	project            modrinth.Project
	projectID          string
	projectTitle       string
	plan               modpacks.UpdatePlan
	decisions          map[string]bool
	conflicts          []string
	overrides          []string
	selected           map[string]bool
	err                error
	message            string
	busy               string
	store              modpacks.Store
}

type modpackMsg struct {
	request   uint64
	kind      string
	archive   modpacks.Archive
	cleanup   func()
	source    modpacks.Source
	results   modrinth.SearchResponse
	versions  []modrinth.Version
	plan      modpacks.UpdatePlan
	overrides []string
	instance  instances.Instance
	err       error
}

// The installation command writes these counters off the event loop; only
// ticks and rendering read them.
type modpackProgress struct {
	done  atomic.Int64
	total atomic.Int64
}

type modpackProgressTickMsg struct{ request uint64 }

func modpackProgressTick(req uint64) tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return modpackProgressTickMsg{request: req}
	})
}

func (m Model) openModpacks() (tea.Model, tea.Cmd) {
	m.mp = modpackState{step: "home", homeCursor: 0, store: modpacks.Store{Root: m.dataRoot}}
	m.state = modpackScreen
	return m, nil
}

func (m *Model) stopModpack() {
	if m.mp.cancel != nil {
		m.mp.cancel()
		m.mp.cancel = nil
	}
	if m.mp.cleanup != nil {
		m.mp.cleanup()
		m.mp.cleanup = nil
	}
	m.mp.request++
}

func (m *Model) modpackInput(action, prompt, initial string) {
	input := textinput.New()
	input.Prompt = prompt
	input.CharLimit = 2048
	input.SetValue(initial)
	input.Focus()
	m.mp.action, m.mp.step, m.mp.input = action, "input", input
	m.mp.err = nil
}

func (m *Model) modpackWork(kind string) (context.Context, uint64) {
	if m.mp.cancel != nil {
		m.mp.cancel()
	}
	m.mp.returnStep = m.mp.step
	m.mp.request++
	ctx, cancel := context.WithCancel(context.Background())
	m.mp.cancel, m.mp.step, m.mp.busy, m.mp.err = cancel, "working", kind, nil
	m.mp.cancelRequested = false
	m.mp.progress = nil
	return ctx, m.mp.request
}

func (m Model) modpackLoad(path string, remote bool) (tea.Model, tea.Cmd) {
	ctx, req := m.modpackWork("inspect")
	store := m.mp.store
	return m, func() tea.Msg {
		var arc modpacks.Archive
		var cleanup func()
		var err error
		source := modpacks.Source{Type: "local"}
		if remote {
			arc, cleanup, err = store.StageURL(ctx, path, modpacks.Hashes{}, 0)
			source.Type = "url"
		} else {
			arc, err = store.Inspect(path)
		}
		return modpackMsg{request: req, kind: "inspect", archive: arc, source: source, cleanup: cleanup, err: err}
	}
}

func (m Model) modpackSearch() (tea.Model, tea.Cmd) {
	ctx, req := m.modpackWork("search")
	query := m.mp.query
	return m, func() tea.Msg {
		results, err := catalogAPI.SearchModpacks(ctx, query, "", 0)
		return modpackMsg{request: req, kind: "search", results: results, err: err}
	}
}

func (m Model) modpackReleases() (tea.Model, tea.Cmd) {
	ctx, req := m.modpackWork("releases")
	id := m.mp.projectID
	return m, func() tea.Msg {
		project, err := catalogAPI.Project(ctx, id)
		if err != nil {
			return modpackMsg{request: req, kind: "releases", err: err}
		}
		if project.ID != id || project.ProjectType != "modpack" {
			return modpackMsg{request: req, kind: "releases", err: fmt.Errorf("project is not a modpack")}
		}
		versions, err := catalogAPI.PackVersions(ctx, id)
		return modpackMsg{request: req, kind: "releases", versions: versions, err: err}
	}
}

func (m Model) modpackStageVersion() (tea.Model, tea.Cmd) {
	ctx, req := m.modpackWork("inspect")
	store, projectID, v := m.mp.store, m.mp.projectID, m.modpackVisibleVersions()[m.mp.cursor]
	return m, func() tea.Msg {
		arc, source, cleanup, err := store.StageVersion(ctx, catalogAPI, projectID, v.ID)
		return modpackMsg{request: req, kind: "inspect", archive: arc, source: source, cleanup: cleanup, err: err}
	}
}

func (m Model) modpackPlan() (tea.Model, tea.Cmd) {
	ctx, req := m.modpackWork("plan")
	store, target, arc, optional, mode := m.mp.store, m.instance, m.mp.archive, m.mp.optional, m.mp.mode
	return m, func() tea.Msg {
		var plan modpacks.UpdatePlan
		var err error
		if mode == "into" {
			plan, err = store.PlanInstall(ctx, target, arc, optional)
		} else {
			plan, err = store.PlanUpdate(ctx, target, arc, optional)
		}
		return modpackMsg{request: req, kind: "plan", plan: plan, err: err}
	}
}

func (m Model) modpackRun() (tea.Model, tea.Cmd) {
	ctx, req := m.modpackWork("install")
	store, arc, source := m.mp.store, m.mp.archive, m.mp.source
	name, options := m.mp.name, m.mp.optional
	memory := m.memoryMB
	if memory <= 0 {
		memory = 2048
	}
	target, mode, plan, choices := m.instance, m.mp.mode, m.mp.plan, m.mp.decisions
	progress := &modpackProgress{}
	m.mp.progress = progress
	work := func() tea.Msg {
		report := func(done, total int64) {
			progress.total.Store(total)
			progress.done.Store(done)
		}
		var in instances.Instance
		var err error
		if mode == "update" {
			err = store.Update(ctx, target, arc, source, options, plan, choices, report)
		} else if mode == "into" {
			err = store.InstallInto(ctx, target, arc, source, options, plan, choices, report)
		} else {
			in, err = store.Import(ctx, arc, source, name, memory, options, report)
		}
		return modpackMsg{request: req, kind: "install", instance: in, err: err}
	}
	return m, tea.Batch(work, modpackProgressTick(req))
}

func (m Model) modpackRecover() (tea.Model, tea.Cmd) {
	ctx, req := m.modpackWork("recover")
	store, target := m.mp.store, m.instance
	return m, func() tea.Msg {
		if err := ctx.Err(); err != nil {
			return modpackMsg{request: req, kind: "recover", err: err}
		}
		return modpackMsg{request: req, kind: "recover", err: modpacks.Recover(store.Root, target)}
	}
}

func (m Model) modpackExportPaths() (tea.Model, tea.Cmd) {
	ctx, req := m.modpackWork("export choices")
	root, in := m.dataRoot, m.instance
	return m, func() tea.Msg {
		gameDir, err := instances.GameDir(root, in)
		if err != nil {
			return modpackMsg{request: req, kind: "export choices", err: err}
		}
		var files []string
		for _, group := range []string{"config", "resourcepacks", "shaderpacks", "options.txt"} {
			base := filepath.Join(gameDir, group)
			if _, e := os.Lstat(base); os.IsNotExist(e) {
				continue
			} else if e != nil {
				return modpackMsg{request: req, kind: "export choices", err: e}
			}
			err = filepath.WalkDir(base, func(path string, d os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if err := ctx.Err(); err != nil {
					return err
				}
				if d.Type()&os.ModeSymlink != 0 {
					return fmt.Errorf("linked export path: %s", path)
				}
				if d.IsDir() {
					return nil
				}
				if !d.Type().IsRegular() {
					return fmt.Errorf("special export file: %s", path)
				}
				rel, err := filepath.Rel(gameDir, path)
				if err == nil {
					files = append(files, filepath.ToSlash(rel))
				}
				return err
			})
			if err != nil {
				return modpackMsg{request: req, kind: "export choices", err: err}
			}
		}
		slices.Sort(files)
		return modpackMsg{request: req, kind: "export choices", overrides: files}
	}
}

func (m Model) modpackExport(output string) (tea.Model, tea.Cmd) {
	ctx, req := m.modpackWork("export")
	store, target := m.mp.store, m.instance
	var selected []string
	for _, path := range m.mp.overrides {
		if m.mp.selected[path] {
			selected = append(selected, path)
		}
	}
	return m, func() tea.Msg {
		err := store.Export(ctx, target, output, selected)
		return modpackMsg{request: req, kind: "export", err: err}
	}
}

func (m Model) updateModpacks(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	if m.state != modpackScreen {
		switch v := msg.(type) {
		case modpackMsg:
			if v.cleanup != nil {
				v.cleanup()
			}
			return m, nil, true
		case modpackProgressTickMsg:
			return m, nil, true
		}
		return m, nil, false
	}
	switch v := msg.(type) {
	case modpackMsg:
		if v.request != m.mp.request {
			if v.cleanup != nil {
				v.cleanup()
			}
			return m, nil, true
		}
		if m.mp.cancel != nil {
			m.mp.cancel()
			m.mp.cancel = nil
		}
		if m.mp.cancelRequested && (errors.Is(v.err, context.Canceled) ||
			v.err == nil && (v.kind == "inspect" || v.kind == "search" || v.kind == "releases" || v.kind == "plan" || v.kind == "export choices")) {
			if v.cleanup != nil {
				v.cleanup()
			}
			m.mp.step, m.mp.busy = m.mp.returnStep, ""
			m.mp.progress = nil
			m.mp.cancelRequested = false
			if v.kind == "plan" && m.mp.mode == "into" {
				m.mp.mode = "import"
			}
			return m, nil, true
		}
		if v.err != nil {
			if v.cleanup != nil {
				v.cleanup()
			}
			m.mp.err, m.mp.step, m.mp.busy = v.err, "result", ""
			m.mp.progress = nil
			return m, nil, true
		}
		switch v.kind {
		case "inspect":
			if m.mp.cleanup != nil {
				m.mp.cleanup()
			}
			m.mp.archive, m.mp.source, m.mp.cleanup = v.archive, v.source, v.cleanup
			m.mp.name = v.archive.Index.Name
			m.mp.optional = map[string]bool{}
			m.mp.cursor = 0
			m.mp.step = "review"
		case "search":
			m.mp.results, m.mp.cursor, m.mp.step = v.results, 0, "results"
		case "releases":
			m.mp.versions = v.versions
			m.mp.minecraftVersions = nil
			m.mp.selectedMinecraft = ""
			m.mp.selectedVersions = nil
			m.mp.showAllVersions = false
			seen := map[string]bool{}
			for _, version := range v.versions {
				for _, game := range version.GameVersions {
					if !seen[game] {
						seen[game] = true
						m.mp.minecraftVersions = append(m.mp.minecraftVersions, game)
					}
				}
			}
			m.mp.cursor = 0
			m.mp.step = "minecraft"
		case "plan":
			m.mp.plan = v.plan
			m.mp.conflicts = nil
			m.mp.decisions = map[string]bool{}
			for _, change := range v.plan.Changes {
				if change.Conflict {
					m.mp.conflicts = append(m.mp.conflicts, change.Path)
					m.mp.decisions[change.Path] = false
				}
			}
			m.mp.cursor, m.mp.step = 0, "conflicts"
		case "install":
			m.mp.step = "result"
			m.mp.message = "Modpack instalado o actualizado con éxito."
			if m.mp.mode == "into" {
				m.mp.message = "Modpack instalado en la instancia actual con éxito."
			}
			if v.instance.ID != "" {
				if err := m.setInstance(v.instance); err != nil {
					m.mp.err = err
				} else {
					m.instanceList, m.instanceErr = instances.List(m.dataRoot)
				}
			}
			if m.mp.cleanup != nil {
				m.mp.cleanup()
				m.mp.cleanup = nil
			}
		case "export choices":
			m.mp.overrides, m.mp.selected, m.mp.cursor, m.mp.step = v.overrides, map[string]bool{}, 0, "export choices"
		case "export":
			m.mp.message, m.mp.step = "Modpack exportado con éxito.", "result"
		case "recover":
			m.mp.message, m.mp.step = "No queda una actualización interrumpida pendiente.", "result"
		}
		m.mp.busy = ""
		m.mp.progress = nil
		return m, nil, true
	case modpackProgressTickMsg:
		if v.request == m.mp.request && m.mp.step == "working" && m.mp.busy == "install" {
			return m, modpackProgressTick(v.request), true
		}
		return m, nil, true
	case tea.KeyMsg:
		next, cmd := m.updateModpackKey(v)
		return next, cmd, true
	}
	return m, nil, false
}

func (m Model) executeHomeOption(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "l":
		m.mp.mode = "import"
		m.modpackInput("local", "Archivo .mrpack local: ", "")
		return m, nil
	case "h":
		m.mp.mode = "import"
		m.modpackInput("url", "URL HTTPS (.mrpack): ", "https://")
		return m, nil
	case "s", "b":
		m.mp.mode = "import"
		m.mp.query = ""
		m.mp.cursor = 0
		return m.modpackSearch()
	case "u":
		if !m.modpackCanManageCurrent() {
			m.mp.err = fmt.Errorf("esta instancia no tiene un modpack actualizable")
			m.mp.step = "result"
			return m, nil
		}
		m.mp.mode = "update"
		m.modpackInput("local", "Nueva versión (.mrpack local): ", "")
		return m, nil
	case "e":
		if !m.modpackCanManageCurrent() {
			m.mp.err = fmt.Errorf("esta instancia no tiene un modpack exportable")
			m.mp.step = "result"
			return m, nil
		}
		return m.modpackExportPaths()
	case "r":
		if m.instance.Kind != "managed" && (m.instance.ID != instances.LegacyID || m.instance.Kind != "legacy") {
			m.mp.err = fmt.Errorf("esta instancia no tiene un modpack recuperable")
			m.mp.step = "result"
			return m, nil
		}
		return m.modpackRecover()
	}
	return m, nil
}

func (m Model) updateModpackKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := key.String()
	switch m.mp.step {
	case "home":
		switch k {
		case "esc", "q":
			m.stopModpack()
			m.state = modrinthHubScreen
			return m, nil
		case "up", "k":
			m.mp.homeCursor = max(0, m.mp.homeCursor-1)
			return m, nil
		case "down", "j":
			m.mp.homeCursor = min(len(modpackHomeOptions)-1, m.mp.homeCursor+1)
			return m, nil
		case "enter", " ":
			selected := modpackHomeOptions[m.mp.homeCursor].key
			return m.executeHomeOption(selected)
		case "l", "h", "s", "b", "u", "e", "r":
			return m.executeHomeOption(k)
		}

	case "input":
		switch k {
		case "esc":
			m.mp.step = "home"
			return m, nil
		case "enter":
			value := strings.TrimSpace(m.mp.input.Value())
			if value == "" {
				m.mp.err = fmt.Errorf("ingresá un valor válido")
				return m, nil
			}
			switch m.mp.action {
			case "local", "url":
				return m.modpackLoad(value, m.mp.action == "url")
			case "name":
				m.mp.name = value
				m.mp.step = "review"
				return m, nil
			case "export":
				return m.modpackExport(value)
			}
		}
		var cmd tea.Cmd
		m.mp.input, cmd = m.mp.input.Update(key)
		return m, cmd

	case "results":
		switch k {
		case "esc":
			m.mp.step = "home"
			return m, nil
		case "up", "k":
			m.mp.cursor = max(0, m.mp.cursor-1)
			return m, nil
		case "down", "j":
			m.mp.cursor = min(max(0, len(m.mp.results.Hits)-1), m.mp.cursor+1)
			return m, nil
		case "enter":
			if len(m.mp.results.Hits) > 0 {
				m.mp.projectID = m.mp.results.Hits[m.mp.cursor].ProjectID
				m.mp.projectTitle = m.mp.results.Hits[m.mp.cursor].Title
				m.mp.showAllVersions = false
				m.mp.cursor = 0
				return m.modpackReleases()
			}
		case "backspace":
			if len(m.mp.query) > 0 {
				_, size := utf8.DecodeLastRuneInString(m.mp.query)
				m.mp.query = m.mp.query[:len(m.mp.query)-size]
				return m.modpackSearch()
			}
		default:
			if utf8.RuneCountInString(k) == 1 && k != "/" {
				m.mp.query += k
				return m.modpackSearch()
			}
		}

	case "minecraft":
		switch k {
		case "esc":
			m.mp.step = "results"
			m.mp.cursor = 0
		case "up", "k":
			m.mp.cursor = max(0, m.mp.cursor-1)
		case "down", "j":
			m.mp.cursor = min(max(0, len(m.mp.minecraftVersions)-1), m.mp.cursor+1)
		case "enter":
			if len(m.mp.minecraftVersions) > 0 {
				m.mp.selectedMinecraft = m.mp.minecraftVersions[m.mp.cursor]
				m.mp.selectedVersions = m.mp.selectedVersions[:0]
				for _, version := range m.mp.versions {
					for _, game := range version.GameVersions {
						if game == m.mp.selectedMinecraft {
							m.mp.selectedVersions = append(m.mp.selectedVersions, version)
							break
						}
					}
				}
				m.mp.cursor = 0
				m.mp.step = "releases"
			}
		}
		return m, nil

	case "releases":
		count := len(m.modpackVisibleVersions())
		limit := min(3, count)
		totalItems := limit
		if !m.mp.showAllVersions && count > 3 {
			totalItems++
		} else if m.mp.showAllVersions {
			totalItems = count
		}
		switch k {
		case "esc":
			if m.mp.showAllVersions {
				m.mp.showAllVersions = false
				m.mp.cursor = 0
				return m, nil
			}
			m.mp.step = "minecraft"
			m.mp.cursor = 0
			return m, nil
		case "v", "tab":
			m.mp.showAllVersions = !m.mp.showAllVersions
			m.mp.cursor = 0
			return m, nil
		case "up", "k":
			m.mp.cursor = max(0, m.mp.cursor-1)
			return m, nil
		case "down", "j":
			m.mp.cursor = min(max(0, totalItems-1), m.mp.cursor+1)
			return m, nil
		case "enter":
			if !m.mp.showAllVersions && count > 3 && m.mp.cursor == limit {
				m.mp.showAllVersions = true
				m.mp.cursor = 0
				return m, nil
			}
			if count > 0 && m.mp.cursor < count {
				return m.modpackStageVersion()
			}
		}

	case "review":
		optional := m.modpackOptionals()
		switch k {
		case "esc":
			m.stopModpack()
			m.mp.step = "home"
			return m, nil
		case "up", "k":
			m.mp.cursor = max(0, m.mp.cursor-1)
			return m, nil
		case "down", "j":
			m.mp.cursor = min(max(0, len(optional)-1), m.mp.cursor+1)
			return m, nil
		case " ":
			if len(optional) > 0 {
				p := optional[m.mp.cursor]
				m.mp.optional[p] = !m.mp.optional[p]
			}
		case "n":
			if m.mp.mode == "import" {
				m.modpackInput("name", "Nombre de la nueva instancia: ", m.mp.name)
				return m, nil
			}
		case "i":
			if m.mp.mode == "import" {
				if reason := m.modpackCurrentTargetReason(); reason == "" {
					m.mp.mode = "into"
					return m.modpackPlan()
				}
			}
		case "enter":
			if m.mp.mode != "update" {
				m.mp.mode = "import"
			}
			if m.mp.mode == "update" {
				return m.modpackPlan()
			}
			return m.modpackRun()
		}

	case "conflicts":
		switch k {
		case "esc":
			m.mp.step = "review"
			if m.mp.mode == "into" {
				m.mp.mode = "import"
			}
			return m, nil
		case "up", "k":
			m.mp.cursor = max(0, m.mp.cursor-1)
			return m, nil
		case "down", "j":
			m.mp.cursor = min(max(0, len(m.mp.plan.Changes)-1), m.mp.cursor+1)
			return m, nil
		case " ":
			if len(m.mp.plan.Changes) > 0 {
				change := m.mp.plan.Changes[m.mp.cursor]
				if change.Conflict {
					m.mp.decisions[change.Path] = !m.mp.decisions[change.Path]
				}
			}
			return m, nil
		case "enter":
			return m.modpackRun()
		}

	case "export choices":
		switch k {
		case "esc":
			m.mp.step = "home"
			return m, nil
		case "up", "k":
			m.mp.cursor = max(0, m.mp.cursor-1)
			return m, nil
		case "down", "j":
			m.mp.cursor = min(max(0, len(m.mp.overrides)-1), m.mp.cursor+1)
			return m, nil
		case " ":
			if len(m.mp.overrides) > 0 {
				p := m.mp.overrides[m.mp.cursor]
				m.mp.selected[p] = !m.mp.selected[p]
			}
			return m, nil
		case "enter":
			m.modpackInput("export", "Guardar archivo como (.mrpack): ", m.instance.Name+".mrpack")
			return m, nil
		}

	case "working":
		if k == "esc" && m.mp.cancel != nil && !m.mp.cancelRequested {
			m.mp.cancelRequested = true
			m.mp.cancel()
		}

	case "result":
		if k == "enter" || k == "esc" {
			m.stopModpack()
			m.mp.step = "home"
			m.mp.err = nil
			m.mp.message = ""
			return m, nil
		}
	}
	return m, nil
}

func (m Model) modpackOptionals() []string {
	var paths []string
	for _, file := range m.mp.archive.Index.Files {
		if file.Env != nil && file.Env.Client == "optional" {
			paths = append(paths, file.Path)
		}
	}
	return paths
}

func (m Model) modpackRuntime() string {
	dep := m.mp.archive.Index.Dependencies
	if loader := dep["fabric-loader"]; loader != "" {
		return "Minecraft " + modText(dep["minecraft"]) + " / Fabric " + modText(loader)
	}
	return "Minecraft " + modText(dep["minecraft"]) + " / Vanilla"
}

func (m Model) modpackCanManageCurrent() bool {
	if m.instance.Kind == "managed" {
		return true
	}
	if m.instance.ID != instances.LegacyID || m.instance.Kind != "legacy" {
		return false
	}
	_, err := os.Lstat(filepath.Join(m.dataRoot, "legacy-pack", "pack.json"))
	return err == nil
}

func (m Model) modpackCurrentTargetReason() string {
	in := m.instance
	if in.ID == "" || in.Kind != "managed" && (in.ID != instances.LegacyID || in.Kind != "legacy") {
		return "no hay una instancia actual seleccionada"
	}
	dep := m.mp.archive.Index.Dependencies
	mc, loader, loaderVersion := dep["minecraft"], "Vanilla", ""
	if dep["fabric-loader"] != "" {
		loader, loaderVersion = "Fabric", dep["fabric-loader"]
	}
	if in.MinecraftVersion == "" || in.Loader == "" || in.Loader == "Fabric" && in.LoaderVersion == "" {
		return "la instancia actual no tiene versión exacta de Minecraft y cargador registrada"
	}
	if in.MinecraftVersion != mc || in.Loader != loader || in.LoaderVersion != loaderVersion {
		return fmt.Sprintf("runtime distinto: instancia Minecraft %s / %s %s; pack %s",
			modText(in.MinecraftVersion), modText(in.Loader), modText(in.LoaderVersion), m.modpackRuntime())
	}
	gameDir, err := instances.GameDir(m.dataRoot, in)
	if err != nil {
		return "no se puede acceder a la carpeta de la instancia: " + displayError(err)
	}
	if in.Kind == "legacy" {
		info, err := os.Lstat(gameDir)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "la carpeta de la instalación existente no está disponible"
		}
	}
	manifestPath := filepath.Join(m.dataRoot, "legacy-pack", "pack.json")
	if in.Kind == "managed" {
		manifestPath = filepath.Join(filepath.Dir(gameDir), "pack.json")
	}
	if _, err := os.Lstat(manifestPath); err == nil {
		return "ya tiene un modpack; usá [u] Actualizar"
	} else if !os.IsNotExist(err) {
		return "no se puede comprobar el modpack actual: " + displayError(err)
	}
	return ""
}

func (m Model) currentPackManifest() (modpacks.Manifest, bool) {
	path := filepath.Join(m.dataRoot, "legacy-pack", "pack.json")
	if m.instance.Kind == "managed" {
		gameDir, err := instances.GameDir(m.dataRoot, m.instance)
		if err != nil {
			return modpacks.Manifest{}, false
		}
		path = filepath.Join(filepath.Dir(gameDir), "pack.json")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return modpacks.Manifest{}, false
	}
	var mf modpacks.Manifest
	if err := json.Unmarshal(data, &mf); err != nil {
		return modpacks.Manifest{}, false
	}
	return mf, true
}
func sanitizeSingleLine(s string) string {
	s = modText(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return strings.Join(strings.Fields(s), " ")
}

func (m Model) modpackVisibleVersions() []modrinth.Version {
	if m.mp.selectedMinecraft == "" {
		return m.mp.versions
	}
	return m.mp.selectedVersions
}

func (m Model) renderModpacks() string {
	var b strings.Builder
	if m.mp.err != nil && m.mp.step != "result" {
		b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Render("⚠ "+modText(displayError(m.mp.err))) + "\n\n")
	}

	switch m.mp.step {
	case "home":
		title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render("MODPACKS")
		div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", max(46, m.centerWidth()-4)))
		b.WriteString(title + "\n" + div + "\n\n")

		gameDir, _ := instances.GameDir(m.dataRoot, m.instance)
		pathWidth := max(20, m.centerWidth()-16)
		b.WriteString(padCol(i18n.T("common.instance_label"), 12) + "  " +
			lipgloss.NewStyle().Foreground(colorAccent2).Bold(true).Render(modText(m.instance.Name)) + "\n")
		b.WriteString(padCol(i18n.T("common.version_label"), 12) + "  " +
			lipgloss.NewStyle().Foreground(colorGray).Render(modText(m.instance.MinecraftVersion+" / "+m.instance.Loader+" "+m.instance.LoaderVersion)) + "\n")
		b.WriteString(padCol(i18n.T("common.path_label"), 12) + "  " +
			lipgloss.NewStyle().Foreground(colorDimGray).Render(formatDisplayPath(modText(gameDir), pathWidth)) + "\n\n")
		rowWidth := max(44, m.centerWidth()-6)
		for i, opt := range modpackHomeOptions {
			if i == m.mp.homeCursor {
				b.WriteString(cursorStyle.Render("► ") + lipgloss.NewStyle().Foreground(colorWhite).Bold(true).MaxWidth(rowWidth).Render(opt.label) + "\n")
			} else {
				b.WriteString("  " + lipgloss.NewStyle().Foreground(colorGray).MaxWidth(rowWidth).Render(opt.label) + "\n")
			}
		}
		b.WriteString("\n")
		if mf, ok := m.currentPackManifest(); ok {
			b.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render(fmt.Sprintf("  %d files · Pack: %s (%s)", len(mf.Files), modText(mf.Name), modText(mf.IndexVersionID))) + "\n")
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render("  Sin modpack gestionado en esta instancia") + "\n")
		}
		b.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render("  [Space/Enter] Seleccionar   [l] Local   [h] URL   [s] Buscar\n  [u] Actualizar   [e] Exportar   [r] Recuperar   [Esc] Volver") + "\n")
	case "input":
		b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("MODPACKS") + "\n\n")
		b.WriteString(m.mp.input.View() + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render("[Enter] Confirmar   [Esc] Volver") + "\n")
	case "results":
		headerWidth := max(20, m.centerWidth()-6)
		headerLines := wrapResetLines([]string{"Modrinth Modpacks"}, headerWidth)
		filterLines := wrapResetLines([]string{"[Minecraft: elegir después de seleccionar modpack]  [Sort: Relevance]"}, headerWidth)
		b.WriteString(strings.Join(headerLines, "\n") + "\n" + strings.Join(filterLines, "\n") + "\n\n")

		b.WriteString(i18n.T("modrinth.search_label", modText(m.mp.query)) + "\n")
		textWidth := m.catalogWidth()
		useTable := m.width >= 75 && textWidth >= 55
		descWidth := max(10, textWidth-54)
		if useTable && len(m.mp.results.Hits) > 0 {
			header := "  " + padCol("Name", 22) + "  " + padCol("Author", 14) + "  " + padCol("Downloads", 10) + "  " + padCol("Short description", descWidth)
			b.WriteString(lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(header) + "\n")
			divWidth := max(40, textWidth-2)
			b.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render("  "+strings.Repeat("─", divWidth)) + "\n")
		}

		lines := make([]string, 0, len(m.mp.results.Hits))
		for i, hit := range m.mp.results.Hits {
			prefix := "  "
			if i == m.mp.cursor {
				prefix = "► "
			}
			cleanTitle := sanitizeSingleLine(hit.Title)
			cleanAuthor := sanitizeSingleLine(hit.Author)
			cleanDesc := sanitizeSingleLine(hit.Description)
			if useTable {
				desc := padCol(cleanDesc, descWidth)
				lines = append(lines, prefix+padCol(cleanTitle, 22)+"  "+padCol(cleanAuthor, 14)+"  "+padCol(formatDownloads(hit.Downloads), 10)+"  "+desc)
			} else {
				lines = append(lines, prefix+cleanTitle+" · "+cleanAuthor+" · "+formatDownloads(hit.Downloads)+" — "+cleanDesc)
			}
		}
		reserve := 7
		if useTable && len(m.mp.results.Hits) > 0 {
			reserve = 9
		}
		b.WriteString(m.catalogPage(lines, m.mp.cursor, reserve))
		b.WriteString(fmt.Sprintf("Showing %d of %d modpacks\n", len(m.mp.results.Hits), m.mp.results.Total))
		b.WriteString("[↑/↓] Navigate   [Enter] View details   [Esc] Back\n")

	case "minecraft":
		b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(modText(m.mp.projectTitle)) + "\n")
		b.WriteString("Elegí la versión de Minecraft del modpack (independiente de la instancia actual):\n\n")
		if len(m.mp.minecraftVersions) == 0 {
			b.WriteString("  No hay versiones de Minecraft publicadas para este modpack.\n")
		}
		lines := make([]string, 0, len(m.mp.minecraftVersions))
		for i, game := range m.mp.minecraftVersions {
			prefix := "  "
			if i == m.mp.cursor {
				prefix = "► "
			}
			lines = append(lines, prefix+modText(game))
		}
		b.WriteString(m.catalogPage(lines, m.mp.cursor, 6))
		b.WriteString("[↑/↓] Navegar   [Enter] Elegir versión   [Esc] Volver\n")

	case "releases":
		title := modText(m.mp.projectTitle)
		if title == "" {
			title = modText(m.mp.projectID)
		}
		versions := m.modpackVisibleVersions()
		count := len(versions)
		if count == 0 {
			b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(title) + "\n\n")
			b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("  No hay releases para esta versión de Minecraft.") + "\n\n")
			b.WriteString("[Esc] Elegir otra versión de Minecraft\n")
		} else if !m.mp.showAllVersions {
			b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(title) + "\n")
			b.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render("Minecraft "+modText(m.mp.selectedMinecraft)+" · Elegí el release del modpack:") + "\n\n")
			limit := min(3, count)
			for i := range limit {
				v := versions[i]
				prefix := "  "
				if i == m.mp.cursor {
					prefix = cursorStyle.Render("► ")
				}
				verStr := modText(v.Name)
				if v.Number != "" && v.Number != v.Name {
					verStr += " (" + modText(v.Number) + ")"
				}
				channelStr := lipgloss.NewStyle().Foreground(colorDimGray).Render(modText(v.Channel) + " · " + modText(strings.Join(v.Loaders, ", ")))
				b.WriteString(prefix + padCol(verStr, 34) + "  " + channelStr + "\n")
			}
			if count > 3 {
				prefix := "  "
				if m.mp.cursor == limit {
					prefix = cursorStyle.Render("► ")
				}
				expandText := fmt.Sprintf("[▼ Ver todo el historial / otras versiones (%d disponibles)]", count)
				b.WriteString("\n" + prefix + lipgloss.NewStyle().Foreground(colorAccent).Render(expandText) + "\n")
			}
			b.WriteString("\n" + lipgloss.NewStyle().Foreground(colorDimGray).Render("[↑/↓] Navegar   [Enter] Inspeccionar / Desplegar   [v / Tab] Expandir   [Esc] Volver") + "\n")
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(title) + "  " + lipgloss.NewStyle().Foreground(colorDimGray).Render(fmt.Sprintf("(Minecraft %s: %d releases)", modText(m.mp.selectedMinecraft), count)) + "\n")
			b.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render("Historial completo · Presioná [v] o [Tab] para colapsar a los últimos releases.") + "\n\n")
			lines := make([]string, 0, count)
			for i, v := range versions {
				prefix := "  "
				if i == m.mp.cursor {
					prefix = "► "
				}
				verStr := modText(v.Name)
				if v.Number != "" && v.Number != v.Name {
					verStr += " (" + modText(v.Number) + ")"
				}
				lines = append(lines, prefix+padCol(verStr, 34)+"  "+padCol(modText(v.Channel), 10)+"  "+modText(strings.Join(v.Loaders, ", ")))
			}
			b.WriteString(m.catalogPage(lines, m.mp.cursor, 8))
			b.WriteString(fmt.Sprintf("Showing %d of %d versions\n", min(count, 8), count))
			b.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render("[↑/↓] Navegar   [Enter] Inspeccionar versión   [v / Tab] Colapsar   [Esc] Volver") + "\n")
		}
	case "review":
		idx := m.mp.archive.Index
		b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(modText(idx.Name)) + "  " + lipgloss.NewStyle().Foreground(colorGray).Render("("+modText(idx.VersionID)+")") + "\n\n")
		if idx.Summary != "" {
			b.WriteString(lipgloss.NewStyle().Foreground(colorWhite).Render(modText(idx.Summary)) + "\n\n")
		}
		b.WriteString(padCol("Runtime pack:", 14) + "  " + lipgloss.NewStyle().Foreground(colorAccent2).Render(m.modpackRuntime()) + "\n")
		b.WriteString(padCol("Archivos:", 14) + "  " + fmt.Sprintf("%d declarados", len(idx.Files)) + "\n")
		b.WriteString(padCol("Tamaño:", 14) + "  " + fmt.Sprintf("%.1f MiB", float64(m.modpackSize())/(1<<20)) + "\n\n")

		if m.mp.mode == "import" || m.mp.mode == "into" {
			b.WriteString(padCol("Nueva:", 14) + "  " + lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render(modText(m.mp.name)) + "  " + lipgloss.NewStyle().Foreground(colorDimGray).Render("[n] Cambiar nombre") + "\n")
			b.WriteString(padCol("Actual:", 14) + "  " + modText(m.instance.Name) + "\n")
			if gameDir, err := instances.GameDir(m.dataRoot, m.instance); err == nil {
				b.WriteString(padCol("Ruta actual:", 14) + "  " + formatDisplayPath(modText(gameDir), max(20, m.centerWidth()-16)) + "\n")
			}
			if reason := m.modpackCurrentTargetReason(); reason != "" {
				b.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render("  [i] No disponible: "+modText(reason)) + "\n\n")
			} else {
				b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("  [i] Comparar rutas e instalar en la instancia actual") + "\n\n")
			}
		}

		optional := m.modpackOptionals()
		if len(optional) > 0 {
			b.WriteString(lipgloss.NewStyle().Foreground(colorBlue).Render("Archivos cliente opcionales:") + "\n")
			for i, p := range optional {
				prefix := "  "
				if i == m.mp.cursor {
					prefix = cursorStyle.Render("► ")
				}
				mark := " "
				if m.mp.optional[p] {
					mark = "x"
				}
				b.WriteString(prefix + "[" + mark + "] " + modText(p) + "\n")
			}
			b.WriteString("\n")
		}
		prompt := "[Enter] Instalar en instancia nueva (predeterminado)   [Esc] Cancelar"
		if m.mp.mode == "update" {
			b.WriteString(padCol("Destino:", 14) + "  " + modText(m.instance.Name) + "\n")
			prompt = "[Enter] Comparar cambios y actualizar   [Esc] Cancelar"
		}
		b.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render(prompt) + "\n")

	case "conflicts":
		title, verb := "ACTUALIZACIÓN DE MODPACK — RESOLUCIÓN DE CAMBIOS", "Aplicar actualización"
		if m.mp.mode == "into" {
			title, verb = "INSTALAR EN INSTANCIA ACTUAL — REVISIÓN DE RUTAS", "Instalar en instancia actual"
		}
		b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(title) + "\n")
		b.WriteString("Pack: " + m.modpackRuntime() + "  ·  Destino: " + modText(m.instance.Name) + "\n")
		if gameDir, err := instances.GameDir(m.dataRoot, m.instance); err == nil {
			b.WriteString("Ruta: " + formatDisplayPath(modText(gameDir), max(20, m.centerWidth()-12)) + "\n")
		}
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("Cambios propuestos: %d · Conflictos detectados: %d\n", len(m.mp.plan.Changes), len(m.mp.conflicts)))
		b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("Cada ruta local se conserva por defecto. [Espacio] permite sobrescribir solo la ruta seleccionada.") + "\n\n")

		lines := make([]string, 0, len(m.mp.plan.Changes))
		for i, change := range m.mp.plan.Changes {
			prefix := "  "
			if i == m.mp.cursor {
				prefix = "► "
			}
			decision := "Automático"
			if change.Conflict {
				decision = "Conservar local"
				if m.mp.decisions[change.Path] {
					decision = "Sobrescribir"
				}
			}
			lines = append(lines, fmt.Sprintf("%s[%s] %s / local: %s — %s", prefix, decision, change.Upstream, change.LocalChange, modText(change.Path)))
		}
		b.WriteString(m.catalogPage(lines, m.mp.cursor, 7))
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(colorDimGray).Render("[↑/↓] Revisar rutas   [Space] Alternar conflicto   [Enter] "+verb+"   [Esc] Volver") + "\n")

	case "export choices":
		b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("EXPORTAR MODPACK (.mrpack)") + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("Seleccioná los archivos locales a incluir como overrides (saves y secretos excluidos):") + "\n\n")
		for i, p := range m.mp.overrides {
			prefix := "  "
			if i == m.mp.cursor {
				prefix = cursorStyle.Render("► ")
			}
			mark := " "
			if m.mp.selected[p] {
				mark = "x"
			}
			b.WriteString(prefix + "[" + mark + "] " + modText(p) + "\n")
		}
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(colorDimGray).Render("[Space] Alternar   [Enter] Confirmar selección   [Esc] Volver") + "\n")

	case "working":
		label := "Procesando modpack"
		switch m.mp.busy {
		case "inspect":
			label = "Descargando e inspeccionando modpack"
		case "search":
			label = "Buscando modpacks"
		case "releases":
			label = "Cargando versiones"
		case "plan":
			label = "Comparando cambios del modpack"
		case "install":
			label = "Instalando modpack"
		case "recover":
			label = "Recuperando actualización"
		case "export choices":
			label = "Buscando archivos para exportar"
		case "export":
			label = "Exportando modpack"
		}
		b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(label) + "\n\n")
		if m.mp.busy == "install" && m.mp.progress != nil {
			done, total := m.mp.progress.done.Load(), m.mp.progress.total.Load()
			if total > 0 {
				b.WriteString(fmt.Sprintf("Archivos descargados: %d%% (%.1f / %.1f MiB)\n\n", done*100/total, float64(done)/(1<<20), float64(total)/(1<<20)))
				if done == total {
					b.WriteString("Preparando archivos y runtime...\n\n")
				}
			} else {
				b.WriteString("Preparando archivos y runtime...\n\n")
			}
		}
		if m.mp.cancelRequested {
			b.WriteString("Cancelando; esperando a que termine la operación...\n")
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render("[Esc] Cancelar") + "\n")
		}

	case "result":
		if m.mp.err == nil {
			b.WriteString(lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render("✔ "+m.mp.message) + "\n\n")
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render("⚠ Error: "+modText(displayError(m.mp.err))) + "\n\n")
		}
		b.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render("[Enter / Esc] Volver al menú de modpacks") + "\n")
	}

	return b.String()
}

func (m Model) modpackSize() int64 {
	var total int64
	for _, file := range m.mp.archive.Index.Files {
		if file.Env == nil || file.Env.Client != "unsupported" && (file.Env.Client != "optional" || m.mp.optional[file.Path]) {
			total += file.FileSize
		}
	}
	return total
}
