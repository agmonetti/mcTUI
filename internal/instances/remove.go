package instances

import (
	"fmt"
	"os"
	"path/filepath"

	"mcTUI/internal/operationlock"
)

// RetiredList returns managed instances removed from the picker but still recoverable.
func RetiredList(root string) ([]Instance, error) {
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("invalid data root")
	}
	trash := filepath.Join(root, "retired")
	if err := checkManagedDirectory(trash, true); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(trash)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result := make([]Instance, 0, len(entries))
	for _, entry := range entries {
		in, err := readManagedAt(filepath.Join(trash, entry.Name()), entry.Name())
		if err != nil {
			return nil, err
		}
		result = append(result, in)
	}
	return result, nil
}

// Retire moves a managed instance out of the picker without deleting data or backups.
func Retire(root, id string) error {
	if !filepath.IsAbs(root) || !validID(id) {
		return fmt.Errorf("invalid instance ID or data root")
	}
	in, err := Read(root, id)
	if err != nil {
		return err
	}
	gameDir, err := GameDir(root, in)
	if err != nil {
		return err
	}
	if err := checkManagedDirectory(root, false); err != nil {
		return err
	}
	releaseTrash, err := operationlock.Acquire(root)
	if err != nil {
		return err
	}
	defer releaseTrash()
	trash := filepath.Join(root, "retired")
	if err := checkManagedDirectory(trash, true); err != nil {
		return err
	}
	if err := os.MkdirAll(trash, 0700); err != nil {
		return err
	}
	if err := checkManagedDirectory(trash, false); err != nil {
		return err
	}
	destination := filepath.Join(trash, id)
	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf("retired instance already exists: %s", destination)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := checkManagedDirectory(gameDir, true); err != nil {
		return err
	}
	if _, err := os.Lstat(gameDir); err == nil {
		release, err := operationlock.Acquire(gameDir)
		if err != nil {
			return err
		}
		defer release()
	} else if !os.IsNotExist(err) {
		return err
	}
	selected, err := Selected(root)
	if err != nil {
		return err
	}
	if selected.ID == id {
		if err := Select(root, LegacyID); err != nil {
			return err
		}
	}
	return os.Rename(filepath.Join(root, "instances", id), destination)
}

// RestoreRetired moves a retired instance back into the picker.
func RestoreRetired(root, id string) error {
	if !filepath.IsAbs(root) || !validID(id) {
		return fmt.Errorf("invalid instance ID or data root")
	}
	if err := checkManagedDirectory(root, false); err != nil {
		return err
	}
	release, err := operationlock.Acquire(root)
	if err != nil {
		return err
	}
	defer release()
	trash := filepath.Join(root, "retired")
	if err := checkManagedDirectory(trash, false); err != nil {
		return err
	}
	source := filepath.Join(trash, id)
	if _, err := readManagedAt(source, id); err != nil {
		return err
	}
	if err := checkInstancesDir(root); err != nil {
		return err
	}
	destination := filepath.Join(root, "instances", id)
	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf("instance already exists: %s", destination)
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(source, destination)
}

// PurgeRetired permanently deletes a retired instance and its Reset history.
// Callers must first confirm both paths with the user.
func PurgeRetired(root, id string) error {
	if !filepath.IsAbs(root) || !validID(id) {
		return fmt.Errorf("invalid instance ID or data root")
	}
	if err := checkManagedDirectory(root, false); err != nil {
		return err
	}
	release, err := operationlock.Acquire(root)
	if err != nil {
		return err
	}
	defer release()
	trash := filepath.Join(root, "retired")
	if err := checkManagedDirectory(trash, false); err != nil {
		return err
	}
	dir := filepath.Join(trash, id)
	if _, err := readManagedAt(dir, id); err != nil {
		return err
	}
	gameDir := filepath.Join(dir, "minecraft")
	if err := checkManagedDirectory(gameDir, true); err != nil {
		return err
	}
	if _, err := os.Lstat(gameDir); err == nil {
		gameRelease, err := operationlock.Acquire(gameDir)
		if err != nil {
			return err
		}
		// Windows cannot remove a directory containing an open lock file.
		if err := gameRelease(); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	backup := filepath.Join(root, "backups", id)
	if err := checkManagedDirectory(filepath.Dir(backup), true); err != nil {
		return err
	}
	if err := checkManagedDirectory(backup, true); err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("could not remove retired instance %s: %w", dir, err)
	}
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("instance removed but backups remain at %s: %w", backup, err)
	}
	return nil
}

func readManagedAt(dir, id string) (Instance, error) {
	if !validID(id) {
		return Instance{}, fmt.Errorf("invalid retired instance ID: %q", id)
	}
	return readMetadata(dir, id)
}
