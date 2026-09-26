package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"mcTUI/internal/i18n"
	"mcTUI/internal/modrinth"
	"mcTUI/internal/mods"
)

type catalogSearchMsg struct {
	Request uint64
	Offset  int
	Results modrinth.SearchResponse
	Err     error
}
type catalogTickMsg uint64
type catalogDetailMsg struct {
	Request uint64
	Project modrinth.Project
	Version modrinth.Version
	Err     error
}
type catalogPlanMsg struct {
	Request uint64
	Plan    modrinth.InstallPlan
	Err     error
}
type catalogUpdatesMsg struct {
	Request  uint64
	Versions []modrinth.Version
	Records  []mods.Record
	Items    []mods.Mod
	Err      error
}
type catalogInstalledMsg struct {
	Items []mods.Mod
	Err   error
}
type catalogProgressMsg struct{ Downloaded int64 }

var catalogAPI = modrinth.NewClient() // Share Modrinth rate limits across searches, updates and downloads.
var catalogSorts = []string{"relevance", "downloads", "follows", "newest", "updated"}

func (m Model) catalogSort() string {
	if m.catalogSortIndex >= 0 && m.catalogSortIndex < len(catalogSorts) {
		return catalogSorts[m.catalogSortIndex]
	}
	return "relevance"
}

func formatDownloads(n int64) string {
	if n <= 0 {
		return "0"
	}
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1000000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1000000)
}

func padCol(s string, width int) string {
	r := []rune(s)
	if len(r) > width {
		if width > 1 {
			return string(r[:width-1]) + "…"
		}
		return string(r[:width])
	}
	return s + strings.Repeat(" ", width-len(r))
}

