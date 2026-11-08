package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"mcTUI/internal/i18n"
	"mcTUI/internal/instances"
	"mcTUI/internal/modrinth"
	"mcTUI/internal/packs"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

type packListMsg struct {
	request uint64
	items   []packs.Pack
	err     error
}
type packSearchMsg struct {
	request uint64
	offset  int
	results modrinth.SearchResponse
	err     error
}
type packDetailMsg struct {
	request uint64
	project modrinth.Project
	version modrinth.Version
	err     error
}
type packDoneMsg struct {
	request uint64
	items   []packs.Pack
	err     error
}
type packSearchTick uint64

func (m *Model) stopPackRead() {
	if m.packCancel != nil {
		m.packCancel()
		m.packCancel = nil
	}
	m.packContext = nil
	m.packRequest++
}

func (m *Model) startPackRead() {
	m.stopPackRead()
	m.packContext, m.packCancel = context.WithCancel(context.Background())
}

func (m Model) openPacks(kind packs.Kind) (tea.Model, tea.Cmd) {
	m.stopPackRead()
	m.packKind = kind
	m.packCursor = 0
	m.packItems = nil
	m.packErr = nil
	m.packBusy = true
	m.state = packWorkingScreen
	m.packReturn = packListScreen
	gameDir, err := instances.GameDir(m.dataRoot, m.instance)
	if err != nil {
		m.packBusy = false
		m.packErr = err
		m.state = packListScreen
		return m, nil
	}
	m.packStore = packs.Store{GameDir: gameDir}
	return m, m.refreshPacks()
}

func (m Model) refreshPacks() tea.Cmd {
	store, kind, request := m.packStore, m.packKind, m.packRequest
	return func() tea.Msg {
		items, err := store.List(kind)
		return packListMsg{request: request, items: items, err: err}
	}
}

func (m Model) packOperation(action, source, filename string) (tea.Model, tea.Cmd) {
	m.stopPackRead()
	m.packBusy = true
	m.state = packWorkingScreen
	m.packReturn = packListScreen
	store, kind, request := m.packStore, m.packKind, m.packRequest
	return m, func() tea.Msg {
		var err error
		switch action {
		case "add":
			err = store.Add(kind, source)
		case "remove":
			err = store.Remove(kind, filename)
		}
		items, listErr := store.List(kind)
		if err == nil {
			err = listErr
		}
		return packDoneMsg{request: request, items: items, err: err}
	}
}

func (m Model) packCategory() string {
	if m.packKind == packs.Resource {
		return "minecraft"
	}
	if m.packEngine == "optifine" {
		return "optifine"
	}
	return "iris"
}

func (m Model) packMinecraftVersion() string {
	if m.instance.Kind == "legacy" {
		return m.versionSelect
	}
	return m.instance.MinecraftVersion
}

func (m Model) packSearch() tea.Cmd {
	ctx, req := m.packContext, m.packRequest
	kind, category, version := string(m.packKind), m.packCategory(), m.packMinecraftVersion()
	query, offset, sort := m.packQuery, m.packOffset, catalogSorts[m.packSortIndex]
	return func() tea.Msg {
		results, err := catalogAPI.SearchProjects(ctx, query, kind, category, version, offset, sort)
		return packSearchMsg{request: req, offset: offset, results: results, err: err}
	}
}

func (m Model) packDetail() tea.Cmd {
	ctx, req := m.packContext, m.packRequest
	id := m.packResults.Hits[m.packSearchCursor].ProjectID
	kind, category, version := string(m.packKind), m.packCategory(), m.packMinecraftVersion()
	return func() tea.Msg {
		project, err := catalogAPI.Project(ctx, id)
		if err != nil {
			return packDetailMsg{request: req, err: err}
		}
		if project.ProjectType != kind {
			return packDetailMsg{request: req, err: fmt.Errorf("unexpected project type: %s", project.ProjectType)}
		}
		selected, err := catalogAPI.CompatibleVersion(ctx, id, category, version, "release")
		return packDetailMsg{request: req, project: project, version: selected, err: err}
	}
}

