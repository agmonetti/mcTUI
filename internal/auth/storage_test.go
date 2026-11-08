package auth

import (
	"os"
	"testing"
	"time"
)

func TestOfflineUUID(t *testing.T) {
	// Standard Minecraft test vector for "Steve" offline UUID:
	// md5("OfflinePlayer:Steve") with RFC 4122 v3 flags
	uuid1 := OfflineUUID("Steve")
	if uuid1 == "" || len(uuid1) != 36 {
		t.Fatalf("expected 36-char uuid, got %q", uuid1)
	}

	// Determinism check
	uuid2 := OfflineUUID("Steve")
	if uuid1 != uuid2 {
		t.Fatalf("expected deterministic uuid, got %s and %s", uuid1, uuid2)
	}

	// Different username check
	uuidAlex := OfflineUUID("Alex")
	if uuid1 == uuidAlex {
		t.Fatalf("expected different uuid for Alex, got same")
	}
}

func TestStorageAddRemoveSetActive(t *testing.T) {
	tmpDir := t.TempDir()
	origAppData := os.Getenv("APPDATA")
	origXDG := os.Getenv("XDG_DATA_HOME")
	origHome := os.Getenv("HOME")
	defer func() {
		os.Setenv("APPDATA", origAppData)
		os.Setenv("XDG_DATA_HOME", origXDG)
		os.Setenv("HOME", origHome)
	}()

	os.Setenv("APPDATA", tmpDir)
	os.Setenv("XDG_DATA_HOME", tmpDir)
	os.Setenv("HOME", tmpDir)

	s := &Storage{
		ActiveAccountID: "acc-1",
		Accounts: []Account{
			{
				ID:        "acc-1",
				Username:  "User1",
				Type:      AccountTypeOffline,
				CreatedAt: time.Now(),
			},
		},
	}

	// Test GetActive
	act := s.GetActive()
	if act == nil || act.Username != "User1" {
		t.Fatalf("expected User1, got %v", act)
	}

	// Test AddAccount
	acc2 := Account{
		ID:        "acc-2",
		Username:  "User2",
		Type:      AccountTypeMicrosoft,
		CreatedAt: time.Now(),
	}
	s.AddAccount(acc2, true)
	if s.ActiveAccountID != "acc-2" {
		t.Fatalf("expected active acc-2, got %s", s.ActiveAccountID)
	}
	if len(s.Accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(s.Accounts))
	}

	// Test SetActive
	err := s.SetActive("acc-1")
	if err != nil || s.ActiveAccountID != "acc-1" {
		t.Fatalf("failed to set active to acc-1: %v", err)
	}

	// Test RemoveAccount
	removed := s.RemoveAccount("acc-1")
	if !removed {
		t.Fatalf("expected acc-1 to be removed")
	}
	if s.ActiveAccountID != "acc-2" {
		t.Fatalf("expected active to fall back to acc-2, got %s", s.ActiveAccountID)
	}

	// Test Save and Load
	err = SaveStorage(s)
	if err != nil {
		t.Fatalf("failed to save storage: %v", err)
	}

	loaded, err := LoadStorage()
	if err != nil {
		t.Fatalf("failed to load storage: %v", err)
	}
	if len(loaded.Accounts) != 1 || loaded.ActiveAccountID != "acc-2" {
		t.Fatalf("loaded data mismatch: %+v", loaded)
	}
}

func TestFormatUUID(t *testing.T) {
	raw := "c36a9fb64f2a41ff90bdae7cc92031eb"
	expected := "c36a9fb6-4f2a-41ff-90bd-ae7cc92031eb"
	got := FormatUUID(raw)
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}

	// Already formatted
	if FormatUUID(expected) != expected {
		t.Fatalf("expected unchanged formatted uuid")
	}
}

func TestLoadStorageEmptyWhenNoFile(t *testing.T) {
	tmpDir := t.TempDir()
	origAppData := os.Getenv("APPDATA")
	origXDG := os.Getenv("XDG_DATA_HOME")
	origHome := os.Getenv("HOME")
	defer func() {
		os.Setenv("APPDATA", origAppData)
		os.Setenv("XDG_DATA_HOME", origXDG)
		os.Setenv("HOME", origHome)
	}()

	os.Setenv("APPDATA", tmpDir)
	os.Setenv("XDG_DATA_HOME", tmpDir)
	os.Setenv("HOME", tmpDir)

	loaded, err := LoadStorage()
	if err != nil {
		t.Fatalf("unexpected error loading empty storage: %v", err)
	}
	if len(loaded.Accounts) != 0 || loaded.ActiveAccountID != "" {
		t.Fatalf("expected 0 accounts and empty active account, got %+v", loaded)
	}
}