func (m Model) catalogClient() *modrinth.Client { return catalogAPI }
func (m *Model) startCatalogRead() {
	if m.catalogCancel != nil {
		m.catalogCancel()
	}
	m.catalogContext, m.catalogCancel = context.WithCancel(context.Background())
	m.catalogRequest++
}
func (m *Model) stopCatalogRead() {
	if m.catalogCancel != nil {
		m.catalogCancel()
		m.catalogCancel = nil
	}
	m.catalogContext = nil
	m.catalogRequest++
}
func (m Model) catalogChannel() string {
	if m.instance.ModChannel == "beta" || m.instance.ModChannel == "alpha" {
		return m.instance.ModChannel
	}
	return "release"
}
func (m Model) catalogSearch() tea.Cmd {
	query, loader, mc, sort, request, offset, ctx := m.catalogQuery, m.instance.Loader, m.instance.MinecraftVersion, m.catalogSort(), m.catalogRequest, m.catalogOffset, m.catalogContext
	return func() tea.Msg {
		results, err := m.catalogClient().Search(ctx, query, loader, mc, offset, sort)
		return catalogSearchMsg{request, offset, results, err}
	}
}
func (m Model) catalogDebounce() tea.Cmd {
	req := m.catalogRequest
	return tea.Tick(350*time.Millisecond, func(time.Time) tea.Msg { return catalogTickMsg(req) })
}
func (m Model) catalogDetail() tea.Cmd {
	id := m.catalogResults.Hits[m.catalogCursor].ProjectID
	loader, mc, request, ctx := m.instance.Loader, m.instance.MinecraftVersion, m.catalogRequest, m.catalogContext
	return func() tea.Msg {
		c := m.catalogClient()
		p, err := c.Project(ctx, id)
		if err != nil {
			return catalogDetailMsg{Request: request, Err: err}
		}
		v, err := c.CompatibleVersion(ctx, id, loader, mc, m.catalogChannel())
		return catalogDetailMsg{Request: request, Project: p, Version: v, Err: err}
	}
}
func (m Model) catalogResolve() tea.Cmd {
	projectID, loader, mc, channel, request, ctx := m.catalogVersion.ProjectID, m.instance.Loader, m.instance.MinecraftVersion, m.catalogChannel(), m.catalogRequest, m.catalogContext
	installed := map[string]string{}
	for _, item := range m.modItems {
		if item.Enabled && item.ProjectID != "" {
			installed[item.ProjectID] = item.VersionID
		}
	}
	return func() tea.Msg {
		plan, err := m.catalogClient().ResolveLatestCompatible(ctx, projectID, strings.ToLower(loader), mc, channel, installed)
		return catalogPlanMsg{Request: request, Plan: plan, Err: err}
	}
}
func (m Model) catalogResolveUpdates() tea.Cmd {
	selected := []modrinth.Version{}
	for _, v := range m.catalogUpdates {
		if m.catalogSelected[v.ProjectID] {
			selected = append(selected, v)
		}
	}
	loader, mc, request, ctx := strings.ToLower(m.instance.Loader), m.instance.MinecraftVersion, m.catalogRequest, m.catalogContext
	installed := map[string]string{}
	for _, item := range m.modItems {
		if item.Enabled && item.ProjectID != "" {
			installed[item.ProjectID] = item.VersionID
		}
	}
	return func() tea.Msg {
		if len(selected) == 0 {
			return catalogPlanMsg{Request: request, Err: fmt.Errorf("%s", i18n.T("modrinth.no_selection"))}
		}
		c := m.catalogClient()
		plan, err := c.ResolveBatch(ctx, selected, loader, mc, m.catalogChannel(), installed)
		return catalogPlanMsg{Request: request, Plan: plan, Err: err}
	}
}
func (m Model) catalogCheckUpdates() tea.Cmd {
	loader, mc, channel, store, request, ctx := m.instance.Loader, m.instance.MinecraftVersion, m.catalogChannel(), m.modStore, m.catalogRequest, m.catalogContext
	return func() tea.Msg {
		client := m.catalogClient()
		if err := store.Identify(ctx, client); err != nil {
			return catalogUpdatesMsg{Request: request, Err: err}
		}
		items, err := store.Scan()
		if err != nil {
			return catalogUpdatesMsg{Request: request, Err: err}
		}
		hashes := []string{}
		byHash := map[string]mods.Record{}
		for _, item := range items {
			if item.Enabled && !item.Changed && item.ProjectID != "" && item.Hashes.SHA512 != "" {
				hashes = append(hashes, item.Hashes.SHA512)
				byHash[item.Hashes.SHA512] = item.Record
			}
		}
		if len(hashes) == 0 {
			return catalogUpdatesMsg{Request: request, Items: items}
		}
		updates, err := client.Updates(ctx, hashes, strings.ToLower(loader), mc)
		if err != nil {
			return catalogUpdatesMsg{Request: request, Items: items, Err: err}
		}
		out := catalogUpdatesMsg{Request: request, Items: items}
		for _, hash := range hashes {
			v, ok := updates[hash]
			record := byHash[hash]
			if !ok || v.ID == record.VersionID || v.ProjectID != record.ProjectID {
				continue
			}
			if _, err = modrinth.Compatible([]modrinth.Version{v}, strings.ToLower(loader), mc, channel); err != nil {
				// Batch updates have no channel filter: a newer beta can hide a release.
				if _, err = modrinth.Compatible([]modrinth.Version{v}, strings.ToLower(loader), mc, "alpha"); err != nil {
					continue
				}
				versions, fetchErr := client.Versions(ctx, record.ProjectID, strings.ToLower(loader), mc)
				if fetchErr != nil {
					out.Err = fetchErr
					return out
				}
				v, err = modrinth.Compatible(versions, strings.ToLower(loader), mc, channel)
				if err != nil {
					continue
				}
			}
			if v.ID != record.VersionID {
				out.Versions = append(out.Versions, v)
				out.Records = append(out.Records, record)
			}
		}
		return out
	}
}
func (m Model) catalogSelectedItems() []modrinth.PlanItem {
	items := append([]modrinth.PlanItem(nil), m.catalogPlan.Targets...)
	items = append(items, m.catalogPlan.RequiredDeps...)
	chosen := []string{}
	for id, enabled := range m.catalogOptional {
		if enabled {
			chosen = append(chosen, id)
		}
	}
	return append(items, m.catalogPlan.SelectedOptional(chosen)...)
}

