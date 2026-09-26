// Package instances stores managed Minecraft installations independently of
// the existing .minecraft installation, which remains a virtual legacy entry.
package instances

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"mcTUI/internal/config"
)

const LegacyID = "legacy"

type Instance struct {
	SchemaVersion    int    `json:"schema_version"`
	ID               string `json:"id"`
	Name             string `json:"name"`
	Kind             string `json:"kind"`
	MinecraftVersion string `json:"minecraft_version"`
	Loader           string `json:"loader"`
	LoaderVersion    string `json:"loader_version,omitempty"`
	JavaPath         string `json:"java_path,omitempty"`
	MemoryMB         int    `json:"memory_mb"`
}

// GameDir returns the private mutable game directory (or the unchanged
// .minecraft directory for the virtual legacy entry).
func GameDir(root string, in Instance) (string, error) {
	if in.ID == LegacyID && in.Kind == "legacy" {
		return config.MinecraftDirChecked()
	}
	if in.Kind != "managed" || !validID(in.ID) || !filepath.IsAbs(root) {
		return "", fmt.Errorf("invalid managed instance path")
	}
	instanceDir := filepath.Join(root, "instances", in.ID)
	if err := checkManagedDirectory(instanceDir, false); err != nil {
		return "", err
	}
	gameDir := filepath.Join(instanceDir, "minecraft")
	if err := checkManagedDirectory(gameDir, true); err != nil {
		return "", err
	}
	return gameDir, nil
}

func RuntimeDir(root string, in Instance) (string, error) {
	if in.ID == LegacyID && in.Kind == "legacy" {
		return config.MinecraftDirChecked()
	}
	if in.Kind != "managed" || !validID(in.ID) || !filepath.IsAbs(root) {
		return "", fmt.Errorf("invalid managed instance path")
	}
	runtimeDir := filepath.Join(root, "runtime")
	if err := checkManagedDirectory(runtimeDir, true); err != nil {
		return "", err
	}
	return runtimeDir, nil
}

// BackupDir keeps managed Reset history separate from mutable game data.
// The legacy entry retains its existing in-game history unchanged.
func BackupDir(root string, in Instance) (string, error) {
	if in.ID == LegacyID && in.Kind == "legacy" {
		gameDir, err := config.MinecraftDirChecked()
		return filepath.Join(gameDir, "mctui-backups"), err
	}
	if in.Kind != "managed" || !validID(in.ID) || !filepath.IsAbs(root) {
		return "", fmt.Errorf("invalid managed backup path")
	}
	parent := filepath.Join(root, "backups")
	if err := checkManagedDirectory(parent, true); err != nil {
		return "", err
	}
	path := filepath.Join(parent, in.ID)
	if err := checkManagedDirectory(path, true); err != nil {
		return "", err
	}
	return path, nil
}

func legacy() Instance {
	cfg := config.Load()
	return Instance{SchemaVersion: 1, ID: LegacyID, Name: "Instalación existente", Kind: "legacy", MinecraftVersion: cfg.Version, Loader: cfg.Modloader, MemoryMB: cfg.MemoryMB}
}

// List includes the existing installation without writing into it. An
// invalid or unreadable managed entry is an error, not a silently hidden one.
func List(root string) ([]Instance, error) {
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("data root must be absolute")
	}
	result := []Instance{legacy()}
	if err := checkInstancesDir(root); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(root, "instances"))
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !validID(entry.Name()) {
			return nil, fmt.Errorf("unexpected entry in instances: %q", entry.Name())
		}
		in, err := Read(root, entry.Name())
		if err != nil {
			return nil, err
		}
		result = append(result, in)
	}
	return result, nil
}

