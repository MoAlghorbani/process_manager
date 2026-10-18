package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// CredentialItem represents an account or secret stored in the vault.
type CredentialItem struct {
	ID        string    `json:"id"`
	Platform  string    `json:"platform"`
	Username  string    `json:"username"`
	Password  string    `json:"password"`
	URL       string    `json:"url,omitempty"`
	Notes     string    `json:"notes,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CredentialStore provides thread-safe persistent CRUD operations for credentials.
type CredentialStore struct {
	filePath string
	items    map[string]CredentialItem
	lock     sync.RWMutex
}

var (
	globalCredStore     *CredentialStore
	globalCredStoreOnce sync.Once
)

// GetCredentialStore returns the singleton instance of CredentialStore.
func GetCredentialStore() *CredentialStore {
	globalCredStoreOnce.Do(func() {
		baseDir := GetManager().BaseDir
		storePath := filepath.Join(baseDir, "credentials.json")
		globalCredStore = NewCredentialStore(storePath)
	})
	return globalCredStore
}

// NewCredentialStore initializes and loads a CredentialStore.
func NewCredentialStore(filePath string) *CredentialStore {
	cs := &CredentialStore{
		filePath: filePath,
		items:    make(map[string]CredentialItem),
	}
	cs.Load()
	return cs
}

// Load reads and unmarshals credentials from the JSON file.
func (cs *CredentialStore) Load() {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	data, err := os.ReadFile(cs.filePath)
	if err != nil {
		return
	}

	var list []CredentialItem
	if err := json.Unmarshal(data, &list); err != nil {
		return
	}

	cs.items = make(map[string]CredentialItem)
	for _, item := range list {
		if item.ID != "" {
			cs.items[item.ID] = item
		}
	}
}

// Save marshals and writes credentials to the JSON file.
func (cs *CredentialStore) Save() error {
	cs.lock.RLock()
	list := make([]CredentialItem, 0, len(cs.items))
	for _, item := range cs.items {
		list = append(list, item)
	}
	cs.lock.RUnlock()

	// Sort by Platform then Username
	sort.Slice(list, func(i, j int) bool {
		if strings.EqualFold(list[i].Platform, list[j].Platform) {
			return strings.ToLower(list[i].Username) < strings.ToLower(list[j].Username)
		}
		return strings.ToLower(list[i].Platform) < strings.ToLower(list[j].Platform)
	})

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(cs.filePath, data, 0600)
}

// Add creates a new credential item.
func (cs *CredentialStore) Add(platform, username, password, url, notes string) CredentialItem {
	cs.lock.Lock()
	id := fmt.Sprintf("cred_%d", time.Now().UnixNano())
	item := CredentialItem{
		ID:        id,
		Platform:  strings.TrimSpace(platform),
		Username:  strings.TrimSpace(username),
		Password:  password,
		URL:       strings.TrimSpace(url),
		Notes:     strings.TrimSpace(notes),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	cs.items[id] = item
	cs.lock.Unlock()

	_ = cs.Save()
	return item
}

// Update updates an existing credential item.
func (cs *CredentialStore) Update(id, platform, username, password, url, notes string) bool {
	cs.lock.Lock()
	item, exists := cs.items[id]
	if !exists {
		cs.lock.Unlock()
		return false
	}

	item.Platform = strings.TrimSpace(platform)
	item.Username = strings.TrimSpace(username)
	item.Password = password
	item.URL = strings.TrimSpace(url)
	item.Notes = strings.TrimSpace(notes)
	item.UpdatedAt = time.Now()

	cs.items[id] = item
	cs.lock.Unlock()

	_ = cs.Save()
	return true
}

// Delete removes a credential item by ID.
func (cs *CredentialStore) Delete(id string) bool {
	cs.lock.Lock()
	if _, exists := cs.items[id]; !exists {
		cs.lock.Unlock()
		return false
	}
	delete(cs.items, id)
	cs.lock.Unlock()

	_ = cs.Save()
	return true
}

// Get retrieves a credential by ID.
func (cs *CredentialStore) Get(id string) (CredentialItem, bool) {
	cs.lock.RLock()
	defer cs.lock.RUnlock()
	item, ok := cs.items[id]
	return item, ok
}

// List returns all credentials, optionally filtered by a search keyword.
func (cs *CredentialStore) List(search string) []CredentialItem {
	cs.lock.RLock()
	defer cs.lock.RUnlock()

	q := strings.ToLower(strings.TrimSpace(search))
	var results []CredentialItem

	for _, item := range cs.items {
		if q == "" ||
			strings.Contains(strings.ToLower(item.Platform), q) ||
			strings.Contains(strings.ToLower(item.Username), q) ||
			strings.Contains(strings.ToLower(item.URL), q) ||
			strings.Contains(strings.ToLower(item.Notes), q) {
			results = append(results, item)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if strings.EqualFold(results[i].Platform, results[j].Platform) {
			return strings.ToLower(results[i].Username) < strings.ToLower(results[j].Username)
		}
		return strings.ToLower(results[i].Platform) < strings.ToLower(results[j].Platform)
	})

	return results
}

// TotalCount returns the total number of stored credentials.
func (cs *CredentialStore) TotalCount() int {
	cs.lock.RLock()
	defer cs.lock.RUnlock()
	return len(cs.items)
}

// GeneratePassword creates a cryptographically secure random password.
func GeneratePassword(length int, includeSymbols bool) string {
	if length < 8 {
		length = 16
	}
	if length > 64 {
		length = 64
	}

	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	const digits = "0123456789"
	const symbols = "!@#$%^&*()-_=+[]{}<>~"

	charset := letters + digits
	if includeSymbols {
		charset += symbols
	}

	result := make([]byte, length)
	charsetLen := big.NewInt(int64(len(charset)))

	for i := 0; i < length; i++ {
		num, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			result[i] = charset[i%len(charset)]
		} else {
			result[i] = charset[num.Int64()]
		}
	}

	return string(result)
}