func (m Model) catalogSelectedConflict() error {
	selected := map[string]bool{}
	for _, item := range m.catalogSelectedItems() {
		selected[item.Version.ProjectID] = true
	}
	for _, item := range m.catalogSelectedItems() {
		for _, id := range item.IncompatibleIDs {
			if selected[id] {
				return fmt.Errorf("%s", i18n.T("modrinth.optional_conflict", modText(item.Version.ProjectID), modText(id)))
			}
			for _, local := range m.modItems {
				if local.Enabled && local.ProjectID == id {
					return fmt.Errorf("%s", i18n.T("modrinth.optional_conflict", modText(item.Version.ProjectID), modText(id)))
				}
			}
		}
	}
	return nil
}

func (m Model) catalogInstall(ctx context.Context, plan modrinth.InstallPlan) tea.Cmd {
	store := m.modStore
	optional := []string{}
	for id, enabled := range m.catalogOptional {
		if enabled {
			optional = append(optional, id)
		}
	}
	overwrite, events := m.catalogModifiedConfirm, m.catalogEvents
	return func() tea.Msg {
		go func() {
			err := store.Install(ctx, catalogAPI, plan, optional, overwrite, func(n, _ int64) {
				select {
				case events <- catalogProgressMsg{n}:
				default: // Drop intermediate updates rather than block the download.
				}
			})
			items, scanErr := store.Scan()
			if err == nil {
				err = scanErr
			}
			events <- catalogInstalledMsg{items, err}
		}()
		return <-events
	}
}
func (m Model) catalogWait() tea.Cmd {
	events := m.catalogEvents
	return func() tea.Msg { return <-events }
}
func (m Model) updateCatalog(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch v := msg.(type) {
	case catalogTickMsg:
		if m.state == modrinthSearchScreen && uint64(v) == m.catalogRequest {
			return m, m.catalogSearch(), true
		}
		return m, nil, true
	case catalogSearchMsg:
		if m.state == modrinthSearchScreen && v.Request == m.catalogRequest {
			m.catalogLoading = false
			if v.Err == nil {
				if v.Offset > 0 {
					m.catalogResults.Hits = append(m.catalogResults.Hits, v.Results.Hits...)
					m.catalogResults.Limit, m.catalogResults.Total = v.Results.Limit, v.Results.Total
				} else {
					m.catalogResults = v.Results
					m.catalogCursor = 0
				}
				m.catalogOffset = v.Offset + len(v.Results.Hits)
			}
			m.catalogErr = v.Err
		}
		return m, nil, true
	case catalogDetailMsg:
		if m.state == modrinthWorkingScreen && v.Request == m.catalogRequest && m.catalogEvents == nil {
			m.stopCatalogRead()
			m.catalogProject, m.catalogVersion, m.catalogErr = v.Project, v.Version, v.Err
			m.state = modrinthDetailScreen
		}
		return m, nil, true
	case catalogPlanMsg:
		if m.state == modrinthWorkingScreen && v.Request == m.catalogRequest && m.catalogEvents == nil {
			m.stopCatalogRead()
			m.catalogPlan, m.catalogErr = v.Plan, v.Err
			if v.Err == nil {
				m.catalogVersion = v.Plan.TargetMod
			}
			m.catalogOptional = map[string]bool{}
			m.catalogCursor = 0
			m.catalogModifiedConfirm = false
			m.state = modrinthReviewScreen
		}
		return m, nil, true
	case catalogUpdatesMsg:
		if m.state == modrinthWorkingScreen && v.Request == m.catalogRequest && m.catalogEvents == nil {
			m.stopCatalogRead()
			if v.Items != nil {
				m.modItems = v.Items
			}
			m.catalogUpdates, m.catalogUpdateRecords, m.catalogErr = v.Versions, v.Records, v.Err
			m.catalogCursorUpdate = 0
			m.catalogSelected = map[string]bool{}
			for _, item := range v.Versions {
				m.catalogSelected[item.ProjectID] = true
			}
			m.state = modrinthUpdatesScreen
		}
		return m, nil, true
	case catalogProgressMsg:
		if m.state == modrinthWorkingScreen && m.catalogEvents != nil {
			m.catalogDownloaded = v.Downloaded
			return m, m.catalogWait(), true
		}
		return m, nil, true
	case catalogInstalledMsg:
		if m.state == modrinthWorkingScreen && m.catalogEvents != nil {
			m.modItems, m.modErr = v.Items, v.Err
			m.catalogCancel = nil
			m.catalogContext = nil
			m.catalogEvents = nil
			m.state = modsScreen
		}
		return m, nil, true
	}
	return m, nil, false
}
func (m Model) updateCatalogKey(key string) (tea.Model, tea.Cmd) {
	if key == "esc" || key == "n" && m.state != modrinthSearchScreen {
		if m.state == modrinthWorkingScreen {
			if m.catalogEvents != nil {
				m.catalogCancel() // wait for installation cleanup before leaving
				return m, nil
			}
			m.stopCatalogRead()
		}
		if m.state == modrinthSearchScreen || m.state == modrinthDetailScreen {
			m.stopCatalogRead()
		}
		if m.state == modrinthDetailScreen {
			m.startCatalogRead()
			m.state = modrinthSearchScreen
			if m.catalogLoading || len(m.catalogResults.Hits) == 0 {
				m.catalogLoading = true
				m.catalogOffset = 0
				return m, m.catalogSearch()
			}
		} else {
			m.state = modsScreen
		}
		return m, nil
	}
	switch m.state {
	case modrinthSearchScreen:
		switch key {
		case "tab":
			m.catalogSortIndex = (m.catalogSortIndex + 1) % len(catalogSorts)
			m.catalogOffset = 0
			m.catalogResults = modrinth.SearchResponse{}
			m.catalogCursor = 0
			m.catalogLoading = true
			m.startCatalogRead()
			return m, m.catalogSearch()
		case "up":
			m.catalogCursor = max(0, m.catalogCursor-1)
		case "down":
			m.catalogCursor = min(max(0, len(m.catalogResults.Hits)-1), m.catalogCursor+1)
			if len(m.catalogResults.Hits) > 0 && m.catalogCursor == len(m.catalogResults.Hits)-1 && m.catalogOffset < m.catalogResults.Total && !m.catalogLoading {
				m.catalogLoading = true
				return m, m.catalogSearch()
			}
		case "enter":
			if len(m.catalogResults.Hits) > 0 {
				m.startCatalogRead()
				m.state = modrinthWorkingScreen
				return m, m.catalogDetail()
			}
		case "backspace":
			if len(m.catalogQuery) > 0 {
				_, size := utf8.DecodeLastRuneInString(m.catalogQuery)
				m.catalogQuery = m.catalogQuery[:len(m.catalogQuery)-size]
				m.catalogOffset = 0
				m.catalogResults = modrinth.SearchResponse{}
				m.catalogLoading = false
				m.startCatalogRead()
				return m, m.catalogDebounce()
			}
		default:
			if utf8.RuneCountInString(key) == 1 && key != "/" {
				m.catalogQuery += key
				m.catalogOffset = 0
				m.catalogResults = modrinth.SearchResponse{}
				m.catalogLoading = false
				m.startCatalogRead()
				return m, m.catalogDebounce()
			}
		}
	case modrinthDetailScreen:
		if key == "enter" {
			m.startCatalogRead()
			m.state = modrinthWorkingScreen
			return m, m.catalogResolve()
		}
	case modrinthReviewScreen:
		if key == " " && len(m.catalogPlan.OptionalDeps) > 0 {
			id := m.catalogPlan.OptionalDeps[m.catalogCursor%len(m.catalogPlan.OptionalDeps)].Version.ProjectID
			m.catalogOptional[id] = !m.catalogOptional[id]
			m.catalogModifiedConfirm = false
			m.catalogErr = nil
		}
		if key == "up" {
			m.catalogCursor = max(0, m.catalogCursor-1)
		}
		if key == "down" {
			m.catalogCursor = min(max(0, len(m.catalogPlan.OptionalDeps)-1), m.catalogCursor+1)
		}
		if key == "m" {
			m.catalogModifiedConfirm = true
			m.catalogErr = nil
		}
		if key == "y" && m.catalogErr == nil {
			if err := m.catalogSelectedConflict(); err != nil {
				m.catalogErr = err
				break
			}
			changed := false
			for _, planned := range m.catalogSelectedItems() {
				for _, local := range m.modItems {
					if local.ProjectID == planned.Version.ProjectID && local.Changed && !planned.AlreadyInstalled {
						changed = true
					}
				}
			}
			if changed && !m.catalogModifiedConfirm {
				m.catalogErr = fmt.Errorf("%s", i18n.T("modrinth.modified_confirm"))
				break
			}
			ctx, cancel := context.WithCancel(context.Background())
			m.catalogCancel = cancel
			m.catalogEvents = make(chan tea.Msg, 1)
			m.catalogDownloaded, m.catalogTotal = 0, 0
			for _, item := range m.catalogSelectedItems() {
				if !item.AlreadyInstalled {
					m.catalogTotal += item.File.Size
				}
			}
			m.state = modrinthWorkingScreen
			return m, m.catalogInstall(ctx, m.catalogPlan)
		}
	case modrinthUpdatesScreen:
		if key == "up" {
			m.catalogCursorUpdate = max(0, m.catalogCursorUpdate-1)
		}
		if key == "down" {
			m.catalogCursorUpdate = min(max(0, len(m.catalogUpdates)-1), m.catalogCursorUpdate+1)
		}
		if key == " " && len(m.catalogUpdates) > 0 {
			id := m.catalogUpdates[m.catalogCursorUpdate].ProjectID
			m.catalogSelected[id] = !m.catalogSelected[id]
		}
		if key == "y" && len(m.catalogUpdates) > 0 {
			m.startCatalogRead()
			m.state = modrinthWorkingScreen
			return m, m.catalogResolveUpdates()
		}
	case modrinthWorkingScreen:
		// Esc cancels a download via the context above; a worker response cannot navigate back into a closed screen.
	}
	return m, nil
}
func (m Model) renderCatalog() string {
	var b strings.Builder
	b.WriteString(i18n.T("modrinth.catalog_title", modText(m.instance.Name)) + "\n")
	b.WriteString(i18n.T("modrinth.filter_status", modText(m.instance.Loader), modText(m.instance.MinecraftVersion), i18n.T("modrinth.channel."+m.catalogChannel()), i18n.T("modrinth.sort."+m.catalogSort())) + "\n\n")
	if m.catalogErr != nil {
		b.WriteString("⚠ " + modText(m.catalogErr.Error()) + "\n\n")
	}
	switch m.state {
	case modrinthSearchScreen:
		b.WriteString(i18n.T("modrinth.search_label", modText(m.catalogQuery)) + "\n")
		textWidth := m.catalogWidth()
		useTable := m.width >= 75 && textWidth >= 55
		descWidth := max(10, textWidth-54)
		if useTable && len(m.catalogResults.Hits) > 0 {
			header := "  " + padCol(i18n.T("modrinth.col_name"), 22) + "  " + padCol(i18n.T("modrinth.col_author"), 14) + "  " + padCol(i18n.T("modrinth.col_downloads"), 10) + "  " + padCol(i18n.T("modrinth.col_description"), descWidth)
			b.WriteString(lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(header) + "\n")
			divWidth := max(40, textWidth-2)
			b.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render("  "+strings.Repeat("─", divWidth)) + "\n")
		}
		lines := make([]string, 0, len(m.catalogResults.Hits))
		for i, hit := range m.catalogResults.Hits {
			prefix := "  "
			if i == m.catalogCursor {
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
		if useTable && len(m.catalogResults.Hits) > 0 {
			reserve = 9
		}
		b.WriteString(m.catalogPage(lines, m.catalogCursor, reserve))
		b.WriteString(i18n.T("modrinth.search_results_count", len(m.catalogResults.Hits), m.catalogResults.Total) + "\n")
		b.WriteString(i18n.T("modrinth.search_hint"))
	case modrinthDetailScreen:
		titleStr := modText(m.catalogProject.Title)
		if m.catalogProject.Slug != "" && m.catalogProject.Slug != m.catalogProject.Title {
			titleStr += " (" + modText(m.catalogProject.Slug) + ")"
		}
		b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(titleStr) + "\n")
		author := ""
		if m.catalogCursor < len(m.catalogResults.Hits) {
			author = m.catalogResults.Hits[m.catalogCursor].Author
		}
		if author == "" {
			author = m.catalogProject.Team
		}
		metaLine := ""
		if author != "" {
			metaLine += i18n.T("modrinth.detail_by", modText(author)) + "  ·  "
		}
		metaLine += i18n.T("modrinth.detail_downloads", formatDownloads(m.catalogProject.Downloads))
		b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(metaLine) + "\n\n")

		desc := strings.TrimSpace(m.catalogProject.Description)
		if desc != "" {
			b.WriteString(modText(desc) + "\n\n")
		}

		b.WriteString(lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(i18n.T("modrinth.detail_compatible_title")) + "\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 48)) + "\n")
		verNumber := m.catalogVersion.Number
		if verNumber == "" {
			verNumber = m.catalogVersion.Name
		}
		file, hasFile := m.catalogVersion.PrimaryFile()
		fileSize := ""
		if hasFile {
			fileSize = " (" + catalogSize(file.Size) + ")"
		}
		b.WriteString(labelStyle.Render(i18n.T("common.version_label")) + "  " +
			lipgloss.NewStyle().Foreground(colorAccent2).Bold(true).Render(modText(verNumber)) + "  " +
			lipgloss.NewStyle().Foreground(colorAccent).Render("["+modText(m.catalogVersion.Channel)+"]") + fileSize + "\n")
		if hasFile && file.Filename != "" {
			b.WriteString(labelStyle.Render(i18n.T("mods.file_label")) + "  " +
				lipgloss.NewStyle().Foreground(colorWhite).Render(modText(file.Filename)) + "\n")
		}
		if !m.catalogVersion.Published.IsZero() {
			b.WriteString(labelStyle.Render(i18n.T("modrinth.detail_published", m.catalogVersion.Published.Format("2006-01-02"))) + "\n")
		}
		if len(m.catalogVersion.Loaders) > 0 {
			b.WriteString(i18n.T("modrinth.detail_loaders", modText(strings.Join(m.catalogVersion.Loaders, ", "))) + "  ·  " +
				i18n.T("modrinth.detail_game_versions", modText(strings.Join(m.catalogVersion.GameVersions, ", "))) + "\n")
		}
		b.WriteString("\n")

		b.WriteString(lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(i18n.T("modrinth.detail_deps_title")) + "\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 48)) + "\n")
		var reqDeps, optDeps, incDeps []string
		for _, dep := range m.catalogVersion.Dependencies {
			name := dep.ProjectID
			if name == "" {
				name = dep.VersionID
			}
			if name == "" {
				name = dep.FileName
			}
			switch dep.Type {
			case "required":
				reqDeps = append(reqDeps, name)
			case "optional":
				optDeps = append(optDeps, name)
			case "incompatible":
				incDeps = append(incDeps, name)
			}
		}
		if len(reqDeps) == 0 && len(optDeps) == 0 && len(incDeps) == 0 {
			b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("modrinth.detail_no_deps")) + "\n")
		} else {
			if len(reqDeps) > 0 {
				b.WriteString(lipgloss.NewStyle().Foreground(colorGreen).Render("  ✓ "+i18n.T("modrinth.detail_required_count", len(reqDeps))) + " " + modText(strings.Join(reqDeps, ", ")) + "\n")
			}
			if len(optDeps) > 0 {
				b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Render("  • "+i18n.T("modrinth.detail_optional_count", len(optDeps))) + " " + modText(strings.Join(optDeps, ", ")) + "\n")
			}
			if len(incDeps) > 0 {
				b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Render("  ✗ "+i18n.T("modrinth.detail_incompatible_count", len(incDeps))) + " " + modText(strings.Join(incDeps, ", ")) + "\n")
			}
		}
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(colorDimGray).Render(i18n.T("modrinth.detail_hint")))
	case modrinthReviewScreen:
		b.WriteString(i18n.T("modrinth.plan_title") + "\n")
		b.WriteString(i18n.T("modrinth.destination", modText(filepath.Join(m.modStore.GameDir, "mods"))) + "\n")
		lines := []string{}
		for _, item := range m.catalogPlan.Targets {
			lines = append(lines, i18n.T("modrinth.target_mod", modText(item.File.Filename), modText(item.Version.Number))+" · "+catalogSize(item.File.Size)+catalogInstalledLabel(item))
		}
		lines = append(lines, i18n.T("modrinth.required_deps"))
		for _, item := range m.catalogPlan.RequiredDeps {
			lines = append(lines, "  ✓ "+modText(item.File.Filename)+" · "+modText(item.Version.Number)+" · "+catalogSize(item.File.Size)+catalogInstalledLabel(item))
		}
		lines = append(lines, i18n.T("modrinth.optional_deps"))
		optionalStart := len(lines)
		chosen := []string{}
		for id, enabled := range m.catalogOptional {
			if enabled {
				chosen = append(chosen, id)
			}
		}
		selected := map[string]bool{}
		for _, v := range m.catalogPlan.SelectedOptional(chosen) {
			selected[v.Version.ProjectID] = true
		}
		for i, item := range m.catalogPlan.OptionalDeps {
			prefix, check := "  ", " "
			if m.catalogCursor == i {
				prefix = "► "
			}
			if selected[item.Version.ProjectID] {
				check = "x"
			}
			line := prefix + "[" + check + "] " + modText(item.File.Filename) + " · " + modText(item.Version.Number) + " · " + catalogSize(item.File.Size) + catalogInstalledLabel(item)
			if selected[item.Version.ProjectID] && !m.catalogOptional[item.Version.ProjectID] {
				line += " " + i18n.T("modrinth.required_by_parent", modText(strings.Join(item.RequiredBy, ", ")))
			}
			lines = append(lines, line)
		}
		for _, planned := range m.catalogSelectedItems() {
			for _, local := range m.modItems {
				if local.ProjectID == planned.Version.ProjectID && !planned.AlreadyInstalled {
					lines = append(lines, i18n.T("modrinth.replacement", modText(local.File), modText(planned.File.Filename)))
					if local.Changed {
						lines = append(lines, i18n.T("modrinth.modified_file", modText(local.File)))
					}
				}
			}
		}
		b.WriteString(m.catalogPage(lines, optionalStart+m.catalogCursor, 9))
		b.WriteString(i18n.T("modrinth.conflicts_label") + " ")
		if err := m.catalogSelectedConflict(); err != nil {
			b.WriteString("⚠ " + modText(err.Error()) + "\n")
		} else {
			b.WriteString(i18n.T("modrinth.no_conflicts") + "\n")
		}
		if m.catalogModifiedConfirm {
			b.WriteString(i18n.T("modrinth.modified_approved") + "\n")
		}
		var total int64
		count := 0
		for _, item := range m.catalogSelectedItems() {
			if !item.AlreadyInstalled {
				total += item.File.Size
				count++
			}
		}
		b.WriteString(i18n.T("modrinth.total_download", catalogSize(total), count) + " · SHA-512\n")
		b.WriteString(i18n.T("modrinth.confirm_prompt") + "  " + i18n.T("modrinth.review_hint"))
	case modrinthUpdatesScreen:
		b.WriteString(i18n.T("modrinth.updates_title", len(m.catalogUpdates)) + "\n")
		lines := make([]string, 0, len(m.catalogUpdates))
		for i, v := range m.catalogUpdates {
			prefix, check := "  ", " "
			if i == m.catalogCursorUpdate {
				prefix = "► "
			}
			if m.catalogSelected[v.ProjectID] {
				check = "x"
			}
			old := ""
			if i < len(m.catalogUpdateRecords) {
				old = m.catalogUpdateRecords[i].VersionID
				if old == "" {
					old = m.catalogUpdateRecords[i].File
				}
			}
			for _, local := range m.modItems {
				if local.ProjectID == v.ProjectID && local.Metadata.Version != "" {
					old = local.Metadata.Version
					break
				}
			}
			file, _ := v.PrimaryFile()
			lines = append(lines, fmt.Sprintf("%s[%s] %s: %s → %s · %s [%s]", prefix, check, modText(m.catalogUpdateName(v.ProjectID)), modText(old), modText(v.Number), catalogSize(file.Size), modText(v.Channel)))
		}
		b.WriteString(m.catalogPage(lines, m.catalogCursorUpdate, 7))
		b.WriteString(i18n.T("modrinth.backup_path", modText(filepath.Join(filepath.Dir(m.modStore.Registry), "backups", "mods_replaced"))) + "\n")
		b.WriteString(i18n.T("modrinth.updates_hint"))
	case modrinthWorkingScreen:
		b.WriteString(i18n.T("modrinth.working"))
		if m.catalogEvents != nil {
			b.WriteString(fmt.Sprintf("\n%d / %d bytes (%s / %s)", m.catalogDownloaded, m.catalogTotal, catalogSize(m.catalogDownloaded), catalogSize(m.catalogTotal)))
		}
	}
	return b.String()
}

