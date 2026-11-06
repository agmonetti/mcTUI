// Package launcher orchestrates everything needed to start Minecraft:
// resolving version metadata (package mojang), downloading missing
// files, resolving Fabric if requested (package fabric), picking a
// compatible Java binary (package java), and finally exec'ing the game.
package launcher

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"

	"mcTUI/internal/java"
	"mcTUI/internal/mojang"
	"mcTUI/internal/operationlock"
)

// maxConcurrentDownloads caps simultaneous downloads for libraries and
// Fabric dependencies (assets use their own, larger limit below) to avoid
// overwhelming the connection with hundreds of simultaneous requests.
const maxConcurrentDownloads = 20

// ResolvePreparedJava applies the shared selection and compatibility rules
// after preparation has loaded the selected Minecraft version's metadata.
func ResolvePreparedJava(runtimeDir string, prepared Prepared, overridePath string) (string, error) {
	candidate, err := java.ResolveBinary(prepared.javaMajor, runtimeDir, overridePath)
	if err != nil {
		return "", err
	}
	return candidate.Path, nil
}

// validateAssets verifies the index against Mojang's published hash, including
// cached copies, before trusting its list of asset objects.
func validateAssets(mcDir string, data mojang.VersionData, wg *sync.WaitGroup, download func(url, destination, expectedSHA1 string) error, onAssets func(int)) (string, error) {
	hash, err := hex.DecodeString(data.AssetIndex.SHA1)
	if err != nil || len(hash) != 20 {
		return "", fmt.Errorf("missing or invalid asset index SHA-1")
	}
	indexPath := filepath.Join(mcDir, "assets", "indexes", data.AssetIndex.ID+".json")
	wg.Add(1)
	if err := download(data.AssetIndex.URL, indexPath, data.AssetIndex.SHA1); err != nil {
		return "", fmt.Errorf("asset index: %w", err)
	}
	if err := downloadAssetObjects(mcDir, indexPath, wg, func(url, dest, hash string) { _ = download(url, dest, hash) }, onAssets); err != nil {
		return "", fmt.Errorf("invalid asset index: %w", err)
	}
	return data.AssetIndex.ID, nil
}

// downloadAssetObjects reads an asset index JSON and kicks off downloads
// for every object listed, capped at 50 concurrent downloads.
func downloadAssetObjects(mcDir, indexPath string, wg *sync.WaitGroup, download func(url, destination, expectedSHA1 string), onAssets func(int)) error {
	indexBytes, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}
	var assetIndexData struct {
		Objects map[string]struct {
			Hash string `json:"hash"`
		} `json:"objects"`
	}
	if err := json.Unmarshal(indexBytes, &assetIndexData); err != nil {
		return err
	}
	if assetIndexData.Objects == nil {
		return fmt.Errorf("missing objects")
	}
	for _, obj := range assetIndexData.Objects {
		if len(obj.Hash) != 40 {
			return fmt.Errorf("invalid asset hash %q", obj.Hash)
		}
		if _, err := hex.DecodeString(obj.Hash); err != nil {
			return fmt.Errorf("invalid asset hash %q: %w", obj.Hash, err)
		}
	}
	if onAssets != nil {
		onAssets(len(assetIndexData.Objects))
	}

	const maxConcurrentAssets = 50
	assetSem := make(chan struct{}, maxConcurrentAssets)
	for _, obj := range assetIndexData.Objects {
		hash := obj.Hash
		subDir := hash[:2]
		url := "https://resources.download.minecraft.net/" + subDir + "/" + hash
		dest := filepath.Join(mcDir, "assets", "objects", subDir, hash)

		wg.Add(1)
		assetSem <- struct{}{}
		go func(u, d, h string) {
			defer func() { <-assetSem }()
			download(u, d, h)
		}(url, dest, obj.Hash)
	}
	return nil
}

// Game owns Java, its log and the instance lock until Java exits, even if
// the caller never calls Wait. Wait may be called more than once.
type Game struct {
	cmd     *exec.Cmd
	done    chan struct{}
	code    int
	err     error
	LogPath string
}

// AuthSession holds the identity credentials for launching the game.
type AuthSession struct {
	Username    string
	UUID        string
	AccessToken string
	UserType    string
}

// StartPrepared launches artifacts already verified by PrepareContext. It does
// not prepare again, and refuses to start when this game directory is busy.
func StartPrepared(gameDir, runtimeDir, javaBinary, username, targetVersion string, memoryMB int, prepared Prepared) (*Game, error) {
	return StartPreparedWithAuth(gameDir, runtimeDir, javaBinary, targetVersion, memoryMB, AuthSession{
		Username:    username,
		UUID:        uuid.New().String(),
		AccessToken: "0",
		UserType:    "legacy",
	}, prepared)
}

