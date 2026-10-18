package main

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

var (
	user32DLL              = syscall.NewLazyDLL("user32.dll")
	kernel32DLL            = syscall.NewLazyDLL("kernel32.dll")
	procGetCurrentThreadId = kernel32DLL.NewProc("GetCurrentThreadId")
	procRegisterHotKey     = user32DLL.NewProc("RegisterHotKey")
	procUnregisterHotKey   = user32DLL.NewProc("UnregisterHotKey")
	procGetMessageW        = user32DLL.NewProc("GetMessageW")
	procPeekMessageW       = user32DLL.NewProc("PeekMessageW")
	procPostThreadMessageW = user32DLL.NewProc("PostThreadMessageW")
)

const (
	WM_HOTKEY    = 0x0312
	WM_USER      = 0x0400
	WM_RELOAD_HK = WM_USER + 101
	WM_STOP_HK   = WM_USER + 102

	MOD_ALT      = 0x0001
	MOD_CONTROL  = 0x0002
	MOD_SHIFT    = 0x0004
	MOD_WIN      = 0x0008
	MOD_NOREPEAT = 0x4000
)

type POINT struct {
	X, Y int32
}

type MSG struct {
	Hwnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       POINT
	LPrivate uint32
}

// parseGlobalShortcut translates user strings like "Ctrl+Alt+1", "Ctrl+Shift+R", "F5" to Win32 modifiers and Virtual Key.
func parseGlobalShortcut(shortcutStr string) (uint32, uint32, error) {
	raw := strings.TrimSpace(shortcutStr)
	if raw == "" {
		return 0, 0, fmt.Errorf("empty shortcut")
	}

	parts := strings.Split(raw, "+")
	var mod uint32 = MOD_NOREPEAT
	var vk uint32

	for _, p := range parts {
		part := strings.TrimSpace(strings.ToLower(p))
		switch part {
		case "ctrl", "control":
			mod |= MOD_CONTROL
		case "shift":
			mod |= MOD_SHIFT
		case "alt":
			mod |= MOD_ALT
		case "super", "win", "cmd":
			mod |= MOD_WIN
		default:
			u := strings.ToUpper(part)
			if len(u) == 1 && u[0] >= '0' && u[0] <= '9' {
				vk = uint32(u[0])
			} else if len(u) == 1 && u[0] >= 'A' && u[0] <= 'Z' {
				vk = uint32(u[0])
			} else if strings.HasPrefix(u, "F") && len(u) >= 2 {
				switch u {
				case "F1":
					vk = 0x70
				case "F2":
					vk = 0x71
				case "F3":
					vk = 0x72
				case "F4":
					vk = 0x73
				case "F5":
					vk = 0x74
				case "F6":
					vk = 0x75
				case "F7":
					vk = 0x76
				case "F8":
					vk = 0x77
				case "F9":
					vk = 0x78
				case "F10":
					vk = 0x79
				case "F11":
					vk = 0x7A
				case "F12":
					vk = 0x7B
				}
			} else if u == "SPACE" {
				vk = 0x20
			} else if u == "RETURN" || u == "ENTER" {
				vk = 0x0D
			} else if u == "TAB" {
				vk = 0x09
			} else if u == "ESC" || u == "ESCAPE" {
				vk = 0x1B
			} else if u == "DEL" || u == "DELETE" {
				vk = 0x2E
			} else if u == "BACKSPACE" {
				vk = 0x08
			} else if u == "UP" {
				vk = 0x26
			} else if u == "DOWN" {
				vk = 0x28
			} else if u == "LEFT" {
				vk = 0x25
			} else if u == "RIGHT" {
				vk = 0x27
			}
		}
	}

	if vk == 0 {
		return 0, 0, fmt.Errorf("no valid key specified in shortcut: %s", shortcutStr)
	}

	// Safety check: Alphanumeric keys without a modifier (Ctrl, Alt, Win) would block typing across Windows.
	// Only Function keys (F1-F12) are allowed without a modifier key.
	hasModifier := (mod & (MOD_CONTROL | MOD_ALT | MOD_SHIFT | MOD_WIN)) != 0
	isFunctionKey := vk >= 0x70 && vk <= 0x7B
	if !hasModifier && !isFunctionKey {
		return 0, 0, fmt.Errorf("global shortcut '%s' requires at least one modifier key (Ctrl, Alt, Shift, or Win)", shortcutStr)
	}

	return mod, vk, nil
}

