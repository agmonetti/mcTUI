package launcher

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunGameUsesSeparateGameAndRuntimeDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell script")
	}
	root := t.TempDir()
	game := filepath.Join(root, "instance", "minecraft")
	runtimeDir := filepath.Join(root, "runtime")
	argsFile := filepath.Join(root, "args")
	pwdFile := filepath.Join(root, "pwd")
	java := filepath.Join(root, "java")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" + argsFile + "'\npwd > '" + pwdFile + "'\n"
	if err := os.WriteFile(java, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	runGame(game, runtimeDir, java, "fake-classpath", "fake.Main", "Player", "1.20.4", "assets", 1024, "")
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "--gameDir\n"+game+"\n") || !strings.Contains(string(args), "--assetsDir\n"+filepath.Join(runtimeDir, "assets")+"\n") {
		t.Fatalf("wrong game/runtime paths: %s", args)
	}
	pwd, err := os.ReadFile(pwdFile)
	if err != nil || strings.TrimSpace(string(pwd)) != game {
		t.Fatalf("working directory = %q, error = %v; want %q", pwd, err, game)
	}
}
