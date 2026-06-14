// Package java handles detection of installed Java Runtime Environments
// and selection of a JRE that satisfies a Minecraft version's required
// major version — checking the system PATH, JREs bundled by the official
// Mojang/Microsoft launcher, and common per-OS install locations.
package java

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Candidate represents a discovered Java installation.
type Candidate struct {
	Path  string // path to the "java" binary
	Major int    // detected major version
}

// FindBinary returns a Java installation that satisfies requiredMajor
// (0 = no requirement, any java works), preferring the PATH default if it
// already qualifies. If no installed JRE satisfies the requirement, it
// returns the PATH default anyway (so the caller can fail with a clear
// message) along with ok=false.
//
// mcDir is the .minecraft directory, used to search for JREs bundled by
// the official launcher under <mcDir>/runtime/.
func FindBinary(requiredMajor int, mcDir string) (Candidate, bool) {
	pathJava := Candidate{Path: "java", Major: VersionAt("java")}

	if requiredMajor == 0 || (pathJava.Major > 0 && pathJava.Major >= requiredMajor) {
		return pathJava, true
	}

	// PATH default doesn't qualify - try Mojang's own bundled runtimes
	// first, since they're purpose-matched to specific MC versions.
	for _, candidate := range scanEmbeddedRuntimes(mcDir) {
		if candidate.Major >= requiredMajor {
			return candidate, true
		}
	}

	// Fall back to generic system-wide JRE/JDK installations.
	for _, candidate := range scanSystemInstallations() {
		if candidate.Major >= requiredMajor {
			return candidate, true
		}
	}

	return pathJava, false
}

// VersionAt runs "<binaryPath> -version" and parses the major version
// (handling both old "1.8.0_xxx" and new "21.0.x" / "25" formats).
// Returns 0 if it couldn't be determined (e.g. binary not found).
func VersionAt(binaryPath string) int {
	cmd := exec.Command(binaryPath, "-version")
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return 0
	}

	output := out.String()
	start := strings.Index(output, "\"")
	if start == -1 {
		return 0
	}
	end := strings.Index(output[start+1:], "\"")
	if end == -1 {
		return 0
	}
	versionStr := output[start+1 : start+1+end]

	parts := strings.Split(versionStr, ".")
	if len(parts) == 0 {
		return 0
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}
	if major == 1 && len(parts) > 1 {
		if minor, err := strconv.Atoi(parts[1]); err == nil {
			return minor
		}
	}
	return major
}

// FormatInfo builds a short "Java : ..." status line, cross-checking the
// installed default against a selected version's requirement when known.
func FormatInfo(installedMajor int, requiredMajor int) string {
	if installedMajor == 0 {
		return "not found"
	}
	if requiredMajor == 0 {
		return strconvItoa(installedMajor) + " (default)"
	}
	if installedMajor >= requiredMajor {
		return strconvItoa(installedMajor) + " (default, OK)"
	}
	return strconvItoa(installedMajor) + " (default) — needs " + strconvItoa(requiredMajor) + "+ ⚠"
}

func strconvItoa(n int) string {
	return strconv.Itoa(n)
}

// scanEmbeddedRuntimes looks for JREs bundled by the official
// Mojang/Microsoft launcher under <mcDir>/runtime/. These are often newer
// than the user's system Java and require no extra installation — if the
// user has ever played via the official launcher, a compatible JRE is
// frequently already sitting here.
//
// Structure: <mcDir>/runtime/<component>/<os-dir>/<component>/bin/java(.exe)
// e.g. .minecraft/runtime/java-runtime-delta/windows-x64/java-runtime-delta/bin/java.exe
//
// We glob both variable segments (<component> and <os-dir>) rather than
// hardcoding them, since they vary by platform/architecture and by which
// Minecraft versions were ever installed.
func scanEmbeddedRuntimes(mcDir string) []Candidate {
	runtimeRoot := filepath.Join(mcDir, "runtime")

	binName := "java"
	if runtime.GOOS == "windows" {
		binName = "java.exe"
	}

	pattern := filepath.Join(runtimeRoot, "*", "*", "*", "bin", binName)
	return scanPattern(pattern)
}

// scanSystemInstallations looks in OS-conventional locations for
// installed JREs/JDKs and returns them sorted by major version,
// descending.
func scanSystemInstallations() []Candidate {
	var dirs []string

	switch runtime.GOOS {
	case "linux":
		dirs = globDirs("/usr/lib/jvm/*")
	case "darwin":
		dirs = globDirs("/Library/Java/JavaVirtualMachines/*/Contents/Home")
	case "windows":
		dirs = globDirs(`C:\Program Files\Java\*`)
		dirs = append(dirs, globDirs(`C:\Program Files\Eclipse Adoptium\*`)...)
		dirs = append(dirs, globDirs(`C:\Program Files\Microsoft\*`)...)
		if userProfile := os.Getenv("USERPROFILE"); userProfile != "" {
			dirs = append(dirs, globDirs(filepath.Join(userProfile, ".jdks", "*"))...)
		}
	}

	binName := "java"
	if runtime.GOOS == "windows" {
		binName = "java.exe"
	}

	var candidates []Candidate
	for _, dir := range dirs {
		binPath := filepath.Join(dir, "bin", binName)
		if _, err := os.Stat(binPath); err != nil {
			continue
		}
		if major := VersionAt(binPath); major > 0 {
			candidates = append(candidates, Candidate{Path: binPath, Major: major})
		}
	}
	return sortDescending(candidates)
}

// scanPattern globs pattern and returns valid Java candidates found at
// the matched paths, sorted by major version descending.
func scanPattern(pattern string) []Candidate {
	var candidates []Candidate
	for _, binPath := range globDirs(pattern) {
		if _, err := os.Stat(binPath); err != nil {
			continue
		}
		if major := VersionAt(binPath); major > 0 {
			candidates = append(candidates, Candidate{Path: binPath, Major: major})
		}
	}
	return sortDescending(candidates)
}

// sortDescending sorts candidates by Major version, descending (simple
// insertion sort, lists are always small).
func sortDescending(candidates []Candidate) []Candidate {
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && candidates[j-1].Major < candidates[j].Major; j-- {
			candidates[j-1], candidates[j] = candidates[j], candidates[j-1]
		}
	}
	return candidates
}

// globDirs is a small wrapper around filepath.Glob that returns an empty
// slice instead of an error.
func globDirs(pattern string) []string {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}
	return matches
}
