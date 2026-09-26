package instances

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"mcTUI/internal/operationlock"
)

var copyableLegacyItems = []string{"mods", "config", "saves", "resourcepacks", "shaderpacks", "options.txt"}

type CopyItem struct {
	Name   string
	Exists bool
	Bytes  int64
	Error  string
}

type CopyResult struct {
	Instance    Instance
	StagingPath string
	Copied      []string
}

// InspectCopySource lists the supported legacy items and rejects links or
// special files before they can be selected for migration.
func InspectCopySource(source string) ([]CopyItem, error) {
	if err := checkCopySource(source); err != nil {
		return nil, err
	}
	items := make([]CopyItem, 0, len(copyableLegacyItems))
	for _, name := range copyableLegacyItems {
		item := CopyItem{Name: name}
		path := filepath.Join(source, name)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			items = append(items, item)
			continue
		}
		if err != nil {
			item.Error = err.Error()
		} else if name == "options.txt" {
			item.Exists = true
			if !info.Mode().IsRegular() {
				item.Error = fmt.Sprintf("unsupported source item: %s", path)
			} else {
				item.Bytes = info.Size()
			}
		} else {
			item.Exists = true
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				item.Error = fmt.Sprintf("unsupported source item: %s", path)
			} else if size, err := copyTreeSize(path); err != nil {
				item.Error = err.Error()
			} else {
				item.Bytes = size
			}
		}
		items = append(items, item)
	}
	return items, nil
}

// CopyFromLegacy copies selected items into a new Vanilla instance.
func CopyFromLegacy(root, source, name, version string, selected []string, memoryMB int) (CopyResult, error) {
	return copyInstance(root, source, Instance{Name: name, MinecraftVersion: version, Loader: "Vanilla", MemoryMB: memoryMB}, selected)
}

// CopyInstance duplicates selected mutable data from a managed instance.
// Version, loader, Java and memory are inherited; the new ID is independent.
func CopyInstance(root, sourceID, name string, selected []string) (CopyResult, error) {
	if sourceID == LegacyID {
		return CopyResult{}, fmt.Errorf("use CopyFromLegacy for the existing installation")
	}
	in, err := Read(root, sourceID)
	if err != nil {
		return CopyResult{}, err
	}
	source, err := GameDir(root, in)
	if err != nil {
		return CopyResult{}, err
	}
	in.Name = name
	return copyInstance(root, source, in, selected)
}

