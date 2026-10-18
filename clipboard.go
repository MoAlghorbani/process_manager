package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// Win32 Clipboard Definitions
var (
	procGetClipboardSequenceNumber = user32DLL.NewProc("GetClipboardSequenceNumber")
	procOpenClipboard              = user32DLL.NewProc("OpenClipboard")
	procCloseClipboard             = user32DLL.NewProc("CloseClipboard")
	procGetClipboardData           = user32DLL.NewProc("GetClipboardData")
	procIsClipboardFormatAvailable = user32DLL.NewProc("IsClipboardFormatAvailable")
	procGlobalLock                 = kernel32DLL.NewProc("GlobalLock")
	procGlobalUnlock               = kernel32DLL.NewProc("GlobalUnlock")
)

const (
	CF_UNICODETEXT = 13
)

// readWin32ClipboardText extracts UTF-16 Unicode text from the Windows system clipboard.
func readWin32ClipboardText() (string, bool) {
	avail, _, _ := procIsClipboardFormatAvailable.Call(CF_UNICODETEXT)
	if avail == 0 {
		return "", false
	}

	// Retry briefly in case another process currently locks the clipboard
	var opened uintptr
	for i := 0; i < 3; i++ {
		opened, _, _ = procOpenClipboard.Call(0)
		if opened != 0 {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if opened == 0 {
		return "", false
	}
	defer procCloseClipboard.Call()

	hData, _, _ := procGetClipboardData.Call(CF_UNICODETEXT)
	if hData == 0 {
		return "", false
	}

	ptr, _, _ := procGlobalLock.Call(hData)
	if ptr == 0 {
		return "", false
	}
	defer procGlobalUnlock.Call(hData)

	u16 := (*[1 << 23]uint16)(unsafe.Pointer(ptr))
	length := 0
	for u16[length] != 0 {
		length++
		if length >= 1<<22 {
			break
		}
	}

	if length == 0 {
		return "", false
	}

	text := syscall.UTF16ToString(u16[:length])
	return text, true
}

// ClipboardItem represents a permanently recorded clipboard snippet.
type ClipboardItem struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	Pinned    bool      `json:"pinned"`
	Length    int       `json:"length"`
}

// ClipboardWatcherConfig holds preferences for the clipboard background watcher.
type ClipboardWatcherConfig struct {
	AutoSaveEnabled bool `json:"auto_save_enabled"`
	MaxItems        int  `json:"max_items"`
}

// ClipboardWatcher monitors system clipboard changes and persists snippets permanently.
type ClipboardWatcher struct {
	filePath   string
	items      []ClipboardItem
	config     ClipboardWatcherConfig
	lock       sync.RWMutex
	lastSeq    uint32
	lastClip   string
	running    bool
	stopChan   chan struct{}
	onNewClip  func(item ClipboardItem)
}

var (
	globalClipboardWatcher     *ClipboardWatcher
	globalClipboardWatcherOnce sync.Once
)

// GetClipboardWatcher returns the singleton instance of ClipboardWatcher.
func GetClipboardWatcher() *ClipboardWatcher {
	globalClipboardWatcherOnce.Do(func() {
		baseDir := GetManager().BaseDir
		storePath := filepath.Join(baseDir, "clipboard_history.json")
		globalClipboardWatcher = NewClipboardWatcher(storePath)
	})
	return globalClipboardWatcher
}

// NewClipboardWatcher creates and initializes the clipboard watcher.
func NewClipboardWatcher(filePath string) *ClipboardWatcher {
	cw := &ClipboardWatcher{
		filePath: filePath,
		items:    make([]ClipboardItem, 0),
		config: ClipboardWatcherConfig{
			AutoSaveEnabled: true,
			MaxItems:        300,
		},
		stopChan: make(chan struct{}),
	}
	cw.Load()
	return cw
}

// Load restores clipboard history and settings from disk.
func (cw *ClipboardWatcher) Load() {
	cw.lock.Lock()
	defer cw.lock.Unlock()

	data, err := os.ReadFile(cw.filePath)
	if err != nil {
		return
	}

	var wrapper struct {
		Config ClipboardWatcherConfig `json:"config"`
		Items  []ClipboardItem        `json:"items"`
	}

	if err := json.Unmarshal(data, &wrapper); err == nil {
		cw.config = wrapper.Config
		if cw.config.MaxItems <= 0 {
			cw.config.MaxItems = 300
		}
		cw.items = wrapper.Items
		if len(cw.items) > 0 {
			cw.lastClip = cw.items[0].Content
		}
	}
}

// Save persists clipboard history and settings to disk.
func (cw *ClipboardWatcher) Save() error {
	cw.lock.RLock()
	wrapper := struct {
		Config ClipboardWatcherConfig `json:"config"`
		Items  []ClipboardItem        `json:"items"`
	}{
		Config: cw.config,
		Items:  cw.items,
	}
	cw.lock.RUnlock()

	data, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(cw.filePath, data, 0600)
}

// SetOnNewClip registers a callback fired whenever a new clip is captured.
func (cw *ClipboardWatcher) SetOnNewClip(fn func(item ClipboardItem)) {
	cw.lock.Lock()
	defer cw.lock.Unlock()
	cw.onNewClip = fn
}

// Start begins background monitoring of Windows clipboard sequence numbers.
func (cw *ClipboardWatcher) Start() {
	cw.lock.Lock()
	if cw.running {
		cw.lock.Unlock()
		return
	}
	cw.running = true
	cw.stopChan = make(chan struct{})

	// Initialize sequence number so we don't duplicate on first tick unless changed
	seq, _, _ := procGetClipboardSequenceNumber.Call()
	cw.lastSeq = uint32(seq)
	cw.lock.Unlock()

	go cw.loop()
}

// Stop terminates the clipboard watcher background loop.
func (cw *ClipboardWatcher) Stop() {
	cw.lock.Lock()
	if !cw.running {
		cw.lock.Unlock()
		return
	}
	cw.running = false
	close(cw.stopChan)
	cw.lock.Unlock()
}

func (cw *ClipboardWatcher) loop() {
	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-cw.stopChan:
			return
		case <-ticker.C:
			cw.checkClipboard()
		}
	}
}

