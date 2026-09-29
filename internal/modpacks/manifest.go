package modpacks

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Source distinguishes an API release from an arbitrary local or remote archive.
// Index.VersionID is a pack-authored string and must never be used as an API ID.
type Source struct {
	Type      string `json:"type"`
	ProjectID string `json:"project_id,omitempty"`
	VersionID string `json:"version_id,omitempty"`
}

type BaseFile struct {
	SHA512 string   `json:"sha512"`
	SHA1   string   `json:"sha1,omitempty"`
	Size   int64    `json:"size"`
	Layer  string   `json:"layer"`
	URLs   []string `json:"urls,omitempty"`
}

type Manifest struct {
	SchemaVersion     int                 `json:"schema_version"`
	Name              string              `json:"name"`
	Summary           string              `json:"summary,omitempty"`
	Source            Source              `json:"source"`
	IndexVersionID    string              `json:"index_version_id"`
	IndexSHA512       string              `json:"index_sha512"`
	MinecraftVersion  string              `json:"minecraft_version"`
	Loader            string              `json:"loader"`
	LoaderVersion     string              `json:"loader_version,omitempty"`
	Optional          map[string]bool     `json:"optional,omitempty"`
	Files             map[string]BaseFile `json:"files"`
}

func validHash(hash string, bytes int) bool {
	data, err := hex.DecodeString(hash)
	return err == nil && len(data) == bytes && strings.ToLower(hash) == hash
}

func (m Manifest) validate() error {
	if m.SchemaVersion != 1 || m.IndexVersionID == "" || m.Name == "" || !validHash(m.IndexSHA512, 64) || m.MinecraftVersion == "" ||
		(m.Loader != "Vanilla" && m.Loader != "Fabric") || m.Loader == "Fabric" && m.LoaderVersion == "" ||
		(m.Source.Type != "local" && m.Source.Type != "url" && m.Source.Type != "modrinth") ||
		(m.Source.Type == "modrinth" && (m.Source.ProjectID == "" || m.Source.VersionID == "")) ||
		(m.Source.Type != "modrinth" && (m.Source.ProjectID != "" || m.Source.VersionID != "")) {
		return fmt.Errorf("invalid or unsupported modpack manifest")
	}
	seen := make(map[string]bool, len(m.Files))
	for path, file := range m.Files {
		if err := ValidPath(path); err != nil {
			return err
		}
		key := strings.ToLower(path)
		if seen[key] || file.Size < 0 || !validHash(file.SHA512, 64) || file.SHA1 != "" && !validHash(file.SHA1, 20) ||
			(file.Layer != "downloads" && file.Layer != "overrides" && file.Layer != "client-overrides") {
			return fmt.Errorf("invalid modpack file: %s", path)
		}
		seen[key] = true
	}
	return nil
}

func loadManifest(dir string) (Manifest, error) {
	file := filepath.Join(dir, "pack.json")
	info, err := os.Lstat(file)
	if err != nil {
		return Manifest{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > 16<<20 {
		return Manifest{}, fmt.Errorf("invalid modpack manifest file")
	}
	f, err := os.Open(file)
	if err != nil {
		return Manifest{}, err
	}
	defer f.Close()
	if opened, err := f.Stat(); err != nil || !os.SameFile(info, opened) {
		return Manifest{}, fmt.Errorf("modpack manifest changed while reading")
	}
	var m Manifest
	dec := json.NewDecoder(io.LimitReader(f, 16<<20+1))
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, err
	}
	if err := m.validate(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func saveJSON(dir, name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".pack-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(dir, name))
}

func saveManifest(dir string, m Manifest) error {
	if err := m.validate(); err != nil {
		return err
	}
	return saveJSON(dir, "pack.json", m)
}
