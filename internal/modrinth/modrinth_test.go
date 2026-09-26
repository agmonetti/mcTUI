package modrinth

import (
	"context"
	"crypto/sha512"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mock(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}
	return &Client{HTTP: &http.Client{Transport: transport}}
}
func TestNetworkLimitsAndRedirects(t *testing.T) {
	c := mock(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != UserAgent {
			t.Errorf("wrong user agent")
		}
		switch r.URL.Path {
		case "/v2/search":
			w.Header().Set("X-Ratelimit-Limit", "300")
			w.Header().Set("X-Ratelimit-Remaining", "299")
			w.Header().Set("X-Ratelimit-Reset", "60")
			fmt.Fprint(w, `{"hits":[],"total_hits":0}`)
		case "/v2/project/redirect":
			http.Redirect(w, r, "https://evil.example/", http.StatusFound)
		case "/v2/project/http":
			http.Redirect(w, r, "http://api.modrinth.com/v2/search", http.StatusFound)
		case "/v2/project/large":
			w.Header().Set("Content-Length", fmt.Sprint(maxJSON+1))
			w.WriteHeader(200)
		case "/v2/project/429":
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(429)
		case "/v2/project/slow":
			<-r.Context().Done()
		}
	})
	if _, err := c.Search(context.Background(), "sodium", "fabric", "1.21.1", 0); err != nil {
		t.Fatal(err)
	}
	if c.Rate().Remaining != 299 || c.Rate().Reset != time.Minute {
		t.Fatalf("rate: %+v", c.Rate())
	}
	for _, id := range []string{"redirect", "http", "large"} {
		if _, err := c.Project(context.Background(), id); err == nil {
			t.Fatalf("accepted %s", id)
		}
	}
	if _, err := c.Project(context.Background(), "429"); err == nil {
		t.Fatal("429 accepted")
	}
	if _, err := c.Project(context.Background(), "slow"); err == nil {
		t.Fatal("rate limit not enforced")
	}
	c.mu.Lock()
	c.until = time.Time{}
	c.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := c.Project(ctx, "slow"); err == nil {
		t.Fatal("timeout ignored")
	}
}
func TestStageIntegrityAndCleanup(t *testing.T) {
	data := []byte("a verified JAR")
	digest := sha512.Sum512(data)
	c := mock(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/large" {
			w.Header().Set("Content-Length", fmt.Sprint(maxJAR+1))
			return
		}
		w.Write(data)
	})
	dir := t.TempDir()
	f := File{URL: "https://cdn.modrinth.net/test.jar", Filename: "test.jar", Size: int64(len(data)), Hashes: Hashes{SHA512: hex.EncodeToString(digest[:])}}
	name, err := c.Stage(context.Background(), f, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(filepath.Base(name), ".tmp-") {
		t.Fatal("not staged")
	}
	os.Remove(name)
	f.Hashes.SHA512 = strings.Repeat("0", 128)
	if _, err = c.Stage(context.Background(), f, dir, nil); err == nil {
		t.Fatal("accepted wrong hash")
	}
	f.URL = "https://cdn.modrinth.net/large"
	if _, err = c.Stage(context.Background(), f, dir, nil); err == nil {
		t.Fatal("accepted oversized response")
	}
	if files, _ := os.ReadDir(dir); len(files) != 0 {
		t.Fatalf("temporary files left: %v", files)
	}
	f.URL = "http://cdn.modrinth.net/test.jar"
	if _, err = c.Stage(context.Background(), f, dir, nil); err == nil {
		t.Fatal("accepted HTTP")
	}
}
func TestCompatibleAndResolver(t *testing.T) {
	f := File{Filename: "test.jar", Primary: true, Hashes: Hashes{SHA512: strings.Repeat("a", 128)}}
	version := func(id string) Version {
		return Version{ID: id, ProjectID: id, Channel: "release", Loaders: []string{"fabric"}, GameVersions: []string{"1.21.1"}, Files: []File{f}}
	}
	a, b := version("A"), version("B")
	a.Dependencies = []Dependency{{ProjectID: "B", Type: "required"}, {ProjectID: "C", Type: "embedded"}}
	b.Dependencies = []Dependency{{ProjectID: "A", Type: "required"}}
	c := mock(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/project/B/version") {
			fmt.Fprintf(w, "[%s]", mustJSON(t, b))
		} else if strings.Contains(r.URL.Path, "/project/D/version") {
			w.Write([]byte(`[]`))
		}
	})
	p, err := c.Resolve(context.Background(), a, "fabric", "1.21.1", "release", nil)
	if err != nil || len(p.RequiredDeps) != 1 || len(p.EmbeddedMods) != 1 {
		t.Fatalf("%+v %v", p, err)
	}
	p, err = c.Resolve(context.Background(), a, "fabric", "1.21.1", "release", map[string]string{"B": "B"})
	if err != nil || len(p.RequiredDeps) != 1 || !p.RequiredDeps[0].AlreadyInstalled || len(p.RequiredDeps[0].RequiredBy) != 1 || p.RequiredDeps[0].RequiredBy[0] != "A" {
		t.Fatalf("installed dependency relationship lost: %+v %v", p, err)
	}
	a.Dependencies = append(a.Dependencies, Dependency{ProjectID: "D", Type: "required"})
	if _, err = c.Resolve(context.Background(), a, "fabric", "1.21.1", "release", nil); err == nil {
		t.Fatal("missing required dependency accepted")
	}
	a.Dependencies = []Dependency{{ProjectID: "B", Type: "incompatible"}}
	if _, err = c.Resolve(context.Background(), a, "fabric", "1.21.1", "release", map[string]string{"B": "old"}); err == nil {
		t.Fatal("conflict accepted")
	}
	if _, err = Compatible([]Version{version("other")}, "forge", "1.21.1", "release"); err == nil {
		t.Fatal("wrong loader accepted")
	}
}
func TestSelectedOptionalIncludesRequiredDescendants(t *testing.T) {
	file := File{Filename: "mod.jar", Hashes: Hashes{SHA512: strings.Repeat("a", 128)}}
	version := func(id string) Version {
		return Version{ID: id, ProjectID: id, Channel: "release", Loaders: []string{"fabric"}, GameVersions: []string{"1.21"}, Files: []File{file}}
	}
	a, b, c, d := version("A"), version("B"), version("C"), version("D")
	a.Dependencies = []Dependency{{ProjectID: "B", Type: "optional"}}
	b.Dependencies = []Dependency{{ProjectID: "C", Type: "required"}}
	c.Dependencies = []Dependency{{ProjectID: "D", Type: "required"}}
	versions := map[string]Version{"B": b, "C": c, "D": d}
	client := mock(t, func(w http.ResponseWriter, r *http.Request) {
		for id, v := range versions {
			if strings.Contains(r.URL.Path, "/project/"+id+"/version") {
				fmt.Fprintf(w, "[%s]", mustJSON(t, v))
				return
			}
		}
		http.NotFound(w, r)
	})
	plan, err := client.Resolve(context.Background(), a, "fabric", "1.21", "release", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.SelectedOptional(nil); len(got) != 0 {
		t.Fatalf("unselected optional installed: %+v", got)
	}
	got := plan.SelectedOptional([]string{"B"})
	if len(got) != 3 || got[0].Version.ProjectID != "B" || got[1].Version.ProjectID != "C" || got[2].Version.ProjectID != "D" {
		t.Fatalf("required descendants omitted: %+v", got)
	}
}

func TestBatchGraphSharesDependenciesAndRejectsConflict(t *testing.T) {
	file := File{Filename: "mod.jar", Hashes: Hashes{SHA512: strings.Repeat("a", 128)}}
	version := func(id string) Version {
		return Version{ID: id, ProjectID: id, Channel: "release", Loaders: []string{"fabric"}, GameVersions: []string{"1.21"}, Files: []File{file}}
	}
	a, b, shared := version("A"), version("B"), version("shared")
	a.Dependencies = []Dependency{{ProjectID: "shared", Type: "required"}}
	b.Dependencies = []Dependency{{ProjectID: "shared", Type: "required"}}
	c := mock(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "[%s]", mustJSON(t, shared))
	})
	plan, err := c.ResolveBatch(context.Background(), []Version{a, b}, "fabric", "1.21", "release", map[string]string{"shared": "shared"})
	if err != nil || len(plan.RequiredDeps) != 1 || !plan.RequiredDeps[0].AlreadyInstalled || len(plan.RequiredDeps[0].RequiredBy) != 2 || len(plan.Targets) != 2 {
		t.Fatalf("shared dependency: %+v %v", plan, err)
	}
	b.Dependencies = []Dependency{{ProjectID: "A", Type: "incompatible"}}
	if _, err := c.ResolveBatch(context.Background(), []Version{a, b}, "fabric", "1.21", "release", nil); err == nil {
		t.Fatal("selected targets conflict")
	}
	b.Dependencies = []Dependency{{ProjectID: "shared", VersionID: "different", Type: "required"}}
	if _, err := c.ResolveBatch(context.Background(), []Version{a, b}, "fabric", "1.21", "release", nil); err == nil {
		t.Fatal("conflicting dependency version accepted")
	}
}
func TestOptionalConflictOnlyAppliesWhenChosen(t *testing.T) {
	file := File{Filename: "mod.jar", Hashes: Hashes{SHA512: strings.Repeat("a", 128)}}
	version := func(id string) Version {
		return Version{ID: id, ProjectID: id, Channel: "release", Loaders: []string{"fabric"}, GameVersions: []string{"1.21"}, Files: []File{file}}
	}
	a, b := version("A"), version("B")
	a.Dependencies = []Dependency{{ProjectID: "B", Type: "optional"}}
	b.Dependencies = []Dependency{{ProjectID: "A", Type: "incompatible"}, {ProjectID: "installed", Type: "incompatible"}}
	client := mock(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "[%s]", mustJSON(t, b))
	})
	plan, err := client.Resolve(context.Background(), a, "fabric", "1.21", "release", map[string]string{"installed": "v1"})
	if err != nil || len(plan.OptionalDeps) != 1 || len(plan.IncompatibleIDs) != 0 || len(plan.IncompatibleMods) != 0 {
		t.Fatalf("unselected optional blocked the required plan: %+v %v", plan, err)
	}
	if got := plan.OptionalDeps[0].IncompatibleIDs; len(got) != 2 || got[0] != "A" || got[1] != "installed" {
		t.Fatalf("optional conflicts not retained for confirmation: %v", got)
	}
}
func TestResolverIncompatiblePinnedDependencyNamesVersions(t *testing.T) {
	file := File{Filename: "sodium.jar", Primary: true, Hashes: Hashes{SHA512: strings.Repeat("a", 128)}}
	sodium := Version{
		ID: "opaque-version-id", ProjectID: "opaque-project-id",
		Name: "Sodium 0.9.2 for Fabric 26.1.2", Number: "mc26.1.2-0.9.2-fabric",
		Loaders: []string{"fabric"}, GameVersions: []string{"26.1.2"},
		Files: []File{file},
	}
	c := mock(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/version/opaque-version-id" {
			fmt.Fprint(w, mustJSON(t, sodium))
			return
		}
		http.NotFound(w, r)
	})
	iris := Version{
		ID: "iris-version", ProjectID: "iris-project",
		Loaders: []string{"fabric"}, GameVersions: []string{"26.1"},
		Files:        []File{file},
		Dependencies: []Dependency{{VersionID: sodium.ID, ProjectID: sodium.ProjectID, Type: "required"}},
	}

	_, err := c.Resolve(context.Background(), iris, "fabric", "26.1", "release", nil)
	if err == nil {
		t.Fatal("incompatible pinned dependency was accepted")
	}
	for _, want := range []string{"Sodium 0.9.2 for Fabric 26.1.2", "Minecraft [26.1.2]", "Minecraft 26.1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not explain %q", err, want)
		}
	}
	for _, opaque := range []string{sodium.ID, sodium.ProjectID} {
		if strings.Contains(err.Error(), opaque) {
			t.Errorf("error exposes opaque identifier %q: %v", opaque, err)
		}
	}
}