func (m Model) packInstall() tea.Cmd {
	ctx, req := m.packContext, m.packRequest
	store, kind, version := m.packStore, m.packKind, m.packVersion
	return func() tea.Msg {
		file, ok := version.PrimaryFile()
		if !ok {
			return packDoneMsg{request: req, err: fmt.Errorf("release has no primary file")}
		}
		staged, err := catalogAPI.StagePack(ctx, file, os.TempDir(), nil)
		if err == nil {
			defer os.Remove(staged)
			if err = ctx.Err(); err == nil {
				err = store.Install(kind, staged, file.Filename)
			}
		}
		items, listErr := store.List(kind)
		if err == nil {
			err = listErr
		}
		return packDoneMsg{request: req, items: items, err: err}
	}
}

func (m Model) updatePacks(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch v := msg.(type) {
	case packListMsg:
		if v.request == m.packRequest && m.state == packWorkingScreen {
			m.packItems, m.packErr = v.items, v.err
			m.packCursor = min(m.packCursor, max(0, len(m.packItems)-1))
			m.packBusy = false
			m.state = packListScreen
		}
		return m, nil, true
	case packDoneMsg:
		if v.request == m.packRequest && m.state == packWorkingScreen {
			m.stopPackRead()
			m.packItems, m.packErr = v.items, v.err
			m.packCursor = min(m.packCursor, max(0, len(m.packItems)-1))
			m.packBusy = false
			m.state = packListScreen
		}
		return m, nil, true
	case packSearchMsg:
		if v.request == m.packRequest && m.state == packWorkingScreen && m.packReturn == packSearchScreen {
			m.packBusy = false
			m.packErr = v.err
			if v.err == nil {
				if v.offset == 0 {
					m.packResults = v.results
					m.packSearchCursor = 0
				} else {
					m.packResults.Hits = append(m.packResults.Hits, v.results.Hits...)
					m.packResults.Total = v.results.Total
				}
			}
			m.state = packSearchScreen
		}
		return m, nil, true
	case packDetailMsg:
		if v.request == m.packRequest && m.state == packWorkingScreen && m.packReturn == packDetailScreen {
			m.packBusy = false
			m.packErr = v.err
			if v.err != nil {
				m.state = packSearchScreen
			} else {
				m.packProject, m.packVersion = v.project, v.version
				m.state = packDetailScreen
			}
		}
		return m, nil, true
	case packSearchTick:
		if uint64(v) == m.packRequest && m.state == packSearchScreen {
			m.packBusy = true
			m.state = packWorkingScreen
			m.packReturn = packSearchScreen
			return m, m.packSearch(), true
		}
		return m, nil, true
	}
	return m, nil, false
}