// StartPreparedWithAuth launches artifacts with specific authentication credentials (MSA or Offline).
func StartPreparedWithAuth(gameDir, runtimeDir, javaBinary, targetVersion string, memoryMB int, auth AuthSession, prepared Prepared) (*Game, error) {
	if !filepath.IsAbs(gameDir) || !filepath.IsAbs(runtimeDir) || javaBinary == "" || memoryMB <= 0 || prepared.mainClass == "" || prepared.classpath == "" || prepared.assetIndexID == "" {
		return nil, fmt.Errorf("incomplete launch configuration")
	}
	if err := os.MkdirAll(gameDir, 0755); err != nil {
		return nil, err
	}
	info, err := os.Lstat(gameDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("unsafe game directory %s: %v", gameDir, err)
	}
	release, err := operationlock.Acquire(gameDir)
	if err != nil {
		return nil, err
	}
	logPath := filepath.Join(gameDir, "mctui_latest.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		_ = release()
		return nil, err
	}

	u := auth.Username
	if u == "" {
		u = "Player"
	}
	uID := auth.UUID
	if uID == "" {
		uID = uuid.New().String()
	}
	token := auth.AccessToken
	if token == "" {
		token = "0"
	}
	userType := auth.UserType
	if userType == "" {
		userType = "legacy"
	}

	args := []string{fmt.Sprintf("-Xmx%dM", memoryMB)}
	if runtime.GOOS == "darwin" {
		args = append(args, "-XstartOnFirstThread")
	}
	if prepared.nativesDir != "" {
		args = append(args, "-Djava.library.path="+prepared.nativesDir)
	}
	args = append(args,
		"-cp", prepared.classpath, prepared.mainClass,
		"--username", u,
		"--version", targetVersion,
		"--gameDir", gameDir,
		"--assetsDir", filepath.Join(runtimeDir, "assets"),
		"--assetIndex", prepared.assetIndexID,
		"--uuid", uID,
		"--accessToken", token,
		"--userType", userType,
		"--versionType", "release",
	)
	cmd := exec.Command(javaBinary, args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd.Dir, cmd.Stdout, cmd.Stderr = gameDir, logFile, logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		_ = release()
		return nil, err
	}
	game := &Game{cmd: cmd, LogPath: logPath, done: make(chan struct{})}
	go func() {
		err := cmd.Wait()
		game.code = cmd.ProcessState.ExitCode()
		game.err = errors.Join(err, logFile.Close(), release())
		close(game.done)
	}()
	return game, nil
}

// Wait returns the final exit code and any process/cleanup errors. The lock is
// released by the supervisor only after cmd.Wait has reaped Java.
func (game *Game) Wait() (int, error) {
	<-game.done
	return game.code, game.err
}

// Kill requests immediate termination of Java; callers must still Wait for its
// final result. A failed kill does not release the lock while Java is alive.
func (game *Game) Kill() error {
	select {
	case <-game.done:
		return nil
	default:
	}
	err := game.cmd.Process.Kill()
	if errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}

// deduplicateClasspath removes duplicate libraries from the classpath,
// keeping only the newest version of each artifact. This prevents errors
// like "duplicate ASM classes found" when both Vanilla and Fabric include
// the same library at different versions.
func deduplicateClasspath(entries []string) []string {
	type libInfo struct {
		path    string
		version string
	}
	// key = "group/artifact", value = best candidate
	libs := make(map[string]libInfo)

	for _, entry := range entries {
		// Extract group/artifact from the full path
		// Find "libraries/" prefix and take everything after it up to the version dir
		normalized := filepath.ToSlash(entry)
		idx := strings.Index(normalized, "/libraries/")
		if idx == -1 {
			// Not a standard library path, keep it
			libs[entry] = libInfo{path: entry, version: ""}
			continue
		}
		afterLibraries := normalized[idx+len("/libraries/"):]
		parts := strings.Split(afterLibraries, "/")
		// parts: [org, ow2, asm, asm, 9.6, asm-9.6.jar]
		// key = everything except the last 2 parts (version and filename)
		if len(parts) < 3 {
			libs[entry] = libInfo{path: entry, version: ""}
			continue
		}
		key := strings.Join(parts[:len(parts)-2], "/")
		ver := parts[len(parts)-2]

		// To prevent throwing away natives or classifiers that share the same base artifact and version
		// (e.g. lwjgl-3.4.1-unsafe.jar vs lwjgl-3.4.1-natives-windows.jar), we must include the
		// classifier in the deduplication key.
		baseArtifact := parts[len(parts)-3]
		expectedPrefix := baseArtifact + "-" + ver
		fileName := parts[len(parts)-1]

		classifier := ""
		if strings.HasPrefix(fileName, expectedPrefix+"-") {
			classifier = strings.TrimPrefix(fileName, expectedPrefix+"-")
			classifier = strings.TrimSuffix(classifier, ".jar")
		} else if fileName != expectedPrefix+".jar" {
			classifier = fileName
		}

		fullKey := key
		if classifier != "" {
			fullKey = key + ":" + classifier
		}

		if existing, ok := libs[fullKey]; ok {
			if versionGreaterThan(ver, existing.version) {
				libs[fullKey] = libInfo{path: entry, version: ver}
			}
		} else {
			libs[fullKey] = libInfo{path: entry, version: ver}
		}
	}

	result := make([]string, 0, len(libs))
	for _, info := range libs {
		result = append(result, info.path)
	}
	return result
}

// versionGreaterThan returns true if a is a higher version than b.
// Versions are split by "." and each segment is compared numerically.
// If any segment is not a valid integer, it falls back to string comparison.
func versionGreaterThan(a, b string) bool {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	max := len(as)
	if len(bs) > max {
		max = len(bs)
	}
	for i := 0; i < max; i++ {
		var an, bn int
		var aOK, bOK bool
		if i < len(as) {
			var err error
			an, err = strconv.Atoi(as[i])
			aOK = err == nil
		} else {
			aOK = true
		}
		if i < len(bs) {
			var err error
			bn, err = strconv.Atoi(bs[i])
			bOK = err == nil
		} else {
			bOK = true
		}
		if !aOK || !bOK {
			return a > b
		}
		if an != bn {
			return an > bn
		}
	}
	return false
}
