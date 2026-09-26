package launcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"mcTUI/internal/fabric"
	"mcTUI/internal/mojang"
)

// Prepared contains the launch artifacts verified for one runtime and version.
// The paths are local to this runtime, not portable instance metadata.
type PrepareError struct {
	Stage string
	Err   error
}

func (e *PrepareError) Error() string { return e.Stage + ": " + e.Err.Error() }
func (e *PrepareError) Unwrap() error { return e.Err }

type Prepared struct {
	classpath    string
	mainClass    string
	assetIndexID string
	nativesDir   string
	javaMajor    int
}

// Prepare is the single preparation path used by both the TUI and Launch.
// progress is serialized; its total is intentionally unknown.
func Prepare(runtimeDir, version, modloader string, progress func(string)) (Prepared, error) {
	return PrepareContext(context.Background(), runtimeDir, version, modloader, progress)
}

func PrepareContext(ctx context.Context, runtimeDir, version, modloader string, progress func(string)) (Prepared, error) {
	if err := ctx.Err(); err != nil {
		return Prepared{}, err
	}
	if !filepath.IsAbs(runtimeDir) || !filepath.IsLocal(version) || version != filepath.Base(version) || strings.ContainsAny(version, "/\\") || version == "." || version == ".." || (modloader != "Vanilla" && modloader != "Fabric") {
		return Prepared{}, fmt.Errorf("invalid runtime, version or loader")
	}
	data, ok := mojang.CachedVersionData(runtimeDir, version)
	if !ok {
		var err error
		data, err = mojang.FetchVersionData(ctx, version, runtimeDir)
		if err != nil {
			return Prepared{}, &PrepareError{Stage: "metadata", Err: fmt.Errorf("version %s: %w", version, err)}
		}
	}
	if data.Downloads.Client.URL == "" || data.Downloads.Client.SHA1 == "" || !filepath.IsLocal(data.AssetIndex.ID) || data.AssetIndex.ID != filepath.Base(data.AssetIndex.ID) {
		return Prepared{}, fmt.Errorf("version metadata unavailable or incomplete for %s", version)
	}

	var wg sync.WaitGroup
	checked := mojang.Download(ctx, &wg)
	var mu sync.Mutex
	var firstErr error
	download := func(url, dest, hash string) error {
		err := checked(url, dest, hash)
		mu.Lock()
		if err != nil && firstErr == nil {
			firstErr = err
		}
		if progress != nil {
			progress(filepath.Base(dest))
		}
		mu.Unlock()
		return err
	}
	check := func() error {
		wg.Wait()
		mu.Lock()
		defer mu.Unlock()
		return firstErr
	}

	clientPath := mojang.ClientJarPath(runtimeDir, version)
	wg.Add(1)
	if err := download(data.Downloads.Client.URL, clientPath, data.Downloads.Client.SHA1); err != nil {
		return Prepared{}, &PrepareError{Stage: "client", Err: err}
	}

	var classpath []string
	librariesPath := filepath.Join(runtimeDir, "libraries")
	sem := make(chan struct{}, maxConcurrentDownloads)
	for _, lib := range data.Libraries {
		if !mojang.ShouldIncludeLibrary(lib.Name, lib.Rules) {
			continue
		}
		artifact := lib.Downloads.Artifact
		if artifact.URL == "" && artifact.Path == "" && len(lib.Natives) > 0 {
			continue // Native-only libraries are handled by DownloadAndExtractNatives.
		}
		if artifact.URL == "" || artifact.Path == "" || artifact.SHA1 == "" {
			wg.Wait()
			return Prepared{}, &PrepareError{Stage: "library", Err: fmt.Errorf("missing verifiable artifact for %s", lib.Name)}
		}
		if !filepath.IsLocal(artifact.Path) {
			wg.Wait()
			return Prepared{}, &PrepareError{Stage: "library", Err: fmt.Errorf("invalid library path %q", artifact.Path)}
		}
		path := filepath.Join(librariesPath, artifact.Path)
		classpath = append(classpath, path)
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return Prepared{}, ctx.Err()
		}
		wg.Add(1)
		go func() {
			defer func() { <-sem }()
			_ = download(artifact.URL, path, artifact.SHA1)
		}()
	}
	if err := check(); err != nil {
		return Prepared{}, &PrepareError{Stage: "library", Err: err}
	}

	mainClass := "net.minecraft.client.main.Main"
	if modloader == "Fabric" {
		profile, err := fabric.Resolve(ctx, version, runtimeDir, librariesPath, &wg, download)
		if err != nil {
			return Prepared{}, &PrepareError{Stage: "Fabric", Err: fmt.Errorf("profile unavailable for %s; Vanilla will not be launched: %w", version, err)}
		}
		mainClass = profile.MainClass
		classpath = append(classpath, profile.ClasspathEntries...)
	}

	assetIndexID, err := validateAssets(runtimeDir, data, &wg, download)
	if downloadErr := check(); downloadErr != nil {
		return Prepared{}, &PrepareError{Stage: "assets", Err: downloadErr}
	}
	if err != nil {
		return Prepared{}, &PrepareError{Stage: "assets", Err: err}
	}
	nativesDir, err := mojang.DownloadAndExtractNatives(ctx, data, runtimeDir, version, &wg, download)
	if err != nil {
		return Prepared{}, &PrepareError{Stage: "natives", Err: err}
	}
	if err := ctx.Err(); err != nil {
		return Prepared{}, err
	}
	classpath = append(classpath, clientPath)
	return Prepared{classpath: strings.Join(deduplicateClasspath(classpath), string(os.PathListSeparator)), mainClass: mainClass, assetIndexID: assetIndexID, nativesDir: nativesDir, javaMajor: data.JavaVersion.MajorVersion}, nil
}
