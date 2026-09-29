package modpacks

import (
	"context"
	"fmt"
	"os"

	"mcTUI/internal/modrinth"
)

// StageURL downloads a .mrpack for inspection before the user approves an
// installation. The caller must always invoke cleanup, including on errors.
func (s Store) StageURL(ctx context.Context, url string, hashes Hashes, size int64) (archive Archive, cleanup func(), err error) {
	cleanup = func() {}
	if err = s.root(); err != nil {
		return archive, cleanup, err
	}
	if err = os.MkdirAll(s.Root, 0700); err != nil {
		return archive, cleanup, err
	}
	path, err := Fetch(ctx, s.HTTP, []string{url}, hashes, size, 512<<20, s.Root)
	if err != nil {
		return archive, cleanup, err
	}
	cleanup = func() { _ = os.Remove(path) }
	archive, err = Inspect(path)
	return archive, cleanup, err
}

// StageVersion accepts only a modpack release from the expected project; the
// API version ID is independent of the pack-authored index versionId.
func (s Store) StageVersion(ctx context.Context, client *modrinth.Client, projectID, versionID string) (Archive, Source, func(), error) {
	if client == nil || projectID == "" || versionID == "" {
		return Archive{}, Source{}, func() {}, fmt.Errorf("missing modpack project or version")
	}
	project, err := client.Project(ctx, projectID)
	if err != nil {
		return Archive{}, Source{}, func() {}, err
	}
	if project.ID != projectID || project.ProjectType != "modpack" {
		return Archive{}, Source{}, func() {}, fmt.Errorf("project is not the selected modpack")
	}
	version, err := client.Version(ctx, versionID)
	if err != nil {
		return Archive{}, Source{}, func() {}, err
	}
	if version.ID != versionID || version.ProjectID != projectID {
		return Archive{}, Source{}, func() {}, fmt.Errorf("release does not belong to selected modpack")
	}
	file, ok := version.PrimaryFile()
	if !ok || len(file.Filename) < len(".mrpack") || file.Filename[len(file.Filename)-len(".mrpack"):] != ".mrpack" || file.Hashes.SHA512 == "" || file.Hashes.SHA1 == "" {
		return Archive{}, Source{}, func() {}, fmt.Errorf("modpack release has no verifiable .mrpack")
	}
	archive, cleanup, err := s.StageURL(ctx, file.URL, Hashes{SHA1: file.Hashes.SHA1, SHA512: file.Hashes.SHA512}, file.Size)
	return archive, Source{Type: "modrinth", ProjectID: project.ID, VersionID: version.ID}, cleanup, err
}
