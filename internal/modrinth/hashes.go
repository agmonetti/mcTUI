package modrinth

import "context"

func (c *Client) Identify(ctx context.Context, hashes []string) (map[string]Version, error) {
	out := map[string]Version{}
	err := c.batch(ctx, "/version_files", map[string]any{"hashes": hashes, "algorithm": "sha512"}, &out)
	return out, err
}
func (c *Client) Updates(ctx context.Context, hashes []string, loader, minecraft string) (VersionFileUpdateResponse, error) {
	out := VersionFileUpdateResponse{}
	err := c.batch(ctx, "/version_files/update", map[string]any{"hashes": hashes, "algorithm": "sha512", "loaders": []string{loader}, "game_versions": []string{minecraft}}, &out)
	return out, err
}
