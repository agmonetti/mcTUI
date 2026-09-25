// Package launcher orchestrates everything needed to start Minecraft:
// resolving version metadata (package mojang), downloading missing
// files, resolving Fabric if requested (package fabric), picking a
// compatible Java binary (package java), and finally exec'ing the game.
package launcher

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"mcTUI/internal/java"
	"mcTUI/internal/mojang"
)

// maxConcurrentDownloads caps simultaneous downloads for libraries and
// Fabric dependencies (assets use their own, larger limit below) to avoid
// overwhelming the connection with hundreds of simultaneous requests.
const maxConcurrentDownloads = 20

// Launch starts Minecraft for the given configuration. It prints progress
// and any errors to stdout, and never panics on network failures — if
// everything required is already cached locally, it can run fully
// offline.
func Launch(gameDir, runtimeDir, username, targetVersion, modloader string, memoryMB int, javaPath string) {
	fmt.Print("\033[H\033[2J")
	fmt.Println(strings.Repeat("=", 75))
	fmt.Println(" MODE: Offline / LAN")
	fmt.Println(" (The server must have 'online-mode=false' in server.properties)")
	fmt.Println(strings.Repeat("=", 75))
	fmt.Println()

	fmt.Printf("Starting engine for player: %s (Version: %s - Modloader: %s)\n", username, targetVersion, modloader)
	fmt.Println("1. Verifying Vanilla Engine and Libraries...")

	prepared, err := Prepare(runtimeDir, targetVersion, modloader, nil)
	if err != nil {
		fmt.Println("[!] Cannot prepare game:", err)
		return
	}
	javaBinary, err := ResolvePreparedJava(runtimeDir, prepared, javaPath)
	if err != nil {
		fmt.Println("[!] Cannot select Java:", err)
		return
	}
	runGame(gameDir, runtimeDir, javaBinary, prepared.classpath, prepared.mainClass, username, targetVersion, prepared.assetIndexID, memoryMB, prepared.nativesDir)
}

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
func validateAssets(mcDir string, data mojang.VersionData, wg *sync.WaitGroup, download func(url, destination, expectedSHA1 string) error) (string, error) {
	hash, err := hex.DecodeString(data.AssetIndex.SHA1)
	if err != nil || len(hash) != 20 {
		return "", fmt.Errorf("missing or invalid asset index SHA-1")
	}
	indexPath := filepath.Join(mcDir, "assets", "indexes", data.AssetIndex.ID+".json")
	wg.Add(1)
	if err := download(data.AssetIndex.URL, indexPath, data.AssetIndex.SHA1); err != nil {
		return "", fmt.Errorf("asset index: %w", err)
	}
	if err := downloadAssetObjects(mcDir, indexPath, wg, func(url, dest, hash string) { _ = download(url, dest, hash) }); err != nil {
		return "", fmt.Errorf("invalid asset index: %w", err)
	}
	return data.AssetIndex.ID, nil
}

// downloadAssetObjects reads an asset index JSON and kicks off downloads
// for every object listed, capped at 50 concurrent downloads.
func downloadAssetObjects(mcDir, indexPath string, wg *sync.WaitGroup, download func(url, destination, expectedSHA1 string)) error {
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

// runGame execs java with the assembled classpath and arguments, then
// watches for an early crash (process exits within a few seconds) and
// surfaces the JVM log if so.
func runGame(gameDir, runtimeDir, javaBinary, classpath, mainClass, username, targetVersion, assetIndexID string, memoryMB int, nativesDir string) {
	sessionUUID := uuid.New().String()

	args := []string{
		fmt.Sprintf("-Xmx%dM", memoryMB),
	}

	// macOS requires GLFW (and therefore Minecraft's render thread) to run
	// on the first thread of the process. The official launcher adds this
	// flag automatically on macOS; without it, the JVM crashes on init.
	if runtime.GOOS == "darwin" {
		args = append(args, "-XstartOnFirstThread")
	}

	// Point the JVM to the extracted native libraries (.so, .dll, .dylib)
	if nativesDir != "" {
		args = append(args, "-Djava.library.path="+nativesDir)
	}

	args = append(args,
		"-cp", classpath,
		mainClass,
		"--username", username,
		"--version", targetVersion,
		"--gameDir", gameDir,
		"--assetsDir", filepath.Join(runtimeDir, "assets"),
		"--assetIndex", assetIndexID,
		"--uuid", sessionUUID,
		"--accessToken", "0",
		"--userType", "legacy",
		"--versionType", "release",
	)

	// javaBinary is either "java" from PATH or a locally probed JRE; exec.Command does not invoke a shell.
	cmd := exec.Command(javaBinary, args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd.Dir = gameDir

	if err := os.MkdirAll(gameDir, 0755); err != nil {
		fmt.Println("[!] Cannot create game directory:", err)
		return
	}
	logFile, err := os.Create(filepath.Join(gameDir, "mctui_latest.log"))
	if err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	if logFile != nil {
		defer logFile.Close()
	}
	if err := cmd.Start(); err != nil {
		fmt.Println("\n[!] Error starting process:", err)
		return
	}

	fmt.Println("   Minecraft is starting in the background...")

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case waitErr := <-done:
		fmt.Println()
		if waitErr != nil {
			fmt.Println("[!] Minecraft exited early with an error.")
		} else {
			fmt.Println("[!] Minecraft exited almost immediately.")
		}
		fmt.Println("[!] Last lines of the log:")
		fmt.Println(strings.Repeat("-", 60))
		printLogTail(filepath.Join(gameDir, "mctui_latest.log"), 25)
		fmt.Println(strings.Repeat("-", 60))

	case <-time.After(8 * time.Second):
		fmt.Println()
		fmt.Println("   Minecraft is running.")
		fmt.Println("   You can close this window — the game will keep running")
		fmt.Println("   in the background as a separate process.")
	}
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

// printLogTail prints the last n lines of the file at path.
func printLogTail(path string, n int) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("(could not read log file:", err, ")")
		return
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	start := len(lines) - n
	if start < 0 {
		start = 0
	}
	for _, line := range lines[start:] {
		fmt.Println(line)
	}
}