// copyInstance publishes metadata and data only after the copy completes.
func copyInstance(root, source string, template Instance, selected []string) (result CopyResult, returnErr error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || !validName(template.Name) || !validVersion(template.MinecraftVersion) || !validLoader(template.Loader, template.LoaderVersion) || template.MemoryMB <= 0 || len(selected) == 0 {
		return CopyResult{}, fmt.Errorf("invalid migration settings")
	}
	seen := make(map[string]bool, len(selected))
	for _, item := range selected {
		if !slices.Contains(copyableLegacyItems, item) || seen[item] {
			return CopyResult{}, fmt.Errorf("invalid or duplicate source item: %q", item)
		}
		seen[item] = true
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return CopyResult{}, err
	}
	if err := checkManagedDirectory(root, false); err != nil {
		return CopyResult{}, err
	}
	if err := checkCopySource(source); err != nil {
		return CopyResult{}, err
	}
	release, err := operationlock.Acquire(source)
	if err != nil {
		return CopyResult{}, err
	}
	defer func() {
		if releaseErr := release(); returnErr == nil {
			returnErr = releaseErr
		}
	}()
	sourceRoot, err := os.OpenRoot(source)
	if err != nil {
		return CopyResult{}, err
	}
	defer func() {
		if closeErr := sourceRoot.Close(); returnErr == nil {
			returnErr = closeErr
		}
	}()

	items, err := InspectCopySource(source)
	if err != nil {
		return CopyResult{}, err
	}
	for _, name := range selected {
		for _, item := range items {
			if item.Name != name {
				continue
			}
			if !item.Exists {
				return CopyResult{}, fmt.Errorf("selected source item is missing: %s", name)
			}
			if item.Error != "" {
				return CopyResult{}, fmt.Errorf("selected source item cannot be copied: %s: %s", name, item.Error)
			}
		}
	}

	if err := checkInstancesDir(root); err != nil {
		return CopyResult{}, err
	}
	instancesDir := filepath.Join(root, "instances")
	if err := os.MkdirAll(instancesDir, 0700); err != nil {
		return CopyResult{}, err
	}
	if err := checkInstancesDir(root); err != nil {
		return CopyResult{}, err
	}

	var randomID [16]byte
	if _, err := rand.Read(randomID[:]); err != nil {
		return CopyResult{}, err
	}
	in := template
	in.SchemaVersion, in.ID, in.Kind, in.Name = 1, hex.EncodeToString(randomID[:]), "managed", strings.TrimSpace(template.Name)
	stage, err := os.MkdirTemp(root, ".mctui-copy-*")
	if err != nil {
		return CopyResult{}, err
	}
	result = CopyResult{StagingPath: stage}
	fail := func(err error) (CopyResult, error) {
		return result, fmt.Errorf("copy incomplete; partial data retained at %s: %w", stage, err)
	}
	gameDir := filepath.Join(stage, "minecraft")
	if err := os.Mkdir(gameDir, 0700); err != nil {
		return fail(err)
	}
	for _, name := range selected {
		if name == "options.txt" {
			if err := copyRegularFile(sourceRoot, name, filepath.Join(gameDir, name)); err != nil {
				return fail(err)
			}
		} else {
			dest := filepath.Join(gameDir, name)
			if err := os.Mkdir(dest, 0700); err != nil {
				return fail(err)
			}
			subtree, err := fs.Sub(sourceRoot.FS(), name)
			if err != nil {
				return fail(err)
			}
			if err := os.CopyFS(dest, subtree); err != nil {
				return fail(err)
			}
		}
		result.Copied = append(result.Copied, name)
	}
	if err := save(stage, in); err != nil {
		return fail(err)
	}
	destination := filepath.Join(instancesDir, in.ID)
	if err := os.Rename(stage, destination); err != nil {
		return fail(err)
	}
	result.Instance, result.StagingPath = in, ""
	return result, nil
}

// DiscardCopyStage removes only a migration staging directory directly under
// root, after the caller has explicitly confirmed cleanup.
func DiscardCopyStage(root, stage string) error {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || filepath.Clean(stage) != stage || filepath.Dir(stage) != root || !strings.HasPrefix(filepath.Base(stage), ".mctui-copy-") {
		return fmt.Errorf("invalid migration staging path")
	}
	if err := checkManagedDirectory(root, false); err != nil {
		return err
	}
	info, err := os.Lstat(stage)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("migration staging path is not a regular directory")
	}
	return os.RemoveAll(stage)
}

func checkCopySource(source string) error {
	if !filepath.IsAbs(source) || filepath.Clean(source) != source {
		return fmt.Errorf("invalid migration source: %q", source)
	}
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("migration source is not a regular directory: %s", source)
	}
	return nil
}

func copyTreeSize(root string) (int64, error) {
	var size int64
	err := filepath.WalkDir(root, func(path string, _ fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic link in migration source: %s", path)
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported file in migration source: %s", path)
		}
		if info.Size() > int64(^uint64(0)>>1)-size {
			return fmt.Errorf("migration size overflow")
		}
		size += info.Size()
		return nil
	})
	return size, err
}

func copyRegularFile(source *os.Root, name, destination string) error {
	info, err := source.Lstat(name)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported source file: %s", name)
	}
	in, err := source.Open(name)
	if err != nil {
		return err
	}
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	// ponytail: preflight rejects links; Root confines race-time replacements to source, while per-file no-follow needs OS-specific code.
	_, copyErr := io.Copy(out, in)
	inputCloseErr := in.Close()
	outputCloseErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if inputCloseErr != nil {
		return inputCloseErr
	}
	return outputCloseErr
}
