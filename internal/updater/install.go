package updater

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// TargetAssetName returns the release binary name expected for the given OS and architecture.
func TargetAssetName(goos, goarch string) string {
	ext := ""
	if goos == "windows" {
		ext = ".exe"
	}
	return fmt.Sprintf("mctui-%s-%s%s", goos, goarch, ext)
}

// ParseChecksums parses a standard sha256sum file into a map of filename -> sha256 hex.
func ParseChecksums(content string) map[string]string {
	result := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Format: "<hash>  <filename>" or "<hash> *<filename>"
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			hash := strings.ToLower(strings.TrimSpace(parts[0]))
			filename := strings.TrimPrefix(parts[1], "*")
			result[filename] = hash
		}
	}
	return result
}

// VerifySHA256 computes the SHA-256 digest of filePath and compares it to expectedHex.
func VerifySHA256(filePath, expectedHex string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file for hash verification: %w", err)
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return fmt.Errorf("failed to compute SHA-256 digest: %w", err)
	}

	actualHex := hex.EncodeToString(hasher.Sum(nil))
	expectedClean := strings.ToLower(strings.TrimSpace(expectedHex))
	if actualHex != expectedClean {
		return fmt.Errorf("SHA-256 mismatch: expected %s, got %s", expectedClean, actualHex)
	}

	return nil
}

// DownloadFile downloads a remote URL to destPath, updating onProgress with (written, total).
func DownloadFile(url, destPath string, onProgress func(written, total int64)) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "mcTUI-Updater")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with HTTP %d", resp.StatusCode)
	}

	total := resp.ContentLength
	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	buf := make([]byte, 32*1024)
	var written int64
	for {
		n, rErr := resp.Body.Read(buf)
		if n > 0 {
			wN, wErr := out.Write(buf[:n])
			if wErr != nil {
				return wErr
			}
			written += int64(wN)
			if onProgress != nil {
				onProgress(written, total)
			}
		}
		if rErr != nil {
			if rErr == io.EOF {
				break
			}
			return rErr
		}
	}

	return nil
}

// ApplyUpdate replaces targetPath with tempBinaryPath.
// Handles Unix atomic renames and Windows running-binary replacement via rename to .old.
func ApplyUpdate(tempBinaryPath, targetPath string) error {
	// Ensure executable permissions before replacement
	_ = os.Chmod(tempBinaryPath, 0755)

	if runtime.GOOS == "windows" {
		oldPath := targetPath + ".old"
		_ = os.Remove(oldPath) // remove previous backup if exists
		if err := os.Rename(targetPath, oldPath); err != nil {
			return fmt.Errorf("failed to move current executable to backup: %w", err)
		}
		if err := os.Rename(tempBinaryPath, targetPath); err != nil {
			// Rollback if possible
			_ = os.Rename(oldPath, targetPath)
			return fmt.Errorf("failed to place new executable: %w", err)
		}
		return nil
	}

	// On Linux / macOS, renaming within the same filesystem is atomic and allowed even if binary is running.
	if err := os.Rename(tempBinaryPath, targetPath); err != nil {
		return fmt.Errorf("failed to replace executable: %w", err)
	}

	return nil
}

// ProgressCallback reports the current update stage and an overall progress percent (0.0 to 1.0).
type ProgressCallback func(stage string, percent float64)

// InstallRelease downloads the appropriate binary for the host OS/arch from rel,
// verifies its SHA-256 against checksums.txt, and replaces the running executable.
func InstallRelease(rel Release, onProgress ProgressCallback) error {
	targetName := TargetAssetName(runtime.GOOS, runtime.GOARCH)

	var targetAsset *Asset
	var checksumsAsset *Asset
	for i := range rel.Assets {
		if rel.Assets[i].Name == targetName {
			targetAsset = &rel.Assets[i]
		} else if rel.Assets[i].Name == "checksums.txt" {
			checksumsAsset = &rel.Assets[i]
		}
	}

	if targetAsset == nil {
		return fmt.Errorf("no binary available in release %s for %s/%s (%s)", rel.TagName, runtime.GOOS, runtime.GOARCH, targetName)
	}
	if checksumsAsset == nil {
		return fmt.Errorf("checksums.txt not found in release %s", rel.TagName)
	}

	if onProgress != nil {
		onProgress("checksums", 0.05)
	}

	// 1. Download checksums.txt
	req, err := http.NewRequest("GET", checksumsAsset.BrowserDownloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request for checksums: %w", err)
	}
	req.Header.Set("User-Agent", "mcTUI-Updater")
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download checksums.txt")
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read checksums.txt: %w", err)
	}
	checksumMap := ParseChecksums(string(bodyBytes))
	expectedHash, ok := checksumMap[targetName]
	if !ok || expectedHash == "" {
		return fmt.Errorf("checksum for %s not found in release checksums.txt", targetName)
	}

	// 2. Identify destination binary path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to determine executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable symlinks: %w", err)
	}

	execDir := filepath.Dir(execPath)
	tempFile := filepath.Join(execDir, fmt.Sprintf(".mctui-update-%d.tmp", os.Getpid()))
	defer func() {
		_ = os.Remove(tempFile)
	}()

	// 3. Download binary asset with progress
	if onProgress != nil {
		onProgress("downloading", 0.10)
	}
	err = DownloadFile(targetAsset.BrowserDownloadURL, tempFile, func(written, total int64) {
		if onProgress != nil && total > 0 {
			fraction := float64(written) / float64(total)
			// Scale between 0.10 and 0.85
			onProgress("downloading", 0.10+(fraction*0.75))
		}
	})
	if err != nil {
		return fmt.Errorf("failed to download update binary: %w", err)
	}

	// 4. Verify SHA-256
	if onProgress != nil {
		onProgress("verifying", 0.90)
	}
	if err := VerifySHA256(tempFile, expectedHash); err != nil {
		return err
	}

	// 5. Apply update (replace current binary)
	if onProgress != nil {
		onProgress("installing", 0.95)
	}
	if err := ApplyUpdate(tempFile, execPath); err != nil {
		return err
	}

	if onProgress != nil {
		onProgress("done", 1.0)
	}

	return nil
}