// GlobalHotkeyManager manages system-wide Windows hotkeys that work from any application.
type GlobalHotkeyManager struct {
	pm          *ProcessManager
	threadID    uintptr
	threadReady chan bool
	lock        sync.Mutex
	hotkeyMap   map[int]string // id -> serviceName
	onAction    func(serviceName string, actionMsg string)
	running     bool
}

var (
	globalHK     *GlobalHotkeyManager
	globalHKOnce sync.Once
)

func GetGlobalHotkeyManager(pm *ProcessManager) *GlobalHotkeyManager {
	globalHKOnce.Do(func() {
		globalHK = &GlobalHotkeyManager{
			pm:          pm,
			threadReady: make(chan bool, 1),
			hotkeyMap:   make(map[int]string),
		}
	})
	return globalHK
}

// Start launches the dedicated background message pump for system-wide Windows hotkeys.
func (ghm *GlobalHotkeyManager) Start(onAction func(serviceName string, actionMsg string)) {
	ghm.lock.Lock()
	if ghm.running {
		ghm.onAction = onAction
		ghm.lock.Unlock()
		ghm.Reload()
		return
	}
	ghm.running = true
	ghm.onAction = onAction
	ghm.lock.Unlock()

	go ghm.runLoop()
	<-ghm.threadReady
}

