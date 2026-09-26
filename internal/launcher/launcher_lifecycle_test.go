package launcher

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"mcTUI/internal/operationlock"
)

func TestGameLifecycle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell Java stub; Windows needs a native process fixture")
	}
	for _, tc := range []struct {
		name, script string
		code         int
		kill         bool
	}{
		{"normal", "echo ready > ready; while [ ! -f finish ]; do sleep .05; done; echo finished", 0, false},
		{"error", "echo ready > ready; while [ ! -f finish ]; do sleep .05; done; exit 7", 7, false},
		{"forced", "echo ready > ready; exec sleep 60", -1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gameDir, runtimeDir := t.TempDir(), t.TempDir()
			java := filepath.Join(t.TempDir(), "java")
			if err := os.WriteFile(java, []byte("#!/bin/sh\n"+tc.script+"\n"), 0700); err != nil {
				t.Fatal(err)
			}
			game, err := StartPrepared(gameDir, runtimeDir, java, "Player", "1.20.4", 1024, Prepared{classpath: "fake", mainClass: "fake.Main", assetIndexID: "assets"})
			if err != nil {
				t.Fatal(err)
			}
			// Never leave a fixture running if an assertion fails.
			t.Cleanup(func() { _ = game.Kill(); _, _ = game.Wait() })
			deadline := time.Now().Add(5 * time.Second)
			for {
				if _, err := os.Stat(filepath.Join(gameDir, "ready")); err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("Java never became ready")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if release, err := operationlock.Acquire(gameDir); err == nil {
				_ = release()
				t.Fatal("lock released while Java is alive")
			}
			if tc.kill {
				if err := game.Kill(); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(filepath.Join(gameDir, "finish"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			// No caller has called Wait yet: the supervisor must reap Java and
			// release the lock independently of TUI message delivery.
			select {
			case <-game.done:
			case <-time.After(5 * time.Second):
				t.Fatal("supervisor did not finish")
			}
			code, err := game.Wait()
			if tc.kill {
				if err == nil || code == 0 {
					t.Fatalf("forced exit: code=%d err=%v", code, err)
				}
			} else if code != tc.code || (err != nil) != (tc.code != 0) {
				t.Fatalf("exit: code=%d err=%v; want code %d", code, err, tc.code)
			}
			if repeatCode, repeatErr := game.Wait(); repeatCode != code || !errors.Is(repeatErr, err) {
				t.Fatalf("second Wait: code=%d err=%v; first=%d %v", repeatCode, repeatErr, code, err)
			}
			if err := game.Kill(); err != nil {
				t.Fatalf("Kill after exit: %v", err)
			}
			release, err := operationlock.Acquire(gameDir)
			if err != nil {
				t.Fatal("lock held after Java exit:", err)
			}
			if err := release(); err != nil {
				t.Fatal(err)
			}
			if tc.name == "normal" {
				log, err := os.ReadFile(game.LogPath)
				if err != nil || !strings.Contains(string(log), "finished") {
					t.Fatalf("final log: %q, %v", log, err)
				}
			}
		})
	}
}
