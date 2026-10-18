package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unsafe"
)

// Win32 API Definitions & Procs for Mouse Jiggler & Sleep Guard
var (
	procGetCursorPos            = user32DLL.NewProc("GetCursorPos")
	procSetCursorPos            = user32DLL.NewProc("SetCursorPos")
	procMouseEvent              = user32DLL.NewProc("mouse_event")
	procKeybdEvent              = user32DLL.NewProc("keybd_event")
	procGetLastInputInfo        = user32DLL.NewProc("GetLastInputInfo")
	procGetTickCount            = kernel32DLL.NewProc("GetTickCount")
	procSetThreadExecutionState = kernel32DLL.NewProc("SetThreadExecutionState")
)

const (
	ES_CONTINUOUS       = 0x80000000
	ES_SYSTEM_REQUIRED  = 0x00000001
	ES_DISPLAY_REQUIRED = 0x00000002

	MOUSEEVENTF_MOVE = 0x0001

	KEYEVENTF_KEYUP = 0x0002

	VK_SHIFT   = 0x10
	VK_CONTROL = 0x11
	VK_F15     = 0x7E // Invisible Ghost Key (resets idle timer without typing)
	VK_SCROLL  = 0x91 // Scroll Lock
)

type LASTINPUTINFO struct {
	CbSize uint32
	DwTime uint32
}

func getCursorPos() (int, int) {
	var pt POINT
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	return int(pt.X), int(pt.Y)
}

func setCursorPos(x, y int) {
	procSetCursorPos.Call(uintptr(x), uintptr(y))
}

func sendRelativeMouseMove(dx, dy int) {
	procMouseEvent.Call(MOUSEEVENTF_MOVE, uintptr(uint32(int32(dx))), uintptr(uint32(int32(dy))), 0, 0)
}

func sendKeyTap(vk int) {
	procKeybdEvent.Call(uintptr(vk), 0, 0, 0)
	time.Sleep(20 * time.Millisecond)
	procKeybdEvent.Call(uintptr(vk), 0, KEYEVENTF_KEYUP, 0)
}

func getIdleDurationSeconds() float64 {
	var lii LASTINPUTINFO
	lii.CbSize = uint32(unsafe.Sizeof(lii))
	ret, _, _ := procGetLastInputInfo.Call(uintptr(unsafe.Pointer(&lii)))
	if ret != 0 {
		nowMs, _, _ := procGetTickCount.Call()
		elapsedMs := (uint32(nowMs) - lii.DwTime)
		return float64(elapsedMs) / 1000.0
	}
	return 0.0
}

func setKeepAwake(enabled bool) {
	if enabled {
		procSetThreadExecutionState.Call(uintptr(ES_CONTINUOUS | ES_SYSTEM_REQUIRED | ES_DISPLAY_REQUIRED))
	} else {
		procSetThreadExecutionState.Call(uintptr(ES_CONTINUOUS))
	}
}

// JigglerConfig represents user configurable preferences for anti-idle
type JigglerConfig struct {
	Shortcut         string `json:"shortcut"`
	MouseEnabled     bool   `json:"mouse_enabled"`
	MouseIntervalSec int    `json:"mouse_interval_sec"`
	MouseMoveMode    string `json:"mouse_move_mode"` // "zen", "gentle", "orbit", "api_only"
	KeyEnabled       bool   `json:"key_enabled"`
	KeyIntervalSec   int    `json:"key_interval_sec"`
	KeyChoice        string `json:"key_choice"` // "f15", "shift", "control"
	IdleGuardEnabled bool   `json:"idle_guard_enabled"`
	IdleThresholdSec int    `json:"idle_threshold_sec"`
	KeepAwakeEnabled bool   `json:"keep_awake_enabled"`
}

func DefaultJigglerConfig() JigglerConfig {
	return JigglerConfig{
		Shortcut:         "Ctrl+Alt+J",
		MouseEnabled:     true,
		MouseIntervalSec: 10,
		MouseMoveMode:    "orbit",
		KeyEnabled:       true,
		KeyIntervalSec:   30,
		KeyChoice:        "f15",
		IdleGuardEnabled: false,
		IdleThresholdSec: 15,
		KeepAwakeEnabled: true,
	}
}

// JigglerStats represents runtime metrics
type JigglerStats struct {
	Running          bool
	TotalMouseMoves  int
	TotalKeyTaps     int
	NextMouseMoveSec int
	NextKeyTapSec    int
	CurrentIdleSec   float64
	LastActionMsg    string
}

// JigglerEngine supervises the background anti-idle worker
type JigglerEngine struct {
	config     JigglerConfig
	running    bool
	stopChan   chan struct{}
	stats      JigglerStats
	lock       sync.RWMutex
	configFile string
}

var (
	globalJiggler     *JigglerEngine
	globalJigglerOnce sync.Once
)

func GetJigglerEngine() *JigglerEngine {
	globalJigglerOnce.Do(func() {
		cfgPath := filepath.Join(GetManager().BaseDir, "jiggler_config.json")
		globalJiggler = NewJigglerEngine(cfgPath)
	})
	return globalJiggler
}

func NewJigglerEngine(configPath string) *JigglerEngine {
	je := &JigglerEngine{
		config:     DefaultJigglerConfig(),
		configFile: configPath,
	}
	je.LoadConfig()
	return je
}