func (m Model) updatePacksKey(key string) (tea.Model, tea.Cmd) {
	switch m.state {
	case modrinthHubScreen:
		switch key {
		case "esc", "q":
			m.state = menuScreen
		case "up", "k":
			m.hubCursor = max(0, m.hubCursor-1)
		case "down", "j":
			m.hubCursor = min(3, m.hubCursor+1)
		case "enter":
			switch m.hubCursor {
			case 0:
				return m.openMods()
			case 1:
				return m.openPacks(packs.Resource)
			case 2:
				return m.openPacks(packs.Shader)
			case 3:
				return m.openModpacks()
			}
		case "ctrl+c":
			return m, tea.Quit
		}
	case packWorkingScreen:
		if key == "esc" && m.packContext != nil {
			m.packCancel()
		}
	case packListScreen:
		switch key {
		case "esc", "q":
			m.stopPackRead()
			m.state = modrinthHubScreen
		case "up", "k":
			m.packCursor = max(0, m.packCursor-1)
		case "down", "j":
			m.packCursor = min(max(0, len(m.packItems)-1), m.packCursor+1)
		case "r":
			m.packBusy = true
			m.state = packWorkingScreen
			return m, m.refreshPacks()
		case "d":
			if len(m.packItems) > 0 {
				m.packTarget = m.packItems[m.packCursor].File
				m.state = packConfirmScreen
			}
		case "a":
			m.pickerForJava, m.pickerForMods, m.pickerForPacks = false, false, true
			m.pickerShowHidden = false
			var err error
			m.pickerPath, err = os.UserHomeDir()
			if err != nil {
				m.packErr = err
				break
			}
			m.loadPickerDir()
			m.state = packPickerScreen
		case "b", "s", "/":
			m.packQuery = ""
			m.packOffset, m.packSortIndex, m.packSearchCursor = 0, 0, 0
			m.packResults = modrinth.SearchResponse{}
			m.packErr = nil
			m.packEngine = "iris"
			m.startPackRead()
			m.packBusy = true
			m.state = packWorkingScreen
			m.packReturn = packSearchScreen
			return m, m.packSearch()
		case "enter":
			if len(m.packItems) == 0 {
				m.packQuery = ""
				m.packOffset, m.packSortIndex, m.packSearchCursor = 0, 0, 0
				m.packResults = modrinth.SearchResponse{}
				m.packErr = nil
				m.packEngine = "iris"
				m.startPackRead()
				m.packBusy = true
				m.state = packWorkingScreen
				m.packReturn = packSearchScreen
				return m, m.packSearch()
			}
		}
	case packPickerScreen:
		switch key {
		case "esc", "q":
			m.pickerForPacks = false
			m.state = packListScreen
		case "ctrl+a":
			if len(m.pickerEntries) > 0 && m.pickerEntries[m.pickerCursor].IsDir() {
				source := filepath.Join(m.pickerPath, m.pickerEntries[m.pickerCursor].Name())
				m.pickerForPacks = false
				return m.packOperation("add", source, "")
			}
		case "up", "k":
			m.pickerCursor = max(0, m.pickerCursor-1)
		case "down", "j":
			m.pickerCursor = min(max(0, len(m.pickerEntries)-1), m.pickerCursor+1)
		case "left", "h":
			m.pickerPath = filepath.Dir(m.pickerPath)
			m.loadPickerDir()
		case "ctrl+h":
			m.pickerShowHidden = !m.pickerShowHidden
			m.loadPickerDir()
		case "enter", "right", "l":
			if len(m.pickerEntries) > 0 {
				entry := m.pickerEntries[m.pickerCursor]
				path := filepath.Join(m.pickerPath, entry.Name())
				if entry.IsDir() {
					m.pickerPath = path
					m.loadPickerDir()
				} else if key == "enter" {
					m.pickerForPacks = false
					return m.packOperation("add", path, "")
				}
			}
		}
	case packConfirmScreen:
		switch key {
		case "esc", "n", "q":
			m.state = packListScreen
		case "y", "enter":
			return m.packOperation("remove", "", m.packTarget)
		}
	case packSearchScreen:
		switch key {
		case "esc":
			m.stopPackRead()
			m.state = packListScreen
		case "tab":
			m.packSortIndex = (m.packSortIndex + 1) % len(catalogSorts)
			return m.restartPackSearch()
		case "up":
			m.packSearchCursor = max(0, m.packSearchCursor-1)
		case "down":
			m.packSearchCursor = min(max(0, len(m.packResults.Hits)-1), m.packSearchCursor+1)
			if len(m.packResults.Hits) > 0 && m.packSearchCursor == len(m.packResults.Hits)-1 && len(m.packResults.Hits) < m.packResults.Total {
				m.packOffset = len(m.packResults.Hits)
				m.packBusy = true
				m.state = packWorkingScreen
				m.packReturn = packSearchScreen
				return m, m.packSearch()
			}
		case "enter":
			if len(m.packResults.Hits) > 0 {
				m.startPackRead()
				m.packBusy = true
				m.state = packWorkingScreen
				m.packReturn = packDetailScreen
				return m, m.packDetail()
			}
		case "backspace":
			if len(m.packQuery) > 0 {
				_, size := utf8.DecodeLastRuneInString(m.packQuery)
				m.packQuery = m.packQuery[:len(m.packQuery)-size]
				return m.debouncePackSearch()
			}
		case "ctrl+e":
			if m.packKind == packs.Shader {
				if m.packEngine == "optifine" {
					m.packEngine = "iris"
				} else {
					m.packEngine = "optifine"
				}
				return m.restartPackSearch()
			}
		default:
			if utf8.RuneCountInString(key) == 1 && key != "/" {
				m.packQuery += key
				return m.debouncePackSearch()
			}
		}
	case packDetailScreen:
		switch key {
		case "esc":
			m.state = packSearchScreen
		case "enter":
			m.state = packReviewScreen
		}
	case packReviewScreen:
		switch key {
		case "esc", "n":
			m.state = packDetailScreen
		case "y":
			m.startPackRead()
			m.packBusy = true
			m.state = packWorkingScreen
			m.packReturn = packListScreen
			return m, m.packInstall()
		}
	}
	return m, nil
}