func (cw *ClipboardWatcher) checkClipboard() {
	cw.lock.RLock()
	enabled := cw.config.AutoSaveEnabled
	cw.lock.RUnlock()
	if !enabled {
		return
	}

	seqRet, _, _ := procGetClipboardSequenceNumber.Call()
	currentSeq := uint32(seqRet)

	cw.lock.RLock()
	lastSeq := cw.lastSeq
	cw.lock.RUnlock()

	if currentSeq == 0 || currentSeq == lastSeq {
		return
	}

	cw.lock.Lock()
	cw.lastSeq = currentSeq
	cw.lock.Unlock()

	text, ok := readWin32ClipboardText()
	if !ok {
		return
	}

	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return
	}

	cw.lock.Lock()
	if text == cw.lastClip {
		cw.lock.Unlock()
		return
	}
	cw.lastClip = text

	id := fmt.Sprintf("clip_%d", time.Now().UnixNano())
	item := ClipboardItem{
		ID:        id,
		Content:   text,
		Timestamp: time.Now(),
		Pinned:    false,
		Length:    len(text),
	}

	// Prepend to items list (newest first)
	cw.items = append([]ClipboardItem{item}, cw.items...)

	// Trim items beyond MaxItems, keeping all pinned items
	maxItems := cw.config.MaxItems
	if maxItems <= 0 {
		maxItems = 300
	}

	if len(cw.items) > maxItems {
		var trimmedItems []ClipboardItem
		unpinnedCount := 0
		for _, it := range cw.items {
			if it.Pinned {
				trimmedItems = append(trimmedItems, it)
			} else {
				if unpinnedCount < maxItems {
					trimmedItems = append(trimmedItems, it)
					unpinnedCount++
				}
			}
		}
		cw.items = trimmedItems
	}

	callback := cw.onNewClip
	cw.lock.Unlock()

	_ = cw.Save()

	if callback != nil {
		callback(item)
	}
}

// ToggleAutoSave enables or disables background clipboard capture.
func (cw *ClipboardWatcher) ToggleAutoSave() bool {
	cw.lock.Lock()
	cw.config.AutoSaveEnabled = !cw.config.AutoSaveEnabled
	newState := cw.config.AutoSaveEnabled
	cw.lock.Unlock()

	_ = cw.Save()
	return newState
}

// IsAutoSaveEnabled returns true if auto-saving is active.
func (cw *ClipboardWatcher) IsAutoSaveEnabled() bool {
	cw.lock.RLock()
	defer cw.lock.RUnlock()
	return cw.config.AutoSaveEnabled
}

// List returns clipboard items, optionally filtered by keyword and pinned-only flag.
func (cw *ClipboardWatcher) List(search string, pinnedOnly bool) []ClipboardItem {
	cw.lock.RLock()
	defer cw.lock.RUnlock()

	q := strings.ToLower(strings.TrimSpace(search))
	var result []ClipboardItem

	for _, item := range cw.items {
		if pinnedOnly && !item.Pinned {
			continue
		}
		if q == "" || strings.Contains(strings.ToLower(item.Content), q) {
			result = append(result, item)
		}
	}

	// Newest first, pinned on top
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Pinned != result[j].Pinned {
			return result[i].Pinned
		}
		return result[i].Timestamp.After(result[j].Timestamp)
	})

	return result
}

// TogglePin pins or unpins an item by ID.
func (cw *ClipboardWatcher) TogglePin(id string) bool {
	cw.lock.Lock()
	defer cw.lock.Unlock()

	for i, item := range cw.items {
		if item.ID == id {
			cw.items[i].Pinned = !cw.items[i].Pinned
			go cw.Save()
			return cw.items[i].Pinned
		}
	}
	return false
}

// Delete removes a specific clipboard snippet by ID.
func (cw *ClipboardWatcher) Delete(id string) bool {
	cw.lock.Lock()
	defer cw.lock.Unlock()

	idx := -1
	for i, item := range cw.items {
		if item.ID == id {
			idx = i
			break
		}
	}
	if idx >= 0 {
		cw.items = append(cw.items[:idx], cw.items[idx+1:]...)
		go cw.Save()
		return true
	}
	return false
}

// ClearUnpinned clears all non-pinned clipboard items.
func (cw *ClipboardWatcher) ClearUnpinned() int {
	cw.lock.Lock()
	defer cw.lock.Unlock()

	var pinnedOnly []ClipboardItem
	cleared := 0
	for _, item := range cw.items {
		if item.Pinned {
			pinnedOnly = append(pinnedOnly, item)
		} else {
			cleared++
		}
	}
	cw.items = pinnedOnly
	go cw.Save()
	return cleared
}

// TotalCount returns total recorded clips.
func (cw *ClipboardWatcher) TotalCount() int {
	cw.lock.RLock()
	defer cw.lock.RUnlock()
	return len(cw.items)
}
