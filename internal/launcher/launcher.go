// Package launcher orchestrates everything needed to start Minecraft:
// resolving version metadata (package mojang), downloading missing
// files, resolving Fabric if requested (package fabric), picking a
// compatible Java binary (package java), and finally exec'ing the game.
package launcher

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"mcTUI/internal/fabric"
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
func Launch(mcDir, username, targetVersion, modloader string, memoryMB int) {
	fmt.Print("\033[H\033[2J")
	fmt.Println(strings.Repeat("=", 75))
	fmt.Println(" MODE: Offline / LAN")
	fmt.Println(" (The server must have 'online-mode=false' in server.properties)")
	fmt.Println(strings.Repeat("=", 75))
	fmt.Println()

	fmt.Printf("Starting engine for player: %s (Version: %s - Modloader: %s)\n", username, targetVersion, modloader)
	fmt.Println("1. Verifying Vanilla Engine and Libraries...")

	clientPath := mojang.ClientJarPath(mcDir, targetVersion)
	haveClient := mojang.ClientJarExists(mcDir, targetVersion)

	data, haveVersionData := mojang.FetchVersionData(targetVersion, mcDir)
	if !haveVersionData {
		if cached, ok := mojang.CachedVersionData(mcDir, targetVersion); ok {
			data = cached
			haveVersionData = true
			fmt.Println("   Using cached version metadata.")
		}
	}

	if !haveVersionData && !haveClient {
		fmt.Println("\n[!] Cannot launch: no internet connection and no local")
		fmt.Println("    installation found for version", targetVersion+".")
		fmt.Println("    Connect to the internet at least once to download it.")
		return
	}

	var classpathEntries []string
	librariesPath := filepath.Join(mcDir, "libraries")
	var wg sync.WaitGroup
	download := mojang.Download(&wg)

	// Download client.jar if missing and we have a URL for it.
	if !haveClient {
		if !haveVersionData || data.Downloads.Client.URL == "" {
			fmt.Println("\n[!] Cannot download client.jar: no version data available.")
			return
		}
		wg.Add(1)
		download(data.Downloads.Client.URL, clientPath, data.Downloads.Client.SHA1)
		wg.Wait() // wait specifically for the client jar before proceeding
	}

	if haveVersionData {
		libSem := make(chan struct{}, maxConcurrentDownloads)
		for _, lib := range data.Libraries {
			// Check OS rules before including this library
			if !mojang.ShouldIncludeLibrary(lib.Rules) {
				continue
			}
			url := lib.Downloads.Artifact.URL
			path := lib.Downloads.Artifact.Path
			if url != "" && path != "" {
				fullPath := filepath.Join(librariesPath, path)
				classpathEntries = append(classpathEntries, fullPath)
				wg.Add(1)
				libSem <- struct{}{}
				go func(u, d, sha1 string) {
					defer func() { <-libSem }()
					download(u, d, sha1)
				}(url, fullPath, lib.Downloads.Artifact.SHA1)
			}
		}
	} else {
		fmt.Println("   No version metadata available — skipping library check (offline mode).")
	}

	mainClass := "net.minecraft.client.main.Main"

	if modloader == "Fabric" {
		fmt.Println("-> Fabric injection detected. Resolving profile...")

		profile, ok := fabric.Resolve(targetVersion, mcDir, librariesPath, &wg, download, maxConcurrentDownloads)
		if ok {
			mainClass = profile.MainClass
			classpathEntries = append(classpathEntries, profile.ClasspathEntries...)
		} else {
			fmt.Println("[!] Fabric profile unavailable (offline and not cached, or no")
			fmt.Println("    compatible loader for this version). Falling back to Vanilla.")
		}
	}

	fmt.Println("2. Validating Assets...")
	assetIndexID, haveAssetIndex := validateAssets(mcDir, data, haveVersionData, &wg, download)

	// Download and extract native libraries (LWJGL, OpenAL, etc.)
	var nativesDir string
	if haveVersionData {
		nativesDir = mojang.DownloadAndExtractNatives(data, mcDir, targetVersion, &wg, download)
	}

	wg.Wait()
	classpathEntries = append(classpathEntries, clientPath)
	classpathEntries = deduplicateClasspath(classpathEntries)
	finalClasspath := strings.Join(classpathEntries, string(filepath.ListSeparator))

	fmt.Println("3. All set! Launching Minecraft...")

	javaBinary := "java"
	if haveVersionData && data.JavaVersion.MajorVersion > 0 {
		candidate, ok := java.FindBinary(data.JavaVersion.MajorVersion, mcDir)
		if !ok {
			fmt.Println()
			fmt.Printf("[!] This version requires Java %d or newer, but no compatible\n", data.JavaVersion.MajorVersion)
			fmt.Println("[!] JRE was found (checked PATH and common install locations).")
			fmt.Println("[!] Download a compatible JRE here:")
			fmt.Println("[!]  ", java.DownloadURL(data.JavaVersion.MajorVersion))
			if runtime.GOOS != "windows" {
				fmt.Println("[!] Or via your package manager, e.g.:")
				fmt.Println("[!]   Arch:    pacman -S jdk-openjdk")
				fmt.Println("[!]   Debian:  apt install openjdk-21-jre  (or newer)")
			}
			return
		}
		javaBinary = candidate.Path
		if candidate.Path != "java" {
			fmt.Printf("   Using Java %d from %s (system default did not meet requirement)\n", candidate.Major, candidate.Path)
		}
	}

	if !haveAssetIndex {
		assetIndexID = data.AssetIndex.ID
	}

	runGame(mcDir, javaBinary, finalClasspath, mainClass, username, targetVersion, assetIndexID, memoryMB, nativesDir)
}