func Read(root, id string) (Instance, error) {
	if id == LegacyID {
		return legacy(), nil
	}
	if !filepath.IsAbs(root) || !validID(id) {
		return Instance{}, fmt.Errorf("invalid instance ID or data root")
	}
	if err := checkInstancesDir(root); err != nil {
		return Instance{}, err
	}
	dir := filepath.Join(root, "instances", id)
	info, err := os.Lstat(dir)
	if err != nil {
		return Instance{}, err
	}
	if !info.IsDir() {
		return Instance{}, fmt.Errorf("instance directory is not a directory: %s", dir)
	}
	path := filepath.Join(dir, "instance.json")
	info, err = os.Lstat(path)
	if err != nil {
		return Instance{}, err
	}
	if !info.Mode().IsRegular() {
		return Instance{}, fmt.Errorf("instance metadata is not a regular file: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Instance{}, err
	}
	var in Instance
	if err := json.Unmarshal(data, &in); err != nil {
		return Instance{}, err
	}
	if in.ID != id || in.Kind != "managed" || in.SchemaVersion != 1 || !validName(in.Name) || !validVersion(in.MinecraftVersion) || !validLoader(in.Loader, in.LoaderVersion) || in.MemoryMB <= 0 || (in.JavaPath != "" && !filepath.IsAbs(in.JavaPath)) {
		return Instance{}, fmt.Errorf("invalid or unsupported instance metadata: %s", path)
	}
	return in, nil
}

func Create(root, name, version, loader, loaderVersion string, memoryMB int) (Instance, error) {
	if !filepath.IsAbs(root) || !validName(name) || !validVersion(version) || !validLoader(loader, loaderVersion) || memoryMB <= 0 {
		return Instance{}, fmt.Errorf("invalid instance settings")
	}
	var randomID [16]byte
	if _, err := rand.Read(randomID[:]); err != nil {
		return Instance{}, err
	}
	in := Instance{SchemaVersion: 1, ID: hex.EncodeToString(randomID[:]), Name: strings.TrimSpace(name), Kind: "managed", MinecraftVersion: version, Loader: loader, LoaderVersion: loaderVersion, MemoryMB: memoryMB}
	dir := filepath.Join(root, "instances", in.ID)
	if err := os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
		return Instance{}, err
	}
	if err := checkInstancesDir(root); err != nil {
		return Instance{}, err
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		return Instance{}, err
	}
	if err := save(dir, in); err != nil {
		return Instance{}, fmt.Errorf("creating instance %s (partial directory at %s): %w", in.ID, dir, err)
	}
	return in, nil
}

func Rename(root, id, name string) error {
	if id == LegacyID || !validName(name) {
		return fmt.Errorf("cannot rename this instance")
	}
	in, err := Read(root, id)
	if err != nil {
		return err
	}
	in.Name = strings.TrimSpace(name)
	return save(filepath.Join(root, "instances", id), in)
}

func UpdateSettings(root string, in Instance) error {
	if in.Kind != "managed" || in.ID == LegacyID || !validID(in.ID) || in.SchemaVersion != 1 || !validName(in.Name) || !validVersion(in.MinecraftVersion) || !validLoader(in.Loader, in.LoaderVersion) || in.MemoryMB <= 0 || (in.JavaPath != "" && !filepath.IsAbs(in.JavaPath)) {
		return fmt.Errorf("invalid instance settings")
	}
	current, err := Read(root, in.ID)
	if err != nil {
		return err
	}
	if in.MinecraftVersion != current.MinecraftVersion || in.Loader != current.Loader || in.LoaderVersion != current.LoaderVersion {
		return fmt.Errorf("Minecraft version and loader are fixed for managed instances")
	}
	return save(filepath.Join(root, "instances", in.ID), in)
}

// Selected defaults to the existing installation until the user selects a
// managed one. A dangling selection is an error rather than a silent switch.
func Selected(root string) (Instance, error) {
	if !filepath.IsAbs(root) {
		return Instance{}, fmt.Errorf("data root must be absolute")
	}
	path := filepath.Join(root, "selection.json")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return Read(root, LegacyID)
	}
	if err != nil {
		return Instance{}, err
	}
	if !info.Mode().IsRegular() {
		return Instance{}, fmt.Errorf("selection is not a regular file: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Instance{}, err
	}
	var state struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return Instance{}, err
	}
	return Read(root, state.ID)
}

func Select(root, id string) error {
	if !filepath.IsAbs(root) {
		return fmt.Errorf("data root must be absolute")
	}
	if _, err := Read(root, id); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	return writeJSON(root, "selection.json", struct {
		ID string `json:"id"`
	}{id})
}

func save(dir string, in Instance) error {
	return writeJSON(dir, "instance.json", in)
}

func writeJSON(dir, name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".instance-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// ponytail: Rename may not be atomic on Windows; add a recovery journal if interrupted writes prove problematic.
	return os.Rename(f.Name(), filepath.Join(dir, name))
}

func checkManagedDirectory(path string, missingOK bool) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) && missingOK {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("managed path is not a regular directory: %s", path)
	}
	return nil
}

func checkInstancesDir(root string) error {
	path := filepath.Join(root, "instances")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("instances directory is not a directory: %s", path)
	}
	return nil
}

func validID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil && id == strings.ToLower(id)
}

func validName(name string) bool {
	if strings.TrimSpace(name) == "" || len(name) > 80 {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func validVersion(version string) bool {
	if version == "" || version == "." || version == ".." || len(version) > 100 {
		return false
	}
	for _, r := range version {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func validLoader(loader, version string) bool {
	return loader == "Vanilla" && version == "" || loader == "Fabric" && validVersion(version)
}
