package auth

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mcTUI/internal/config"
)

var (
	storageMu sync.RWMutex
	// DefaultClientID can be overridden by MCTUI_MSA_CLIENT_ID environment variable.
	// We use the established whitelisted Client ID accepted by Mojang's authentication endpoints.
	DefaultClientID = "c36a9fb6-4f2a-41ff-90bd-ae7cc92031eb"
)

// GetClientID returns the Azure Client ID used for Microsoft OAuth.
func GetClientID() string {
	if env := os.Getenv("MCTUI_MSA_CLIENT_ID"); env != "" {
		return env
	}
	return DefaultClientID
}

// OfflineUUID generates a deterministic Minecraft UUID (v3 MD5 of "OfflinePlayer:<username>").
// Format: 8-4-4-4-12 with lowercase hex.
func OfflineUUID(username string) string {
	hasher := md5.New()
	hasher.Write([]byte("OfflinePlayer:" + username))
	hash := hasher.Sum(nil)

	// Set version 3 (0x30)
	hash[6] = (hash[6] & 0x0f) | 0x30
	// Set RFC 4122 variant (0x80)
	hash[8] = (hash[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		hash[0:4], hash[4:6], hash[6:8], hash[8:10], hash[10:16])
}

// AccountsPath returns the file path for accounts.json under the app data directory.
func AccountsPath() string {
	return config.AppDataDir("mctui", "accounts.json")
}

// LoadStorage loads the accounts configuration. If the file doesn't exist, it initializes
// an account based on the existing username from config.json (or "Player").
func LoadStorage() (*Storage, error) {
	storageMu.RLock()
	defer storageMu.RUnlock()

	path := AccountsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return migrateFromConfig()
		}
		return nil, fmt.Errorf("read accounts file: %w", err)
	}

	var s Storage
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("unmarshal accounts: %w", err)
	}

	// Ensure there is at least an active account if accounts exist
	if len(s.Accounts) > 0 {
		foundActive := false
		for _, acc := range s.Accounts {
			if acc.ID == s.ActiveAccountID {
				foundActive = true
				break
			}
		}
		if !foundActive {
			s.ActiveAccountID = s.Accounts[0].ID
		}
	} else {
		return migrateFromConfig()
	}

	return &s, nil
}

// SaveStorage atomically saves the accounts configuration with 0600 permissions.
func SaveStorage(s *Storage) error {
	storageMu.Lock()
	defer storageMu.Unlock()

	path := AccountsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	bytes, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal accounts: %w", err)
	}

	tmpFile := path + ".tmp"
	if err := os.WriteFile(tmpFile, bytes, 0600); err != nil {
		return fmt.Errorf("write tmp accounts: %w", err)
	}

	if err := os.Rename(tmpFile, path); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("replace accounts file: %w", err)
	}

	return nil
}

func migrateFromConfig() (*Storage, error) {
	cfg := config.Load()
	defaultName := "Player"
	if strings.TrimSpace(cfg.Username) != "" {
		defaultName = strings.TrimSpace(cfg.Username)
	}

	offlineID := OfflineUUID(defaultName)
	initAcc := Account{
		ID:        offlineID,
		Username:  defaultName,
		Type:      AccountTypeOffline,
		CreatedAt: time.Now(),
	}

	s := &Storage{
		ActiveAccountID: offlineID,
		Accounts:        []Account{initAcc},
	}
	return s, nil
}

// GetActive returns a pointer to the active Account, or nil if none.
func (s *Storage) GetActive() *Account {
	for i := range s.Accounts {
		if s.Accounts[i].ID == s.ActiveAccountID {
			return &s.Accounts[i]
		}
	}
	if len(s.Accounts) > 0 {
		return &s.Accounts[0]
	}
	return nil
}

// SetActive sets the active account ID and returns an error if not found.
func (s *Storage) SetActive(id string) error {
	for _, acc := range s.Accounts {
		if acc.ID == id {
			s.ActiveAccountID = id
			return nil
		}
	}
	return fmt.Errorf("account not found: %s", id)
}

// AddAccount adds or updates an account by ID and sets it as active.
func (s *Storage) AddAccount(acc Account, makeActive bool) {
	for i := range s.Accounts {
		if s.Accounts[i].ID == acc.ID {
			s.Accounts[i] = acc
			if makeActive {
				s.ActiveAccountID = acc.ID
			}
			return
		}
	}
	s.Accounts = append(s.Accounts, acc)
	if makeActive || len(s.Accounts) == 1 {
		s.ActiveAccountID = acc.ID
	}
}

// RemoveAccount deletes an account by ID. Returns true if found and removed.
func (s *Storage) RemoveAccount(id string) bool {
	for i, acc := range s.Accounts {
		if acc.ID == id {
			s.Accounts = append(s.Accounts[:i], s.Accounts[i+1:]...)
			if s.ActiveAccountID == id {
				if len(s.Accounts) > 0 {
					s.ActiveAccountID = s.Accounts[0].ID
				} else {
					s.ActiveAccountID = ""
				}
			}
			return true
		}
	}
	return false
}

// UpdateAccount updates matching account fields by ID.
func (s *Storage) UpdateAccount(acc Account) bool {
	for i := range s.Accounts {
		if s.Accounts[i].ID == acc.ID {
			s.Accounts[i] = acc
			return true
		}
	}
	return false
}