func (m Model) restartPackSearch() (tea.Model, tea.Cmd) {
	m.startPackRead()
	m.packOffset = 0
	m.packResults = modrinth.SearchResponse{}
	m.packErr = nil
	m.packBusy = true
	m.state = packWorkingScreen
	m.packReturn = packSearchScreen
	return m, m.packSearch()
}

func (m Model) debouncePackSearch() (tea.Model, tea.Cmd) {
	m.startPackRead()
	m.packOffset = 0
	m.packResults = modrinth.SearchResponse{}
	m.packErr = nil
	req := m.packRequest
	return m, tea.Tick(350*time.Millisecond, func(time.Time) tea.Msg { return packSearchTick(req) })
}

func (m Model) renderPacks() string {
	var b strings.Builder

	titleText := i18n.T("packs.hub_title")
	if m.state != modrinthHubScreen {
		titleText = i18n.T("packs.resource_title")
		if m.packKind == packs.Shader {
			titleText = i18n.T("packs.shader_title")
		}
	}
	title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(titleText)
	div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", max(46, m.centerWidth()-4)))
	b.WriteString(title + "\n" + div + "\n\n")

	if m.state == modrinthHubScreen {
		b.WriteString(labelStyle.Render(i18n.T("common.instance_label")) + "  " +
			lipgloss.NewStyle().Foreground(colorAccent2).Bold(true).Render(modText(m.instance.Name)) + "\n\n")
		for i, label := range []string{i18n.T("menu.mods"), i18n.T("packs.resource"), i18n.T("packs.shader"), i18n.T("packs.modpack")} {
			if m.hubCursor == i {
				b.WriteString(cursorStyle.Render("► "+label) + "\n")
			} else {
				b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("  "+label) + "\n")
			}
		}
		return b.String()
	}

	b.WriteString(labelStyle.Render(i18n.T("common.instance_label")) + "  " +
		lipgloss.NewStyle().Foreground(colorAccent2).Bold(true).Render(modText(m.instance.Name)) + "\n")

	versionText := m.packMinecraftVersion()
	if m.packKind == packs.Shader {
		versionText += " / " + strings.ToUpper(m.packCategory())
	} else if m.instance.Loader != "" {
		versionText += " / " + m.instance.Loader
	}
	b.WriteString(labelStyle.Render(i18n.T("common.version_label")) + "  " +
		lipgloss.NewStyle().Foreground(colorGray).Render(modText(versionText)) + "\n")

	pathWidth := max(20, m.centerWidth()-16)
	b.WriteString(labelStyle.Render(i18n.T("common.path_label")) + "  " +
		lipgloss.NewStyle().Foreground(colorDimGray).Render(formatDisplayPath(modText(m.packStore.GameDir), pathWidth)) + "\n\n")
	switch m.state {
	case packWorkingScreen:
		msg := i18n.T("packs.working")
		if m.packContext != nil && m.packContext.Err() != nil {
			msg = i18n.T("packs.loading")
		}
		b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(msg) + "\n")

	case packConfirmScreen:
		warn := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
		b.WriteString(warn + "\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render(i18n.T("mods.confirm_delete_title")) + "\n\n")
		b.WriteString(labelStyle.Render(i18n.T("mods.file_label")) + lipgloss.NewStyle().Foreground(colorWhite).Render(modText(m.packTarget)) + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("packs.confirm_delete_desc")) + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render(i18n.T("mods.delete_hint")) +
			"   " + lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("common.cancel")))

	case packPickerScreen:
		warn := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
		b.WriteString(warn + "\n")
		addTitle := i18n.T("packs.add_resource_title")
		if m.packKind == packs.Shader {
			addTitle = i18n.T("packs.add_shader_title")
		}
		b.WriteString(lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(addTitle) + "\n\n")
		b.WriteString(labelStyle.Render(i18n.T("mods.picker_path")) + "  " +
			lipgloss.NewStyle().Foreground(colorGray).Render(modText(m.pickerPath)) + "\n\n")
		if m.pickerErr != nil {
			b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Render("⚠ "+modText(displayError(m.pickerErr))) + "\n\n")
		}
		available := m.centerInnerHeight() - 14
		if m.pickerErr != nil {
			available -= 2
		}
		count := max(1, available)
		start := max(0, m.pickerCursor-count+1)
		for i := start; i < min(len(m.pickerEntries), start+count); i++ {
			entry := m.pickerEntries[i]
			if i == m.pickerCursor {
				b.WriteString(cursorStyle.Render("► "+modText(entry.Name())) + "\n")
			} else {
				b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("  "+modText(entry.Name())) + "\n")
			}
		}
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(colorDimGray).Render(i18n.T("packs.picker_hint")))

	default: // packListScreen
		if m.packErr != nil {
			b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Render("⚠ "+modText(displayError(m.packErr))) + "\n\n")
		}
		if len(m.packItems) == 0 {
			b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("  "+i18n.T("packs.empty")) + "\n")
			b.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render("  "+i18n.T("packs.search_prompt")) + "\n\n")
		}
		available := m.centerInnerHeight() - 7
		if m.packErr != nil {
			available -= 2
		}
		if len(m.packItems) == 0 {
			available -= 3
		}
		count := max(1, available)
		start := max(0, m.packCursor-count+1)
		for i := start; i < min(len(m.packItems), start+count); i++ {
			item := m.packItems[i]

			stateStr := lipgloss.NewStyle().Foreground(colorGreen).Render(i18n.T("packs.installed"))
			if item.Problem != "" {
				stateStr = lipgloss.NewStyle().Foreground(colorRed).Render(i18n.T("mods.corrupt"))
			}

			var flags []string
			if item.Directory {
				flags = append(flags, lipgloss.NewStyle().Foreground(colorAccent).Render(i18n.T("packs.folder")))
			}
			if item.FormatRange != "" {
				flags = append(flags, lipgloss.NewStyle().Foreground(colorGray).Render("v"+item.FormatRange))
			}
			if item.Problem != "" {
				flags = append(flags, lipgloss.NewStyle().Foreground(colorRed).Render(item.Problem))
			}

			nameStr := item.Name
			if nameStr == "" {
				nameStr = item.File
			}
			row := stateStr + "  " + lipgloss.NewStyle().Foreground(colorWhite).Render(modText(nameStr))
			if len(flags) > 0 {
				row += "  " + strings.Join(flags, " / ")
			}
			rowWidth := max(44, m.centerWidth()-6)
			if i == m.packCursor {
				b.WriteString(cursorStyle.Render("► ") + lipgloss.NewStyle().MaxWidth(rowWidth).Render(row) + "\n")
			} else {
				b.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render("  ") + lipgloss.NewStyle().MaxWidth(rowWidth).Render(row) + "\n")
			}
		}
		b.WriteString("\n")
		footerRaw := i18n.T("packs.footer", len(m.packItems))
		for _, fl := range strings.Split(footerRaw, "\n") {
			b.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render("  "+fl) + "\n")
		}
	}
	return b.String()
}

