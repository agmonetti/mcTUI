// Package fabric resolves the Fabric Loader launch profile and verifies its libraries.
package fabric

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	urlpkg "net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Profile records both the launch classpath and hashes for offline verification.
type Profile struct {
	MainClass        string
	ClasspathEntries []string
	Hashes           map[string]string
	URLs             map[string]string
}

type Downloader func(url, destination, expectedSHA1 string) error

func ProfilePath(mcDir, version string) string {
	return filepath.Join(mcDir, "versions", version, "fabric-profile.json")
}

func ProfileExists(mcDir, version string) bool {
	info, err := os.Stat(ProfilePath(mcDir, version))
	return err == nil && info.Size() > 0
}

var client = &http.Client{Timeout: 20 * time.Second}

func get(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%s: response too large", url)
	}
	return body, nil
}

func sha1For(ctx context.Context, url string) (string, error) {
	body, err := get(ctx, url+".sha1", 256)
	if err != nil {
		return "", err
	}
	hash := strings.TrimSpace(string(body))
	decoded, err := hex.DecodeString(hash)
	if err != nil || len(decoded) != 20 {
		return "", fmt.Errorf("invalid Fabric library SHA-1 at %s", url)
	}
	return hash, nil
}

// Resolve never falls back to Vanilla or trusts a nonempty, unverified cache.
func Resolve(ctx context.Context, targetVersion, mcDir, librariesPath string, wg *sync.WaitGroup, download Downloader) (Profile, error) {
	cachePath := ProfilePath(mcDir, targetVersion)
	var result Profile
	if cached, err := os.ReadFile(cachePath); err == nil && json.Unmarshal(cached, &result) == nil && result.MainClass != "" && len(result.ClasspathEntries) > 0 && len(result.Hashes) == len(result.ClasspathEntries) && len(result.URLs) == len(result.ClasspathEntries) {
		valid := true
		for _, path := range result.ClasspathEntries {
			rel, err := filepath.Rel(librariesPath, path)
			if err != nil || !filepath.IsLocal(rel) || result.Hashes[path] == "" || result.URLs[path] == "" {
				valid = false
				break
			}
		}
		if valid {
			for _, path := range result.ClasspathEntries {
				wg.Add(1)
				if err := download(result.URLs[path], path, result.Hashes[path]); err != nil {
					return Profile{}, err
				}
			}
			return result, nil
		}
	}

	loaderURL := fmt.Sprintf("https://meta.fabricmc.net/v2/versions/loader/%s", targetVersion)
	body, err := get(ctx, loaderURL, 2<<20)
	if err != nil {
		return Profile{}, err
	}
	var loaders []struct {
		Loader struct {
			Version string `json:"version"`
		} `json:"loader"`
	}
	if err := json.Unmarshal(body, &loaders); err != nil || len(loaders) == 0 || loaders[0].Loader.Version == "" {
		return Profile{}, fmt.Errorf("no valid Fabric loader for %s", targetVersion)
	}
	profileURL := fmt.Sprintf("https://meta.fabricmc.net/v2/versions/loader/%s/%s/profile/json", targetVersion, loaders[0].Loader.Version)
	body, err = get(ctx, profileURL, 2<<20)
	if err != nil {
		return Profile{}, err
	}
	var raw struct {
		MainClass string                       `json:"mainClass"`
		Libraries []struct{ Name, URL string } `json:"libraries"`
	}
	if err := json.Unmarshal(body, &raw); err != nil || raw.MainClass == "" {
		return Profile{}, fmt.Errorf("invalid Fabric profile: %v", err)
	}
	result = Profile{MainClass: raw.MainClass, Hashes: make(map[string]string), URLs: make(map[string]string)}
	for _, lib := range raw.Libraries {
		parts := strings.Split(lib.Name, ":")
		if len(parts) != 3 {
			return Profile{}, fmt.Errorf("invalid Fabric coordinate %q", lib.Name)
		}
		path := strings.ReplaceAll(parts[0], ".", "/") + "/" + parts[1] + "/" + parts[2] + "/" + parts[1] + "-" + parts[2] + ".jar"
		if !fs.ValidPath(path) || !filepath.IsLocal(path) || strings.ContainsAny(path, "\\:") {
			return Profile{}, fmt.Errorf("invalid Fabric library path %q", path)
		}
		base := lib.URL
		if base == "" {
			base = "https://maven.fabricmc.net/"
		}
		url := strings.TrimRight(base, "/") + "/" + path
		parsed, err := urlpkg.Parse(url)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
			return Profile{}, fmt.Errorf("unsafe Fabric library URL %q", url)
		}
		hash, err := sha1For(ctx, url)
		if err != nil {
			return Profile{}, err
		}
		dest := filepath.Join(librariesPath, filepath.FromSlash(path))
		result.ClasspathEntries = append(result.ClasspathEntries, dest)
		result.Hashes[dest] = hash
		result.URLs[dest] = url
		wg.Add(1)
		if err := download(url, dest, hash); err != nil {
			return Profile{}, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return Profile{}, err
	}
	data, err := json.Marshal(result)
	if err != nil {
		return Profile{}, err
	}
	f, err := os.CreateTemp(filepath.Dir(cachePath), ".mctui-fabric-*")
	if err != nil {
		return Profile{}, err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return Profile{}, err
	}
	if err := f.Close(); err != nil {
		return Profile{}, err
	}
	if err := os.Rename(f.Name(), cachePath); err != nil {
		return Profile{}, err
	}
	return result, nil
}