func (je *JigglerEngine) LoadConfig() {
	data, err := os.ReadFile(je.configFile)
	if err == nil {
		var loaded JigglerConfig
		if err := json.Unmarshal(data, &loaded); err == nil {
			if loaded.MouseIntervalSec <= 0 {
				loaded.MouseIntervalSec = 30
			}
			if loaded.KeyIntervalSec <= 0 {
				loaded.KeyIntervalSec = 60
			}
			if loaded.IdleThresholdSec <= 0 {
				loaded.IdleThresholdSec = 15
			}
			if loaded.MouseMoveMode == "" {
				loaded.MouseMoveMode = "zen"
			}
			if loaded.KeyChoice == "" {
				loaded.KeyChoice = "f15"
			}
			if loaded.Shortcut == "" {
				loaded.Shortcut = "Ctrl+Alt+J"
			}
			je.config = loaded
		}
	}
}

func (je *JigglerEngine) SaveConfig() {
	je.lock.RLock()
	data, err := json.MarshalIndent(je.config, "", "  ")
	je.lock.RUnlock()
	if err == nil {
		_ = os.WriteFile(je.configFile, data, 0644)
	}
}

func (je *JigglerEngine) UpdateConfig(cfg JigglerConfig) {
	je.lock.Lock()
	je.config = cfg
	je.lock.Unlock()
	je.SaveConfig()
}

func (je *JigglerEngine) GetConfig() JigglerConfig {
	je.lock.RLock()
	defer je.lock.RUnlock()
	return je.config
}

func (je *JigglerEngine) IsRunning() bool {
	je.lock.RLock()
	defer je.lock.RUnlock()
	return je.running
}

func (je *JigglerEngine) GetStats() JigglerStats {
	je.lock.RLock()
	defer je.lock.RUnlock()
	s := je.stats
	s.Running = je.running
	s.CurrentIdleSec = getIdleDurationSeconds()
	return s
}

func (je *JigglerEngine) Start() {
	je.lock.Lock()
	if je.running {
		je.lock.Unlock()
		return
	}
	je.running = true
	je.stopChan = make(chan struct{})
	cfg := je.config
	je.stats.NextMouseMoveSec = cfg.MouseIntervalSec
	je.stats.NextKeyTapSec = cfg.KeyIntervalSec
	je.stats.LastActionMsg = "Jiggler started"
	je.lock.Unlock()

	if cfg.KeepAwakeEnabled {
		setKeepAwake(true)
	}

	go je.loop()
}

func (je *JigglerEngine) Stop() {
	je.lock.Lock()
	if !je.running {
		je.lock.Unlock()
		return
	}
	je.running = false
	close(je.stopChan)
	je.stats.LastActionMsg = "Jiggler stopped"
	je.lock.Unlock()

	setKeepAwake(false)
}

func (je *JigglerEngine) loop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-je.stopChan:
			return
		case <-ticker.C:
			je.tick()
		}
	}
}

func (je *JigglerEngine) tick() {
	je.lock.Lock()
	defer je.lock.Unlock()

	cfg := je.config
	idleSec := getIdleDurationSeconds()

	// Check User-Presence Guard ONLY if explicitly enabled
	if cfg.IdleGuardEnabled && idleSec < float64(cfg.IdleThresholdSec) {
		je.stats.LastActionMsg = fmt.Sprintf("Paused: User active (%.1fs < %ds)", idleSec, cfg.IdleThresholdSec)
		return
	}

	// 1. Mouse Jiggle Tick
	if cfg.MouseEnabled {
		je.stats.NextMouseMoveSec--
		if je.stats.NextMouseMoveSec <= 0 {
			je.performMouseJiggle(cfg.MouseMoveMode)
			je.stats.TotalMouseMoves++
			je.stats.NextMouseMoveSec = cfg.MouseIntervalSec
			je.stats.LastActionMsg = fmt.Sprintf("Mouse moved (%s)", cfg.MouseMoveMode)
		}
	}

	// 2. Keyboard Anti-Idle Tap Tick
	if cfg.KeyEnabled {
		je.stats.NextKeyTapSec--
		if je.stats.NextKeyTapSec <= 0 {
			je.performKeyTap(cfg.KeyChoice)
			je.stats.TotalKeyTaps++
			je.stats.NextKeyTapSec = cfg.KeyIntervalSec
			je.stats.LastActionMsg = fmt.Sprintf("Key tapped (%s)", cfg.KeyChoice)
		}
	}
}

func (je *JigglerEngine) performMouseJiggle(mode string) {
	origX, origY := getCursorPos()

	switch mode {
	case "zen":
		// Stealth 1-pixel micro-nudge (invisible to eyes, resets Windows idle timer)
		sendRelativeMouseMove(1, 0)
		time.Sleep(15 * time.Millisecond)
		sendRelativeMouseMove(-1, 0)

	case "gentle":
		// Clear visible back-and-forth nudge
		setCursorPos(origX+20, origY)
		time.Sleep(100 * time.Millisecond)
		setCursorPos(origX, origY)

	case "orbit":
		// Smooth circular loop returning to origin
		radius := 28.0
		steps := 20
		for i := 1; i <= steps; i++ {
			angle := (2.0 * math.Pi / float64(steps)) * float64(i)
			tx := origX + int(radius*math.Cos(angle)) - int(radius)
			ty := origY + int(radius*math.Sin(angle))
			setCursorPos(tx, ty)
			time.Sleep(20 * time.Millisecond)
		}
		setCursorPos(origX, origY)

	case "api_only":
		// Sleep prevention without moving mouse
		setKeepAwake(true)

	default:
		setCursorPos(origX+20, origY)
		time.Sleep(100 * time.Millisecond)
		setCursorPos(origX, origY)
	}
}

func (je *JigglerEngine) performKeyTap(choice string) {
	vk := VK_F15 // default ghost key
	switch choice {
	case "shift":
		vk = VK_SHIFT
	case "control":
		vk = VK_CONTROL
	case "scroll":
		vk = VK_SCROLL
	default:
		vk = VK_F15
	}
	sendKeyTap(vk)
}