func catalogSize(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1f KiB", float64(n)/1024)
	}
	return fmt.Sprintf("%.1f MiB", float64(n)/(1024*1024))
}

func catalogInstalledLabel(item modrinth.PlanItem) string {
	if item.AlreadyInstalled {
		return " " + i18n.T("modrinth.already_installed")
	}
	return ""
}

func (m Model) catalogUpdateName(id string) string {
	for _, local := range m.modItems {
		if local.ProjectID == id {
			if local.Metadata.Name != "" {
				return local.Metadata.Name
			}
			return local.File
		}
	}
	return id
}

// catalogPage leaves the selected row visible and the confirmation footer accessible.
func (m Model) catalogPage(lines []string, cursor, reserve int) string {
	centerHeight := max(16, min(24, m.height-18))
	visible := max(1, centerHeight-reserve)
	start := min(max(0, cursor-visible+1), max(0, len(lines)-visible))
	width := m.catalogWidth()
	var b strings.Builder
	if start > 0 {
		b.WriteString("↑\n")
	}
	for _, line := range lines[start:min(len(lines), start+visible)] {
		runes := []rune(line)
		if len(runes) > width {
			line = string(runes[:width-1]) + "…"
		}
		b.WriteString(line + "\n")
	}
	if start+visible < len(lines) {
		b.WriteString("↓\n")
	}
	return b.String()
}
