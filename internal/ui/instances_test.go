package ui

import (
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

func TestManagedFabricLaunchIsBlockedUntilExactLoaderSupport(t *testing.T) {
	root := t.TempDir()
	in, err := instances.Create(root, "Fabric", "1.20.4", "Fabric", "0.15.11", 2048)
	if err != nil {
		t.Fatal(err)
	}
	m := Model{dataRoot: root, instance: in, versionSelect: in.MinecraftVersion, modloader: in.Loader, memoryMB: in.MemoryMB}
	if _, err := m.launchConfig(); err == nil {
		t.Fatal("managed Fabric launched without honoring its exact loader version")
	}
}
