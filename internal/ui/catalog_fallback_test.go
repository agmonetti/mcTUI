package ui

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mcTUI/internal/instances"
	"mcTUI/internal/modrinth"
)

func TestCatalogResolveFallsBackToDependencyCompatibleRelease(t *testing.T) {
	file := modrinth.File{
		Filename: "iris.jar", Primary: true,
		Hashes: modrinth.Hashes{SHA512: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	}
	sodiumNew := modrinth.Version{
		ID: "sodium-new", ProjectID: "sodium", Name: "Sodium 0.9.2",
		Loaders: []string{"fabric"}, GameVersions: []string{"26.1.2"},
		Files: []modrinth.File{file},
	}
	sodiumOld := modrinth.Version{
		ID: "sodium-old", ProjectID: "sodium", Name: "Sodium 0.8.9",
		Loaders: []string{"fabric"}, GameVersions: []string{"26.1"},
		Files: []modrinth.File{file},
	}
	iris := func(id, number, sodiumID string, published time.Time) modrinth.Version {
		return modrinth.Version{
			ID: id, ProjectID: "iris", Number: number, Channel: "release", Published: published,
			Loaders: []string{"fabric"}, GameVersions: []string{"26.1", "26.1.1", "26.1.2"},
			Files:        []modrinth.File{file},
			Dependencies: []modrinth.Dependency{{ProjectID: "sodium", VersionID: sodiumID, Type: "required"}},
		}
	}
	older := iris("iris-old", "1.10.9", sodiumOld.ID, time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC))
	newest := iris("iris-new", "1.11.4", sodiumNew.ID, time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC))
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var value any
		switch r.URL.Path {
		case "/v2/project/iris/version":
			value = []modrinth.Version{older, newest}
		case "/v2/version/sodium-new":
			value = sodiumNew
		case "/v2/version/sodium-old":
			value = sodiumOld
		default:
			http.NotFound(w, r)
			return
		}
		if err := json.NewEncoder(w).Encode(value); err != nil {
			t.Errorf("encode mock response: %v", err)
		}
	}))
	defer server.Close()

	previous := catalogAPI
	catalogAPI = &modrinth.Client{HTTP: &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		},
	}}}
	defer func() { catalogAPI = previous }()

	m := Model{
		catalogVersion: modrinth.Version{ProjectID: "iris"},
		catalogContext: context.Background(),
		instance:       instances.Instance{Loader: "Fabric", MinecraftVersion: "26.1"},
	}
	msg := m.catalogResolve()().(catalogPlanMsg)
	if msg.Err != nil {
		t.Fatal(msg.Err)
	}
	if msg.Plan.TargetMod.ID != older.ID || msg.Plan.TargetMod.Number != older.Number {
		t.Fatalf("catalog selected %s (%s), want dependency-compatible %s (%s)", msg.Plan.TargetMod.ID, msg.Plan.TargetMod.Number, older.ID, older.Number)
	}
}
