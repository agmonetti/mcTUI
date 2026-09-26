package mods

import (
	"context"
	"crypto/sha512"
	"crypto/tls"
	"encoding/hex"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mcTUI/internal/modrinth"
)

func TestPrunePersistsHistoryBeforeDeletingBackups(t *testing.T) {
	s := store(t)
	if err := os.MkdirAll(s.backupDir(), 0700); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(s.backupDir(), "old.jar")
	newer := filepath.Join(s.backupDir(), "new.jar")
	// A nonempty directory cannot be removed; simulate a cleanup failure after
	// the history update without relying on OS permissions.
	if err := os.Mkdir(old, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "blocker"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newer, []byte("jar"), 0600); err != nil {
		t.Fatal(err)
	}
	entries := []HistoryEntry{
		{ProjectID: "p", Previous: Record{File: "old.jar"}, Backup: "old.jar"},
		{ProjectID: "p", Previous: Record{File: "new.jar"}, Backup: "new.jar"},
	}
	if err := s.saveHistory(entries); err != nil {
		t.Fatal(err)
	}
	if _, err := s.prune(entries); err == nil {
		t.Fatal("expected cleanup failure")
	}
	got, err := s.history()
	if err != nil || len(got) != 1 || got[0].Backup != "new.jar" {
		t.Fatalf("history points at discarded backup: %+v %v", got, err)
	}
	if _, err := os.Stat(newer); err != nil {
		t.Fatalf("retained backup lost: %v", err)
	}
}

func TestInstallNetworkFailureAfterFirstDownloadPublishesNothing(t *testing.T) {
	s := store(t)
	path := filepath.Join(t.TempDir(), "valid.jar")
	jar(t, path, nil)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/second.jar" {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()
	transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}
	sum := sha512.Sum512(body)
	plan := modrinth.InstallPlan{}
	for _, name := range []string{"first.jar", "second.jar"} {
		file := modrinth.File{URL: "https://cdn.modrinth.net/" + name, Filename: name, Size: int64(len(body)), Hashes: modrinth.Hashes{SHA512: hex.EncodeToString(sum[:])}}
		plan.Targets = append(plan.Targets, modrinth.PlanItem{Version: modrinth.Version{ID: name, ProjectID: name, Channel: "release"}, File: file})
	}
	err = s.Install(context.Background(), &modrinth.Client{HTTP: &http.Client{Transport: transport}}, plan, nil, false, nil)
	if err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("batch network failure not reported: %v", err)
	}
	files, err := os.ReadDir(filepath.Join(s.GameDir, "mods"))
	if err != nil || len(files) != 0 {
		t.Fatalf("partial batch published: %v %v", files, err)
	}
	items, err := s.Scan()
	if err != nil || len(items) != 0 {
		t.Fatalf("registry changed: %+v %v", items, err)
	}
	if history, err := s.History(); err != nil || len(history) != 0 {
		t.Fatalf("history changed: %+v %v", history, err)
	}
}