func (m Model) renderPackCatalog() string {
	var b strings.Builder
	headerWidth := max(20, m.centerWidth()-6)
	headerLines := wrapResetLines([]string{i18n.T("modrinth.catalog_title", modText(m.instance.Name))}, headerWidth)

	var filterText string
	if m.packKind == packs.Shader {
		filterText = i18n.T("packs.catalog_filter_shader", strings.ToUpper(m.packCategory()), modText(m.packMinecraftVersion()), i18n.T("modrinth.sort."+m.catalogSort()))
	} else {
		filterText = i18n.T("packs.catalog_filter_resource", modText(m.packMinecraftVersion()), i18n.T("modrinth.sort."+m.catalogSort()))
	}
	filterLines := wrapResetLines([]string{filterText}, headerWidth)
	b.WriteString(strings.Join(headerLines, "\n") + "\n" + strings.Join(filterLines, "\n") + "\n\n")

	if m.packErr != nil {
		b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render("⚠ "+i18n.T("modrinth.error_title")) + "\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Render(modText(m.packErr.Error())) + "\n\n")
	}

	switch m.state {
	case packSearchScreen:
		b.WriteString(i18n.T("modrinth.search_label", modText(m.packQuery)) + "\n")
		textWidth := max(20, m.centerWidth()-6)
		useTable := m.width >= 75 && textWidth >= 55
		descWidth := max(10, textWidth-54)
		if useTable && len(m.packResults.Hits) > 0 {
			header := "  " + padCol(i18n.T("modrinth.col_name"), 22) + "  " + padCol(i18n.T("modrinth.col_author"), 14) + "  " + padCol(i18n.T("modrinth.col_downloads"), 10) + "  " + padCol(i18n.T("modrinth.col_description"), descWidth)
			b.WriteString(lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(header) + "\n")
			divWidth := max(40, textWidth-2)
			b.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render("  "+strings.Repeat("─", divWidth)) + "\n")
		}
		if len(m.packResults.Hits) == 0 {
			b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("  "+i18n.T("packs.search_empty")) + "\n")
		}
		lines := make([]string, 0, len(m.packResults.Hits))
		for i, hit := range m.packResults.Hits {
			prefix := "  "
			if i == m.packSearchCursor {
				prefix = "► "
			}
			if useTable {
				desc := padCol(modText(hit.Description), descWidth)
				lines = append(lines, prefix+padCol(modText(hit.Title), 22)+"  "+padCol(modText(hit.Author), 14)+"  "+padCol(formatDownloads(hit.Downloads), 10)+"  "+desc)
			} else {
				lines = append(lines, prefix+modText(hit.Title)+" · "+modText(hit.Author)+" · "+formatDownloads(hit.Downloads)+" — "+modText(hit.Description))
			}
		}
		reserve := 7
		if useTable && len(m.packResults.Hits) > 0 {
			reserve = 9
		}
		b.WriteString(m.catalogPage(lines, m.packSearchCursor, reserve))

		countLabel := i18n.T("packs.search_results_count_resource", len(m.packResults.Hits), m.packResults.Total)
		if m.packKind == packs.Shader {
			countLabel = i18n.T("packs.search_results_count_shader", len(m.packResults.Hits), m.packResults.Total)
		}
		b.WriteString(countLabel + "\n")
		b.WriteString(i18n.T("modrinth.search_hint"))

	case packDetailScreen, packReviewScreen:
		width := max(20, m.centerWidth()-6)
		titleLines := wrapResetLines([]string{modText(m.packProject.Title)}, width)
		file, hasFile := m.packVersion.PrimaryFile()
		fileLines := []string{}
		if hasFile {
			fileLines = wrapResetLines([]string{i18n.T("mods.file_label") + "  " + modText(file.Filename) + " · " + fmt.Sprintf("%.1f MiB", float64(file.Size)/(1<<20))}, width)
		}
		footerLines := 2 // blank line and detail hint
		promptLines := []string{}
		if m.state == packReviewScreen && hasFile {
			promptLines = wrapResetLines([]string{i18n.T("packs.confirm_download", modText(m.packProject.Title), modText(file.Filename))}, width)
			footerLines = 4 + len(promptLines) // blank, divider, prompt, blank, confirmation
		}
		fixed := len(headerLines) + len(filterLines) + 2 + len(titleLines) + 1 + 1 + len(fileLines) + footerLines
		available := max(0, m.centerInnerHeight()-fixed)
		descLines := wrapResetLines(strings.Split(modText(strings.TrimSpace(m.packProject.Description)), "\n"), width)
		if len(descLines) > available {
			descLines = descLines[:available]
			if available > 0 {
				descLines[available-1] = runewidth.Truncate(strings.TrimRight(descLines[available-1], " .")+"…", width, "…")
			}
		}
		b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(strings.Join(titleLines, "\n")) + "\n")
		if len(descLines) > 0 {
			b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(strings.Join(descLines, "\n")) + "\n")
		}
		b.WriteString("\n")
		b.WriteString(labelStyle.Render(i18n.T("common.version_label")) + "  " +
			lipgloss.NewStyle().Foreground(colorAccent2).Render(modText(m.packVersion.Number)) + "\n")
		if hasFile {
			b.WriteString(lipgloss.NewStyle().Foreground(colorWhite).Render(strings.Join(fileLines, "\n")) + "\n")
			if m.state == packReviewScreen {
				warn := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
				b.WriteString("\n" + warn + "\n")
				b.WriteString(strings.Join(promptLines, "\n") + "\n\n")
				b.WriteString(lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render(i18n.T("common.confirm")) +
					"   " + lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("common.cancel")) + "\n")
			}
		}
		if m.state != packReviewScreen {
			b.WriteString("\n" + lipgloss.NewStyle().Foreground(colorDimGray).Render(i18n.T("packs.detail_hint")))
		}
	}
	return b.String()
}
