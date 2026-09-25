package config

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestDataDirChecked(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
		t.Setenv("LOCALAPPDATA", filepath.Join(home, "Local Data"))
	} else {
		t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	}
	got, err := DataDirChecked()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "data", "mctui")
	switch runtime.GOOS {
	case "windows":
		want = filepath.Join(home, "Local Data", "mctui")
	case "darwin":
		want = filepath.Join(home, "Library", "Application Support", "mctui")
	}
	if got != want {
		t.Fatalf("DataDirChecked() = %q, want %q", got, want)
	}
	if runtime.GOOS == "linux" || runtime.GOOS == "windows" {
		if runtime.GOOS == "linux" {
			t.Setenv("XDG_DATA_HOME", "relative")
		} else {
			t.Setenv("LOCALAPPDATA", "relative")
		}
		if _, err := DataDirChecked(); err == nil {
			t.Fatal("relative data root must be rejected")
		}
	}
}