func TestResolveLatestCompatibleFallsBackForPinnedDependency(t *testing.T) {
	file := File{Filename: "mod.jar", Primary: true, Hashes: Hashes{SHA512: strings.Repeat("a", 128)}}
	sodiumNew := Version{
		ID: "sodium-new", ProjectID: "sodium", Name: "Sodium 0.9.2 for 26.1.2",
		Loaders: []string{"fabric"}, GameVersions: []string{"26.1.2"}, Files: []File{file},
	}
	sodiumOld := Version{
		ID: "sodium-old", ProjectID: "sodium", Name: "Sodium 0.8.9 for 26.1",
		Loaders: []string{"fabric"}, GameVersions: []string{"26.1"}, Files: []File{file},
	}
	iris := func(id, number string, published time.Time, sodiumVersion string) Version {
		return Version{
			ID: id, ProjectID: "iris", Number: number, Channel: "release", Published: published,
			Loaders: []string{"fabric"}, GameVersions: []string{"26.1", "26.1.1", "26.1.2"}, Files: []File{file},
			Dependencies: []Dependency{{ProjectID: "sodium", VersionID: sodiumVersion, Type: "required"}},
		}
	}
	older := iris("iris-old", "1.10.9", time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC), sodiumOld.ID)
	newest := iris("iris-new", "1.11.4", time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC), sodiumNew.ID)
	c := mock(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/project/iris/version":
			fmt.Fprint(w, mustJSON(t, []Version{older, newest}))
		case "/v2/version/sodium-new":
			fmt.Fprint(w, mustJSON(t, sodiumNew))
		case "/v2/version/sodium-old":
			fmt.Fprint(w, mustJSON(t, sodiumOld))
		default:
			http.NotFound(w, r)
		}
	})

	plan, err := c.ResolveLatestCompatible(context.Background(), "iris", "fabric", "26.1", "release", map[string]string{"sodium": sodiumOld.ID})
	if err != nil {
		t.Fatal(err)
	}
	if plan.TargetMod.ID != older.ID {
		t.Fatalf("selected Iris %s (%s), want compatible older release %s", plan.TargetMod.ID, plan.TargetMod.Number, older.ID)
	}
	if len(plan.RequiredDeps) != 1 || !plan.RequiredDeps[0].AlreadyInstalled || plan.RequiredDeps[0].Version.ID != sodiumOld.ID {
		t.Fatalf("compatible installed Sodium was not reused: %+v", plan.RequiredDeps)
	}
}

