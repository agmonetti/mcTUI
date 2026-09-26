package mods

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mcTUI/internal/modrinth"
)

type identifyFunc func([]string) (map[string]modrinth.Version, error)

func (f identifyFunc) Identify(_ context.Context, hashes []string) (map[string]modrinth.Version, error) {
	return f(hashes)
}

func TestIdentifyLocalMods(t *testing.T) {
	for _, scenario := range []string{"recognized", "managed", "unknown", "changed", "registry changed", "invalid", "offline"} {
		t.Run(scenario, func(t *testing.T) {
			s := store(t)
			filename := filepath.Join(s.GameDir, "mods", "external.jar")
			jar(t, filename, map[string]string{"fabric.mod.json": fabricJSON})
			if scenario == "managed" || scenario == "registry changed" {
				r, _ := s.load()
				baseline, snapErr := snapshot(filename)
				if snapErr != nil {
					t.Fatal(snapErr)
				}
				baseline.Origin = "local"
				put(&r, baseline.File, &baseline)
				if snapErr = s.save(r); snapErr != nil {
					t.Fatal(snapErr)
				}
			}
			before, err := snapshot(filename)
			if err != nil {
				t.Fatal(err)
			}
			api := identifyFunc(func(hashes []string) (map[string]modrinth.Version, error) {
				if len(hashes) != 1 || hashes[0] != before.Hashes.SHA512 {
					t.Fatalf("unexpected hashes: %v", hashes)
				}
				switch scenario {
				case "offline":
					return nil, errors.New("offline")
				case "unknown":
					return nil, nil
				case "changed":
					jar(t, filename, map[string]string{"fabric.mod.json": fabricJSON, "new": "changed"})
				case "registry changed":
					r, _ := s.load()
					r.Mods[0].ProjectID = "concurrent"
					if err := s.save(r); err != nil {
						t.Fatal(err)
					}
				}
				v := modrinth.Version{ID: "version", ProjectID: "project", Channel: "release", Files: []modrinth.File{{Hashes: modrinth.Hashes{SHA512: before.Hashes.SHA512}}}}
				if scenario == "invalid" {
					v.Files[0].Hashes.SHA512 = strings.Repeat("0", 128)
				}
				return map[string]modrinth.Version{before.Hashes.SHA512: v}, nil
			})
			err = s.Identify(context.Background(), api)
			if (err != nil) != (scenario == "changed" || scenario == "registry changed" || scenario == "invalid" || scenario == "offline") {
				t.Fatalf("unexpected identification error: %v", err)
			}
			items, err := s.Scan()
			if err != nil || len(items) != 1 {
				t.Fatalf("scan: %+v %v", items, err)
			}
			if scenario == "recognized" || scenario == "managed" {
				if !items[0].Managed || items[0].Origin != map[bool]string{true: "local", false: "external"}[scenario == "managed"] || items[0].ProjectID != "project" || items[0].VersionID != "version" || items[0].Hashes.SHA512 != before.Hashes.SHA512 {
					t.Fatalf("identification not persisted: %+v", items[0])
				}
			} else if items[0].ProjectID != map[bool]string{true: "concurrent", false: ""}[scenario == "registry changed"] {
				t.Fatalf("false attribution: %+v", items[0])
			}
			if scenario == "offline" {
				if err := s.Remove(items[0], true); err != nil {
					t.Fatalf("offline local management blocked: %v", err)
				}
			} else if _, err := os.Stat(filename); scenario != "offline" && err != nil {
				t.Fatal("JAR removed", err)
			}
		})
	}
}