func (ghm *GlobalHotkeyManager) runLoop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	tid, _, _ := procGetCurrentThreadId.Call()
	ghm.threadID = tid

	// Force creation of Win32 message queue for this thread
	var msg MSG
	procPeekMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 0)

	ghm.threadReady <- true

	registeredIDs := make([]int, 0)
	lastRegisteredMap := make(map[string]string)

	registerAll := func() {
		svcs := ghm.pm.GetSummary()["services"]
		list, ok := svcs.([]map[string]interface{})
		if !ok {
			return
		}

		currentMap := make(map[string]string)
		for _, s := range list {
			sName := fmt.Sprintf("%v", s["name"])
			sShortcut := ""
			if sc, ok := s["shortcut"].(string); ok {
				sShortcut = strings.TrimSpace(sc)
			}
			if sShortcut != "" && sShortcut != "<nil>" {
				currentMap[sName] = sShortcut
			}
		}

		// Register Jiggler Hotkey if configured
		jigglerSC := strings.TrimSpace(GetJigglerEngine().GetConfig().Shortcut)
		if jigglerSC != "" && jigglerSC != "<nil>" {
			currentMap["__mouse_jiggler__"] = jigglerSC
		}

		// If registrations are already active and shortcuts haven't changed, skip
		if len(registeredIDs) > 0 && len(currentMap) == len(lastRegisteredMap) {
			same := true
			for k, v := range currentMap {
				if lastRegisteredMap[k] != v {
					same = false
					break
				}
			}
			if same {
				return
			}
		}

		// Unregister previous hotkeys
		for _, id := range registeredIDs {
			procUnregisterHotKey.Call(0, uintptr(id))
		}
		registeredIDs = nil
		lastRegisteredMap = currentMap

		ghm.lock.Lock()
		ghm.hotkeyMap = make(map[int]string)
		ghm.lock.Unlock()

		currentID := 1000
		for _, s := range list {
			sName := fmt.Sprintf("%v", s["name"])
			sShortcut := ""
			if sc, ok := s["shortcut"].(string); ok {
				sShortcut = strings.TrimSpace(sc)
			}
			if sShortcut == "" || sShortcut == "<nil>" {
				continue
			}

			mod, vk, err := parseGlobalShortcut(sShortcut)
			if err != nil {
				continue
			}

			currentID++
			ret, _, _ := procRegisterHotKey.Call(0, uintptr(currentID), uintptr(mod), uintptr(vk))
			if ret != 0 {
				registeredIDs = append(registeredIDs, currentID)
				ghm.lock.Lock()
				ghm.hotkeyMap[currentID] = sName
				ghm.lock.Unlock()
			} else {
				fmt.Printf("[GlobalHotkey] Warning: Could not register global shortcut '%s' for '%s' (may already be reserved)\n", sShortcut, sName)
			}
		}

		// Register Mouse Jiggler global shortcut
		if jigglerSC != "" && jigglerSC != "<nil>" {
			mod, vk, err := parseGlobalShortcut(jigglerSC)
			if err == nil {
				currentID++
				ret, _, _ := procRegisterHotKey.Call(0, uintptr(currentID), uintptr(mod), uintptr(vk))
				if ret != 0 {
					registeredIDs = append(registeredIDs, currentID)
					ghm.lock.Lock()
					ghm.hotkeyMap[currentID] = "__mouse_jiggler__"
					ghm.lock.Unlock()
				} else {
					fmt.Printf("[GlobalHotkey] Warning: Could not register global shortcut '%s' for 'Mouse Jiggler' (may already be reserved)\n", jigglerSC)
				}
			}
		}
	}

	registerAll()

	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if ret == 0 || int32(ret) == -1 {
			break
		}

		if msg.Message == WM_HOTKEY {
			hkID := int(msg.WParam)
			ghm.lock.Lock()
			name, exists := ghm.hotkeyMap[hkID]
			callback := ghm.onAction
			ghm.lock.Unlock()

			if exists {
				if name == "__mouse_jiggler__" {
					je := GetJigglerEngine()
					actionMsg := ""
					if je.IsRunning() {
						je.Stop()
						actionMsg = "Mouse Jiggler: Stopped"
					} else {
						je.Start()
						actionMsg = "Mouse Jiggler: Started"
					}
					if callback != nil {
						callback("Mouse Jiggler", actionMsg)
					}
					continue
				}

				go func(targetName string) {
					ghm.pm.lock.RLock()
					ms, ok := ghm.pm.services[targetName]
					isRunning := false
					if ok {
						isRunning = ms.IsRunning()
					}
					ghm.pm.lock.RUnlock()

					actionMsg := ""
					if isRunning {
						_ = ghm.pm.StopService(targetName)
						actionMsg = fmt.Sprintf("Stopped '%s'", targetName)
					} else {
						_ = ghm.pm.StartService(targetName)
						actionMsg = fmt.Sprintf("Started '%s'", targetName)
					}

					if callback != nil {
						callback(targetName, actionMsg)
					}
				}(name)
			}
		} else if msg.Message == WM_RELOAD_HK {
			registerAll()
		} else if msg.Message == WM_STOP_HK {
			for _, id := range registeredIDs {
				procUnregisterHotKey.Call(0, uintptr(id))
			}
			break
		}
	}
}

// Reload signals the hotkey thread to unregister and re-register hotkeys with updated configuration.
func (ghm *GlobalHotkeyManager) Reload() {
	ghm.lock.Lock()
	tid := ghm.threadID
	ghm.lock.Unlock()

	if tid != 0 {
		procPostThreadMessageW.Call(tid, WM_RELOAD_HK, 0, 0)
	}
}

// Stop terminates the global hotkey message loop and cleans up registrations.
func (ghm *GlobalHotkeyManager) Stop() {
	ghm.lock.Lock()
	tid := ghm.threadID
	ghm.running = false
	ghm.lock.Unlock()

	if tid != 0 {
		procPostThreadMessageW.Call(tid, WM_STOP_HK, 0, 0)
	}
}