func TestResolveLatestCompatibleDoesNotFallbackOnIntegrityError(t *testing.T) {
	file := File{Filename: "mod.jar", Primary: true, Hashes: Hashes{SHA512: strings.Repeat("a", 128)}}
	sodiumBroken := Version{
		ID: "sodium-broken", ProjectID: "sodium", Name: "Sodium broken",
		Loaders: []string{"fabric"}, GameVersions: []string{"26.1"}, Files: []File{{Filename: "broken.jar", Primary: true}},
	}
	sodiumOld := Version{
		ID: "sodium-old", ProjectID: "sodium", Name: "Sodium 0.8.9",
		Loaders: []string{"fabric"}, GameVersions: []string{"26.1"}, Files: []File{file},
	}
	iris := func(id string, published time.Time, sodiumVersion string) Version {
		return Version{
			ID: id, ProjectID: "iris", Channel: "release", Published: published,
			Loaders: []string{"fabric"}, GameVersions: []string{"26.1"}, Files: []File{file},
			Dependencies: []Dependency{{ProjectID: "sodium", VersionID: sodiumVersion, Type: "required"}},
		}
	}
	newest := iris("iris-new", time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC), sodiumBroken.ID)
	older := iris("iris-old", time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC), sodiumOld.ID)
	oldDependencyRequests := 0
	c := mock(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/project/iris/version":
			fmt.Fprint(w, mustJSON(t, []Version{older, newest}))
		case "/v2/version/sodium-broken":
			fmt.Fprint(w, mustJSON(t, sodiumBroken))
		case "/v2/version/sodium-old":
			oldDependencyRequests++
			fmt.Fprint(w, mustJSON(t, sodiumOld))
		default:
			http.NotFound(w, r)
		}
	})

	_, err := c.ResolveLatestCompatible(context.Background(), "iris", "fabric", "26.1", "release", nil)
	if err == nil || !strings.Contains(err.Error(), "missing verified primary file") {
		t.Fatalf("integrity error was hidden by fallback: %v", err)
	}
	if oldDependencyRequests != 0 {
		t.Fatalf("fallback hid a non-compatibility error; older dependency fetched %d times", oldDependencyRequests)
	}
}

func TestSearchSortingIndex(t *testing.T) {
	var capturedIndex string
	c := mock(t, func(w http.ResponseWriter, r *http.Request) {
		capturedIndex = r.URL.Query().Get("index")
		fmt.Fprint(w, `{"hits":[],"total_hits":0}`)
	})
	if _, err := c.Search(context.Background(), "test", "fabric", "1.21.1", 0, "downloads"); err != nil {
		t.Fatal(err)
	}
	if capturedIndex != "downloads" {
		t.Fatalf("expected index=downloads, got %q", capturedIndex)
	}
	if _, err := c.Search(context.Background(), "test", "fabric", "1.21.1", 0, "invalid-sort"); err != nil {
		t.Fatal(err)
	}
	if capturedIndex != "relevance" {
		t.Fatalf("expected fallback to relevance, got %q", capturedIndex)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
