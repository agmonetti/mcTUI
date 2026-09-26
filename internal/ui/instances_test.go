package ui

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"mcTUI/internal/instances"
)

func TestInstancePickerAndLaunchPathsStayIsolated(t *testing.T) {
	root := t.TempDir()
	a, err := instances.Create(root, "A", "1.20.4", "Vanilla", "", 2048)
	if err != nil {
		t.Fatal(err)
	}
	b, err := instances.Create(root, "B", "1.20.4", "Vanilla", "", 2048)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := instances.Read(root, instances.LegacyID)
	if err != nil {
		t.Fatal(err)
	}
	m := Model{state: menuScreen, dataRoot: root, instance: legacy, username: "Player"}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m = updated.(Model)
	if m.state != instancesScreen {
		t.Fatal("instance picker did not open")
	}
	for i, in := range m.instanceList {
		if in.ID == a.ID {
			m.instanceCursor = i
			break
		}
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.instance.ID != a.ID {
		t.Fatalf("UI selected %q, want %q", m.instance.ID, a.ID)
	}
	launchA, err := m.launchConfig()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(launchA.GameDir, "saves"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(launchA.GameDir, "saves", "world.dat"), []byte("A"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := m.setInstance(b); err != nil {
		t.Fatal(err)
	}
	launchB, err := m.launchConfig()
	if err != nil {
		t.Fatal(err)
	}
	if launchA.GameDir == launchB.GameDir || launchA.RuntimeDir != launchB.RuntimeDir {
		t.Fatalf("game/runtime paths = %q/%q and %q/%q", launchA.GameDir, launchA.RuntimeDir, launchB.GameDir, launchB.RuntimeDir)
	}
	if _, err := os.Stat(filepath.Join(launchB.GameDir, "saves", "world.dat")); !os.IsNotExist(err) {
		t.Fatalf("instance B sees A's world: %v", err)
	}
	selected, err := instances.Selected(root)
	if err != nil || selected.ID != b.ID {
		t.Fatalf("selected = %q, %v; want %q", selected.ID, err, b.ID)
	}
}

func TestNewManagedInstanceSelectsVersionBeforeName(t *testing.T) {
	root := t.TempDir()
	m := Model{state: instancesScreen, dataRoot: root, versions: []string{"1.20.4", "1.21.1"}, versionSelect: "1.20.4", memoryMB: 2048, input: textinput.New()}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = updated.(Model)
	if m.state != versionsScreen || !m.creatingInstance {
		t.Fatalf("new-instance flow = %v, creating=%v; want version picker", m.state, m.creatingInstance)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != instanceNameScreen || m.creationVersion != "1.21.1" {
		t.Fatalf("selected creation version = %q, state=%v", m.creationVersion, m.state)
	}
	m.input.SetValue("Fresh")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != menuScreen || m.instance.MinecraftVersion != "1.21.1" {
		t.Fatalf("created instance = %+v, state=%v", m.instance, m.state)
	}
}

func TestCopyNameAcceptsLetterC(t *testing.T) {
	input := textinput.New()
	input.Focus()
	m := Model{state: instanceNameScreen, copyingInstance: true, input: input}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = updated.(Model)
	if m.input.Value() != "c" {
		t.Fatalf("copy name swallowed 'c': %q", m.input.Value())
	}
}

func TestInstanceCopyViewEscapesTerminalControlsFromPaths(t *testing.T) {
	m := Model{state: instanceCopySelectScreen, copySourceID: instances.LegacyID, copySource: "/tmp/legacy\x1b[31m", dataRoot: "/tmp/data", creationVersion: "1.21.1", copyItems: []instances.CopyItem{{Name: "mods", Exists: true}}}
	if view := m.View(); strings.Contains(view, "\x1b") || !strings.Contains(view, "COPY FROM EXISTING INSTALLATION") {
		t.Fatalf("copy view did not sanitize terminal controls: %q", view)
	}
}

func TestCopyLegacyFlowCreatesAndSelectsIndependentInstance(t *testing.T) {
	root, source := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(source, "mods"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "mods", "example.jar"), []byte("mod-data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "options.txt"), []byte("video"), 0600); err != nil {
		t.Fatal(err)
	}
	legacy := instances.Instance{ID: instances.LegacyID, Kind: "legacy", Name: "Existing"}
	m := Model{state: instancesScreen, dataRoot: root, instance: legacy, instanceList: []instances.Instance{legacy}, versions: []string{"1.21.1"}, versionSelect: "1.21.1", memoryMB: 2048, input: textinput.New()}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = updated.(Model)
	if m.state != versionsScreen || !m.copyingInstance {
		t.Fatalf("legacy copy action did not open version picker: state=%v", m.state)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != instanceNameScreen {
		t.Fatalf("copy version did not open name prompt: state=%v", m.state)
	}
	m.input.SetValue("Imported")
	m.copySource = source // keep this UI test isolated from the user's real .minecraft
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != instanceCopyInspectScreen || cmd == nil {
		t.Fatalf("copy inspection did not start asynchronously: state=%v", m.state)
	}
	inspect, ok := cmd().(InstanceCopyInspectDoneMsg)
	if !ok {
		t.Fatalf("inspection command returned unexpected message")
	}
	updated, _ = m.Update(inspect)
	m = updated.(Model)
	if m.state != instanceCopySelectScreen || len(m.copyItems) != 6 {
		t.Fatalf("copy selection screen/items = %v/%d", m.state, len(m.copyItems))
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = updated.(Model)
	for range 5 {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(Model)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != instanceCopyConfirmScreen || !strings.Contains(m.View(), "mods") || !strings.Contains(m.View(), "Legacy data will not be moved or overwritten") {
		t.Fatalf("copy confirmation missing selection/safety details: state=%v view=%s", m.state, m.View())
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = updated.(Model)
	if m.state != instanceCopyRunningScreen || cmd == nil {
		t.Fatalf("copy did not start asynchronously: state=%v cmd=%v", m.state, cmd)
	}
	msg, ok := cmd().(InstanceCopyDoneMsg)
	if !ok {
		t.Fatalf("copy command returned %T", cmd())
	}
	updated, _ = m.Update(msg)
	m = updated.(Model)
	if m.state != instanceCopyResultScreen || m.copyErr != nil || m.instance.ID == instances.LegacyID || m.instance.MinecraftVersion != "1.21.1" {
		t.Fatalf("copy result/selection = state %v, err %v, instance %+v", m.state, m.copyErr, m.instance)
	}
	gameDir, err := instances.GameDir(root, m.instance)
	if err != nil {
		t.Fatal(err)
	}
	for rel, want := range map[string]string{"mods/example.jar": "mod-data", "options.txt": "video"} {
		data, err := os.ReadFile(filepath.Join(gameDir, filepath.FromSlash(rel)))
		if err != nil || string(data) != want {
			t.Fatalf("copied %s = %q, %v", rel, data, err)
		}
	}
	if err := os.WriteFile(filepath.Join(source, "mods", "example.jar"), []byte("changed source"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(gameDir, "mods", "example.jar"))
	if err != nil || string(data) != "mod-data" {
		t.Fatalf("destination shares source data: %q, %v", data, err)
	}
}

func TestCopyManagedInstanceOpensNameWithoutVersionPicker(t *testing.T) {
	root := t.TempDir()
	in, err := instances.Create(root, "Original", "1.21.1", "Vanilla", "", 2048)
	if err != nil {
		t.Fatal(err)
	}
	m := Model{state: instancesScreen, dataRoot: root, instanceList: []instances.Instance{in}, input: textinput.New()}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = updated.(Model)
	if m.state != instanceNameScreen || m.copySourceID != in.ID || m.creationVersion != in.MinecraftVersion || m.input.Value() != "" {
		t.Fatalf("managed copy setup = state %v, source %q, version %q, name %q", m.state, m.copySourceID, m.creationVersion, m.input.Value())
	}
}

func TestRetireAndRestoreInstanceFromPicker(t *testing.T) {
	root := t.TempDir()
	in, err := instances.Create(root, "Managed", "1.21.1", "Vanilla", "", 2048)
	if err != nil {
		t.Fatal(err)
	}
	if err := instances.Select(root, in.ID); err != nil {
		t.Fatal(err)
	}
	m := Model{state: instancesScreen, dataRoot: root, instance: in, instanceList: []instances.Instance{in}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = updated.(Model)
	if m.state != instanceRemoveChoiceScreen {
		t.Fatalf("remove choice state = %v", m.state)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m = updated.(Model)
	if m.state != instanceRemoveConfirmScreen || !strings.Contains(m.View(), "retired") {
		t.Fatalf("missing soft-delete confirmation: %s", m.View())
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("retire did not start")
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.removeErr != nil || m.instance.ID != instances.LegacyID {
		t.Fatalf("retire result: %v, selected %q", m.removeErr, m.instance.ID)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	m = updated.(Model)
	if m.state != instanceTrashScreen || len(m.retiredList) != 1 {
		t.Fatalf("retired picker: state %v, items %v", m.state, m.retiredList)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = updated.(Model)
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.removeErr != nil {
		t.Fatalf("restore result: %v", m.removeErr)
	}
	if restored, err := instances.Read(root, in.ID); err != nil || restored.ID != in.ID {
		t.Fatalf("restored instance: %+v, %v", restored, err)
	}
}

func TestFullDeleteFromPickerRemovesGameAndBackups(t *testing.T) {
	root := t.TempDir()
	in, err := instances.Create(root, "Erase", "1.21.1", "Vanilla", "", 2048)
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
	backup := filepath.Join(root, "backups", in.ID)
	if err := os.MkdirAll(backup, 0700); err != nil {
		t.Fatal(err)
	}
	m := Model{state: instancesScreen, dataRoot: root, instance: in, instanceList: []instances.Instance{in}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	m = updated.(Model)
	if m.state != instanceRemoveConfirmScreen || !strings.Contains(m.View(), backup) {
		t.Fatalf("delete confirmation missing backups: %s", m.View())
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = updated.(Model)
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.removeErr != nil || m.instance.ID != instances.LegacyID {
		t.Fatalf("full delete: %v, selected %q", m.removeErr, m.instance.ID)
	}
	for _, path := range []string{filepath.Join(root, "instances", in.ID), filepath.Join(root, "retired", in.ID), backup} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("not removed: %s: %v", path, err)
		}
	}
}

func TestRenameInstanceFromPicker(t *testing.T) {
	root := t.TempDir()
	in, err := instances.Create(root, "Before", "1.20.4", "Vanilla", "", 2048)
	if err != nil {
		t.Fatal(err)
	}
	m := Model{state: instancesScreen, dataRoot: root, instance: in, instanceList: []instances.Instance{in}, input: textinput.New()}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m = updated.(Model)
	if m.state != instanceNameScreen || m.input.Value() != "Before" {
		t.Fatalf("rename screen/input = %v/%q", m.state, m.input.Value())
	}
	m.input.SetValue("After")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	renamed, err := instances.Read(root, in.ID)
	if err != nil || renamed.Name != "After" || m.state != instancesScreen {
		t.Fatalf("renamed = %+v, state=%v, err=%v", renamed, m.state, err)
	}
}

func TestManagedSettingsAndJavaPickerPersist(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell Java stub is Unix-only")
	}
	root := t.TempDir()
	in, err := instances.Create(root, "Managed", "1.20.4", "Vanilla", "", 2048)
	if err != nil {
		t.Fatal(err)
	}
	javaDir := t.TempDir()
	javaPath := filepath.Join(javaDir, "java")
	if err := os.WriteFile(javaPath, []byte(`#!/bin/sh
echo 'openjdk version "17.0.1"' >&2
`), 0700); err != nil {
		t.Fatal(err)
	}
	versionCache := filepath.Join(root, "runtime", "versions", in.MinecraftVersion, "version.json")
	if err := os.MkdirAll(filepath.Dir(versionCache), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(versionCache, []byte(`{"javaVersion":{"majorVersion":21}}`), 0600); err != nil {
		t.Fatal(err)
	}
	m := Model{state: javaPickerScreen, dataRoot: root, instance: in, memoryMB: in.MemoryMB, versionSelect: in.MinecraftVersion, modloader: in.Loader, pickerForJava: true, pickerPath: javaDir}
	m.loadPickerDir()
	if len(m.pickerEntries) != 1 || m.pickerEntries[0].Name() != "java" {
		t.Fatalf("Java picker entries = %v", m.pickerEntries)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != javaPickerScreen || m.pickerErr == nil || !strings.Contains(m.pickerErr.Error(), "requires Java 21") {
		t.Fatalf("incompatible Java was not rejected in picker: state=%v err=%v", m.state, m.pickerErr)
	}
	if err := os.WriteFile(javaPath, []byte(`#!/bin/sh
echo 'openjdk version "21.0.1"' >&2
`), 0700); err != nil {
		t.Fatal(err)
	}
	m.loadPickerDir()
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != menuScreen || m.instance.JavaPath != javaPath || m.javaOverrideMajor != 21 {
		t.Fatalf("compatible Java override was not selected: state=%v path=%q major=%d", m.state, m.instance.JavaPath, m.javaOverrideMajor)
	}

	m.cursorMenu = 5
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	m.cursorMenu = 2
	m.versions = []string{"1.21.1"}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != menuScreen || m.versionSelect != "1.20.4" {
		t.Fatalf("managed Minecraft version changed: state=%v version=%q", m.state, m.versionSelect)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	stored, err := instances.Read(root, in.ID)
	if err != nil || stored.JavaPath != javaPath || stored.MemoryMB != 4096 || stored.MinecraftVersion != "1.20.4" {
		t.Fatalf("managed settings not persisted or version changed: %+v, %v", stored, err)
	}
	launch, err := m.launchConfig()
	if err != nil || launch.JavaPath != javaPath {
		t.Fatalf("launch Java override = %q, %v", launch.JavaPath, err)
	}

	m.state, m.pickerForJava = javaPickerScreen, true
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updated.(Model)
	stored, err = instances.Read(root, in.ID)
	if err != nil || stored.JavaPath != "" || m.javaOverrideMajor != 0 {
		t.Fatalf("automatic Java selection was not restored: %+v, %v", stored, err)
	}
}

func TestPinnedLoaderRejectsVanillaBeforePreparation(t *testing.T) {
	launch := LaunchConfig{RuntimeDir: t.TempDir(), Version: "1.20.4", Modloader: "Vanilla", LoaderVersion: "0.15.11"}
	msg, ok := startLaunchDownload(context.Background(), launch)().(DownloadDoneMsg)
	if !ok || msg.Err == nil || msg.JavaBinary != "" {
		t.Fatalf("unsupported loader combination accepted: %+v", msg)
	}
}

func TestManagedFabricCreationAndExactLaunchSnapshot(t *testing.T) {
	root := t.TempDir()
	m := Model{state: instancesScreen, dataRoot: root, versions: []string{"1.20.4"}, memoryMB: 2048, input: textinput.New()}
	press := func(key tea.KeyMsg) { updated, _ := m.Update(key); m = updated.(Model) }
	press(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	press(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if m.state != instanceLoaderScreen || m.creationVersion != "1.20.4" {
		t.Fatalf("Fabric loader prompt missing: state=%v version=%q", m.state, m.creationVersion)
	}
	m.input.SetValue("0.15.11")
	press(tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != instanceNameScreen || m.creationLoaderVersion != "0.15.11" {
		t.Fatalf("loader version not captured: state=%v loader=%q", m.state, m.creationLoaderVersion)
	}
	m.input.SetValue("Fabric instance")
	press(tea.KeyMsg{Type: tea.KeyEnter})
	stored, err := instances.Read(root, m.instance.ID)
	if err != nil || stored.Loader != "Fabric" || stored.LoaderVersion != "0.15.11" || stored.MinecraftVersion != "1.20.4" {
		t.Fatalf("incorrect managed Fabric identity: %+v, %v", stored, err)
	}
	launch, err := m.launchConfig()
	if err != nil || launch.LoaderVersion != "0.15.11" || launch.Modloader != "Fabric" || launch.Version != "1.20.4" {
		t.Fatalf("Fabric launch snapshot = %+v, %v", launch, err)
	}
	m.versionSelect = "1.20.5"
	if _, err := m.launchConfig(); err == nil {
		t.Fatal("changed managed Minecraft version accepted")
	}
	m.versionSelect, m.modloader = stored.MinecraftVersion, "Vanilla"
	if _, err := m.launchConfig(); err == nil {
		t.Fatal("Fabric instance silently launched as Vanilla")
	}
}
