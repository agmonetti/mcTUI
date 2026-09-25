package java

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
)

func TestResolveBinaryRejectsIncompatibleOverrideAndFindsBundledJava(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell Java stub is Unix-only")
	}
	root := t.TempDir()
	writeJavaStub(t, filepath.Join(root, "old-java"), 17)
	if _, err := ResolveBinary(21, root, filepath.Join(root, "old-java")); err == nil {
		t.Fatal("accepted Java 17 for a Java 21 requirement")
	}

	bundled := filepath.Join(root, "runtime", "runtime-test", "test-platform", "runtime-test", "bin", "java")
	writeJavaStub(t, bundled, 999)
	got, err := ResolveBinary(999, root, "")
	if err != nil || got.Path != bundled || got.Major != 999 {
		t.Fatalf("ResolveBinary() = %+v, %v; want bundled Java 999", got, err)
	}
}

func writeJavaStub(t *testing.T, path string, major int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho 'openjdk version \""+strconv.Itoa(major)+".0.1\"' >&2\n"), 0700); err != nil {
		t.Fatal(err)
	}
}
