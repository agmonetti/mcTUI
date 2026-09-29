package worlds

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"mcTUI/internal/instances"
)

// Copy duplicates a saved world into another instance without changing the
// source. It returns the destination folder name, which may differ from folder
// if a world with that name already exists. Minecraft versions are not checked.
func Copy(root string, source, destination instances.Instance, folder string) (string, error) {
	if folder == "" || folder == "." || folder == ".." || filepath.Base(folder) != folder || strings.ContainsAny(folder, "/\\\x00") || filepath.VolumeName(folder) != "" {
		return "", fmt.Errorf("invalid world folder %q", folder)
	}
	if source.ID == destination.ID && source.Kind == destination.Kind {
		return "", fmt.Errorf("source and destination instances must differ")
	}
	sourceGame, err := instances.GameDir(root, source)
	if err != nil {
		return "", err
	}
	destinationGame, err := instances.GameDir(root, destination)
	if err != nil {
		return "", err
	}
	if filepath.Clean(sourceGame) == filepath.Clean(destinationGame) {
		return "", fmt.Errorf("source and destination game directories must differ")
	}
	sourceSaves := filepath.Join(sourceGame, "saves")
	if err := regularDirectory(sourceGame); err != nil {
		return "", err
	}
	if err := regularDirectory(sourceSaves); err != nil {
		return "", err
	}
	sourceWorld := filepath.Join(sourceSaves, folder)
	if err := regularDirectory(sourceWorld); err != nil {
		return "", err
	}
	if err := ensureDirectory(destinationGame); err != nil {
		return "", err
	}
	destinationSaves := filepath.Join(destinationGame, "saves")
	if err := ensureDirectory(destinationSaves); err != nil {
		return "", err
	}
	// Stage next to saves, rather than inside it: no incomplete save can be
	// discovered by Minecraft or List, and the final rename stays on one FS.
	stage, err := os.MkdirTemp(destinationGame, ".mctui-world-copy-")
	if err != nil {
		return "", err
	}
	defer func() {
		if stage != "" {
			os.RemoveAll(stage)
		}
	}()
	if err := copyWorldTree(sourceWorld, stage); err != nil {
		return "", err
	}
	for n := 0; ; n++ {
		name := copiedWorldName(folder, n)
		err := publishWorld(stage, filepath.Join(destinationSaves, name))
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		stage = ""
		return name, nil
	}
}

func regularDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("not a regular directory: %s", path)
	}
	return nil
}

func ensureDirectory(path string) error {
	if err := os.Mkdir(path, 0700); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	return regularDirectory(path)
}

func copiedWorldName(folder string, n int) string {
	if n == 0 {
		return folder
	}
	suffix := " (copy)"
	if n > 1 {
		suffix = fmt.Sprintf(" (copy %d)", n)
	}
	// A source folder may already use all 255 bytes of a filesystem name.
	base := folder
	if len(base)+len(suffix) > 255 {
		base = base[:255-len(suffix)]
		for !utf8.ValidString(base) {
			base = base[:len(base)-1]
		}
	}
	return base + suffix
}

func copyWorldTree(source, stage string) error {
	sourceInfo, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !sourceInfo.IsDir() || sourceInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("not a regular directory: %s", source)
	}
	root, err := os.OpenRoot(source)
	if err != nil {
		return err
	}
	defer root.Close()
	openedInfo, err := root.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(sourceInfo, openedInfo) {
		return fmt.Errorf("world directory changed while copying: %s", source)
	}
	return fs.WalkDir(root.FS(), ".", func(rel string, _ fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := root.Lstat(rel)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic link in world: %s", filepath.Join(source, rel))
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(stage, filepath.FromSlash(rel))
		if info.IsDir() {
			return os.Mkdir(target, 0700)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported file in world: %s", filepath.Join(source, rel))
		}
		file, err := root.Open(rel)
		if err != nil {
			return err
		}
		opened, err := file.Stat()
		if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
			file.Close()
			return fmt.Errorf("world file changed while copying: %s", filepath.Join(source, rel))
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			file.Close()
			return err
		}
		_, copyErr := io.Copy(out, file)
		closeErr := out.Close()
		inputCloseErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if inputCloseErr != nil {
			return inputCloseErr
		}
		return os.Chmod(target, info.Mode().Perm())
	})
}
