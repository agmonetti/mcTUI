package modrinth

import "time"

type Project struct {
	ID          string `json:"id"`
	ProjectType string `json:"project_type"`
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Body        string `json:"body"`
	Downloads   int64  `json:"downloads"`
	Team        string `json:"team"`
}
type Hashes struct {
	SHA512 string `json:"sha512"`
	SHA256 string `json:"sha256"`
	SHA1   string `json:"sha1"`
}
type File struct {
	Hashes   Hashes `json:"hashes"`
	URL      string `json:"url"`
	Filename string `json:"filename"`
	Primary  bool   `json:"primary"`
	Size     int64  `json:"size"`
}
type Dependency struct {
	VersionID string `json:"version_id"`
	ProjectID string `json:"project_id"`
	FileName  string `json:"file_name"`
	Type      string `json:"dependency_type"`
}
type Version struct {
	ID           string       `json:"id"`
	ProjectID    string       `json:"project_id"`
	Name         string       `json:"name"`
	Number       string       `json:"version_number"`
	Channel      string       `json:"version_type"`
	GameVersions []string     `json:"game_versions"`
	Loaders      []string     `json:"loaders"`
	Published    time.Time    `json:"date_published"`
	Files        []File       `json:"files"`
	Dependencies []Dependency `json:"dependencies"`
}
type SearchResult struct {
	ProjectID   string `json:"project_id"`
	ProjectType string `json:"project_type"`
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Author      string `json:"author"`
	Downloads   int64  `json:"downloads"`
}
type SearchResponse struct {
	Hits   []SearchResult `json:"hits"`
	Offset int            `json:"offset"`
	Limit  int            `json:"limit"`
	Total  int            `json:"total_hits"`
}
type VersionFileUpdateResponse map[string]Version

func (v Version) PrimaryFile() (File, bool) {
	for _, f := range v.Files {
		if f.Primary {
			return f, true
		}
	}
	if len(v.Files) == 1 {
		return v.Files[0], true
	}
	return File{}, false
}