// validateAssets ensures the asset index is available locally (using a
// cached copy or downloading it), then kicks off downloads for any
// missing asset objects. Returns the resolved asset index ID and whether
// it was actually found/usable.
func validateAssets(mcDir string, data mojang.VersionData, haveVersionData bool, wg *sync.WaitGroup, download func(url, destination, expectedSHA1 string)) (string, bool) {
	indexPath := filepath.Join(mcDir, "assets", "indexes", data.AssetIndex.ID+".json")

	if info, err := os.Stat(indexPath); err == nil && info.Size() > 0 {
		downloadAssetObjects(mcDir, indexPath, wg, download)
		return data.AssetIndex.ID, true
	}

	if !haveVersionData || data.AssetIndex.URL == "" {
		fmt.Println("   No asset index available locally and no version data — skipping asset validation (offline mode).")
		return "", false
	}

	os.MkdirAll(filepath.Dir(indexPath), 0755)
	wg.Add(1)
	download(data.AssetIndex.URL, indexPath, "")
	wg.Wait() // need the index before we can enumerate objects

	if info, err := os.Stat(indexPath); err != nil || info.Size() == 0 {
		fmt.Println("[!] Could not download asset index (no internet?). Skipping asset validation.")
		return "", false
	}

	downloadAssetObjects(mcDir, indexPath, wg, download)
	return data.AssetIndex.ID, true
}

// downloadAssetObjects reads an asset index JSON and kicks off downloads
// for every object listed, capped at 50 concurrent downloads.
func downloadAssetObjects(mcDir, indexPath string, wg *sync.WaitGroup, download func(url, destination, expectedSHA1 string)) {
	indexBytes, err := os.ReadFile(indexPath)
	if err != nil {
		return
	}
	var assetIndexData struct {
		Objects map[string]struct {
			Hash string `json:"hash"`
		} `json:"objects"`
	}
	if json.Unmarshal(indexBytes, &assetIndexData) != nil {
		return
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
}

// runGame execs java with the assembled classpath and arguments, then
// watches for an early crash (process exits within a few seconds) and
// surfaces the JVM log if so.
func runGame(mcDir, javaBinary, classpath, mainClass, username, targetVersion, assetIndexID string, memoryMB int, nativesDir string) {
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
        "--gameDir", mcDir,
        "--assetsDir", filepath.Join(mcDir, "assets"),
        "--assetIndex", assetIndexID,
        "--uuid", sessionUUID,
        "--accessToken", "0",
        "--userType", "legacy",
        "--versionType", "release",
    )

    cmd := exec.Command(javaBinary, args...)

	logFile, err := os.Create(filepath.Join(mcDir, "mctui_latest.log"))
	if err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
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
		logFile.Close()
		fmt.Println()
		if waitErr != nil {
			fmt.Println("[!] Minecraft exited early with an error.")
		} else {
			fmt.Println("[!] Minecraft exited almost immediately.")
		}
		fmt.Println("[!] Last lines of the log:")
		fmt.Println(strings.Repeat("-", 60))
		printLogTail(filepath.Join(mcDir, "mctui_latest.log"), 25)
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

		if existing, ok := libs[key]; ok {
			// Keep the one with the higher version string (simple lexicographic works for most cases)
			if ver > existing.version {
				libs[key] = libInfo{path: entry, version: ver}
			}
		} else {
			libs[key] = libInfo{path: entry, version: ver}
		}
	}

	result := make([]string, 0, len(libs))
	for _, info := range libs {
		result = append(result, info.path)
	}
	return result
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
