package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/jchv/go-webview2"
)

var (
	user32Dll           = syscall.NewLazyDLL("user32.dll")
	procFindWindowW     = user32Dll.NewProc("FindWindowW")
	procSetParent       = user32Dll.NewProc("SetParent")
	procSetWindowLongW  = user32Dll.NewProc("SetWindowLongW")
	procGetWindowLongW  = user32Dll.NewProc("GetWindowLongW")
	procSetWindowPos    = user32Dll.NewProc("SetWindowPos")
	procGetClientRect   = user32Dll.NewProc("GetClientRect")
	procShowWindow      = user32Dll.NewProc("ShowWindow")
	procSetFocus        = user32Dll.NewProc("SetFocus")
	procIsWindow        = user32Dll.NewProc("IsWindow")
	procRedrawWindow    = user32Dll.NewProc("RedrawWindow")
)

const (
	gwlStyle        = ^uintptr(15) // -16
	wsChild         = 0x40000000
	wsVisible       = 0x10000000
	wsClipChildren  = 0x02000000
	wsClipSiblings  = 0x04000000
	swHide          = 0
	swShow          = 5
	swpNoZOrder     = 0x0004
	swpNoActivate   = 0x0010
	swpShowWindow   = 0x0040
	swpHideWindow   = 0x0080
	swpFrameChanged = 0x0020
	rdwInvalidate   = 0x0001
	rdwErase        = 0x0004
	rdwAllChildren  = 0x0080
	rdwUpdateNow    = 0x0100
	offscreenCoord  = uintptr(0xFFFF8300) // -32000 in Win32 signed int32
)

type winRect struct {
	Left, Top, Right, Bottom int32
}

// DodgeGameTabController manages the embedded WebView2 child window on demand.
type DodgeGameTabController struct {
	mu               sync.Mutex
	port             int
	fyneWindow       fyne.Window
	parentHwnd       uintptr
	childHwnd        uintptr
	wv               webview2.WebView
	isTabActive      bool
	isDestroyed      bool
	isMounted        bool
	lastRect         winRect
	statusLbl        *widget.Label
	startStopBtn     *widget.Button
	currentGameRoute string
	refreshCard      func()
}

// buildDodgeGameTab creates the Dodge Game Fyne tab with on-demand mounting.
func buildDodgeGameTab(w fyne.Window, a fyne.App, port int) (*container.TabItem, *DodgeGameTabController) {
	ctrl := &DodgeGameTabController{
		port:             port,
		fyneWindow:       w,
		currentGameRoute: "game",
		isMounted:        false,
	}

	// --- Top Toolbar ---
	titleLbl := widget.NewLabelWithStyle("🎮 Training Hub", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	ctrl.statusLbl = widget.NewLabelWithStyle("[🟢 Standby • 0% CPU • 0 MB]", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})

	openBrowserBtn := widget.NewButtonWithIcon("🌐 Open in Browser", theme.NavigateNextIcon(), func() {
		route := ctrl.GetCurrentRoute()
		gameUrl := fmt.Sprintf("http://localhost:%d/%s", port, route)
		go func() {
			_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", gameUrl).Start()
		}()
	})

	restartBtn := widget.NewButtonWithIcon("🔄 Restart", theme.ViewRefreshIcon(), func() {
		ctrl.Reload()
	})

	focusBtn := widget.NewButtonWithIcon("🎯 Focus", theme.ConfirmIcon(), func() {
		ctrl.Focus()
	})

	controlsTip := widget.NewLabelWithStyle(
		"[R-Click / WASD] Move  •  [F / Space] Flash  •  [E / Shift] Shield  •  [R] Restart",
		fyne.TextAlignTrailing,
		fyne.TextStyle{Italic: true},
	)

	var dodgeBtn *widget.Button
	var aimBtn *widget.Button

	dodgeBtn = widget.NewButton("🛡️ LoL Dodge", func() {
		ctrl.SwitchGame("game")
		dodgeBtn.Importance = widget.HighImportance
		aimBtn.Importance = widget.MediumImportance
		dodgeBtn.Refresh()
		aimBtn.Refresh()
		controlsTip.SetText("[R-Click / WASD] Move  •  [F / Space] Flash  •  [E / Shift] Shield  •  [R] Restart")
		if !ctrl.IsMounted() && ctrl.refreshCard != nil {
			ctrl.refreshCard()
		}
	})
	dodgeBtn.Importance = widget.HighImportance

	aimBtn = widget.NewButton("🎯 Valorant Aim", func() {
		ctrl.SwitchGame("aim")
		aimBtn.Importance = widget.HighImportance
		dodgeBtn.Importance = widget.MediumImportance
		aimBtn.Refresh()
		dodgeBtn.Refresh()
		controlsTip.SetText("[Click] Lock Aim  •  [Left-Click] Fire Vandal  •  [ESC] Settings & Sens  •  [R] Restart")
		if !ctrl.IsMounted() && ctrl.refreshCard != nil {
			ctrl.refreshCard()
		}
	})
	aimBtn.Importance = widget.MediumImportance

	// Toolbar Start / Stop Button
	startStopBtn := widget.NewButtonWithIcon("▶ Start Game", theme.MediaPlayIcon(), func() {
		if ctrl.IsMounted() {
			ctrl.StopGame()
		} else {
			ctrl.StartGame()
		}
	})
	startStopBtn.Importance = widget.HighImportance
	ctrl.startStopBtn = startStopBtn

	modes := []string{"Auto", "Iron", "Bronze", "Silver", "Gold", "Platinum", "Diamond", "Ascendant", "Immortal", "Radiant"}
	currentModeIdx := 0

	var modeBtn *widget.Button
	updateMode := func(idx int) {
		currentModeIdx = (idx + len(modes)) % len(modes)
		selectedMode := modes[currentModeIdx]
		modeBtn.SetText("🏆 " + selectedMode + " ⟳")
		ctrl.SetGameMode(selectedMode)
	}

	prevModeBtn := widget.NewButton("◀", func() {
		updateMode(currentModeIdx - 1)
	})

	modeBtn = widget.NewButton("🏆 Auto ⟳", func() {
		updateMode(currentModeIdx + 1)
	})

	nextModeBtn := widget.NewButton("▶", func() {
		updateMode(currentModeIdx + 1)
	})

	isIsometric := true
	var viewBtn *widget.Button
	viewBtn = widget.NewButton("📐 3D Iso", func() {
		isIsometric = !isIsometric
		if isIsometric {
			viewBtn.SetText("📐 3D Iso")
			ctrl.SetViewMode("isometric")
		} else {
			viewBtn.SetText("🧭 2D Top")
			ctrl.SetViewMode("topdown")
		}
	})

	toolbarRow := container.NewHBox(
		titleLbl,
		ctrl.statusLbl,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Game:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		dodgeBtn,
		aimBtn,
		widget.NewSeparator(),
		startStopBtn,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Rank:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		prevModeBtn,
		modeBtn,
		nextModeBtn,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Camera:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		viewBtn,
		widget.NewSeparator(),
		restartBtn,
		focusBtn,
		openBrowserBtn,
	)

	topContainer := container.NewVBox(
		container.NewHScroll(toolbarRow),
		controlsTip,
		widget.NewSeparator(),
	)

	// --- Tab Background / Standby Dashboard (Displayed when engine is NOT mounted) ---
	placeholderBg := canvas.NewRectangle(newCustomDarkTheme().Color(theme.ColorNameBackground, theme.VariantDark))

	standbyHeader := widget.NewLabelWithStyle("🎮 TRAINING HUB — ON DEMAND", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	standbyBadge := widget.NewLabelWithStyle("🍃 ZERO-RESOURCE STANDBY: Game engine is unmounted (0% CPU • 0 MB RAM)", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	gameTitleLbl := widget.NewLabelWithStyle("🛡️ League of Legends Skillshot Dodge Game", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	gameDescLbl := widget.NewLabelWithStyle("Sharpen your reflexes, juking, and flash positioning in 3D Isometric or 2D Top-Down mode.\nPractice against skillshot barrages across 9 competitive ranks (Iron to Challenger).", fyne.TextAlignCenter, fyne.TextStyle{})
	gameControlsLbl := widget.NewLabelWithStyle("🎮 Controls: [Right-Click / WASD] Move  •  [F / Space] Flash  •  [E / Shift] Shield  •  [R] Restart", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	bigLaunchBtn := widget.NewButtonWithIcon("▶  START LOL DODGE GAME", theme.MediaPlayIcon(), func() {
		ctrl.StartGame()
	})
	bigLaunchBtn.Importance = widget.HighImportance

	cardBrowserBtn := widget.NewButtonWithIcon("🌐 Open in Browser", theme.NavigateNextIcon(), func() {
		route := ctrl.GetCurrentRoute()
		gameUrl := fmt.Sprintf("http://localhost:%d/%s", port, route)
		go func() {
			_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", gameUrl).Start()
		}()
	})

	footerHint := widget.NewLabelWithStyle("💡 To preserve resources, the 3D game engine only starts when you click Start.\nPress [⏹ Stop Game] in the top toolbar anytime to unload the engine and free all memory.", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	updateStandbyCard := func() {
		route := ctrl.GetCurrentRoute()
		if route == "aim" {
			gameTitleLbl.SetText("🎯 VALORANT FPS Aim Trainer — The Range")
			gameDescLbl.SetText("First-Person Shooter 3D aim training calibrated to authentic Valorant sensitivity ($1\\text{ tick} = \\text{sens} \\times 0.07^\\circ$) and 103° FOV.\nFeatures 3D Vandal rifle with recoil kickback, metallic headshot 'Dink' audio, and 3 drills.")
			gameControlsLbl.SetText("🎮 Controls: [Click] Lock Aim  •  [Left-Click] Fire Vandal  •  [ESC] Settings & Sens  •  [R] Restart")
			bigLaunchBtn.SetText("▶  START VALORANT AIM TRAINER")
		} else {
			gameTitleLbl.SetText("🛡️ League of Legends Skillshot Dodge Game")
			gameDescLbl.SetText("Sharpen your reflexes, juking, and flash positioning in 3D Isometric or 2D Top-Down mode.\nPractice against skillshot barrages across 9 competitive ranks (Iron to Challenger).")
			gameControlsLbl.SetText("🎮 Controls: [Right-Click / WASD] Move  •  [F / Space] Flash  •  [E / Shift] Shield  •  [R] Restart")
			bigLaunchBtn.SetText("▶  START LOL DODGE GAME")
		}
		bigLaunchBtn.Refresh()
		gameTitleLbl.Refresh()
		gameDescLbl.Refresh()
		gameControlsLbl.Refresh()
	}
	ctrl.refreshCard = updateStandbyCard

	cardContent := container.NewVBox(
		gameTitleLbl,
		gameDescLbl,
		widget.NewSeparator(),
		gameControlsLbl,
		widget.NewSeparator(),
		container.NewCenter(container.NewHBox(bigLaunchBtn, cardBrowserBtn)),
		footerHint,
	)

	previewCard := widget.NewCard("", "", cardContent)

	centerBox := container.NewVBox(
		standbyHeader,
		standbyBadge,
		layout.NewSpacer(),
		previewCard,
		layout.NewSpacer(),
	)

	bodyPlaceholder := container.NewStack(placeholderBg, container.NewCenter(centerBox))

	// Combine into master tab layout
	mainLayout := container.NewBorder(topContainer, nil, nil, nil, bodyPlaceholder)

	tabItem := container.NewTabItemWithIcon("🎮 Training Hub", theme.MediaPlayIcon(), mainLayout)

	// Note: WebView2 is NOT launched automatically here! It remains unmounted until StartGame() is called.

	// Start monitor ticker to keep WebView2 positioned when window resizes (noop when unmounted)
	go ctrl.startPositionMonitor()

	return tabItem, ctrl
}

// IsMounted returns true if the WebView2 child window is currently active.
func (c *DodgeGameTabController) IsMounted() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.isMounted
}

// StartGame mounts and launches the WebView2 instance on demand.
func (c *DodgeGameTabController) StartGame() {
	c.mu.Lock()
	if c.isMounted || c.isDestroyed {
		c.mu.Unlock()
		return
	}
	c.isMounted = true
	c.mu.Unlock()

	fyne.Do(func() {
		if c.statusLbl != nil {
			c.statusLbl.SetText("[⏳ Launching Engine...]")
		}
		if c.startStopBtn != nil {
			c.startStopBtn.SetText("⏹ Stop Game")
			c.startStopBtn.Importance = widget.DangerImportance
			c.startStopBtn.SetIcon(theme.MediaStopIcon())
			c.startStopBtn.Refresh()
		}
	})

	go c.startWebView()
}

// StopGame unmounts and destroys the WebView2 instance to immediately free all CPU and memory.
func (c *DodgeGameTabController) StopGame() {
	c.mu.Lock()
	if !c.isMounted {
		c.mu.Unlock()
		return
	}
	wv := c.wv
	child := c.childHwnd
	parent := c.parentHwnd
	c.isMounted = false
	c.mu.Unlock()

	fyne.Do(func() {
		if c.statusLbl != nil {
			c.statusLbl.SetText("[Unloading Engine...]")
		}
	})

	if child != 0 {
		procSetWindowPos.Call(
			child,
			0,
			offscreenCoord,
			offscreenCoord,
			0,
			0,
			uintptr(swpNoZOrder|swpNoActivate|swpHideWindow),
		)
		procShowWindow.Call(child, uintptr(swHide))
	}

	if wv != nil {
		wv.Dispatch(func() {
			wv.Destroy()
		})
	}

	if parent != 0 {
		procRedrawWindow.Call(parent, 0, 0, uintptr(rdwInvalidate|rdwErase|rdwAllChildren|rdwUpdateNow))
	}

	if c.fyneWindow != nil && c.fyneWindow.Canvas() != nil {
		c.fyneWindow.Canvas().Refresh(c.fyneWindow.Content())
	}
}

func (c *DodgeGameTabController) startWebView() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[WebView2] Recovered from initialization panic: %v", r)
			fyne.Do(func() {
				if c.statusLbl != nil {
					c.statusLbl.SetText("[WebView2 Error - Use Browser]")
				}
				if c.startStopBtn != nil {
					c.startStopBtn.SetText("▶ Start Game")
					c.startStopBtn.Importance = widget.HighImportance
					c.startStopBtn.SetIcon(theme.MediaPlayIcon())
					c.startStopBtn.Refresh()
				}
			})
			c.mu.Lock()
			c.isMounted = false
			c.wv = nil
			c.childHwnd = 0
			c.mu.Unlock()
		}
	}()

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Locate main Fyne window HWND
	const winTitle = "Windows Process Manager & Service Supervisor (Native Go)"
	titleUTF16, _ := syscall.UTF16PtrFromString(winTitle)

	var parent uintptr
	for i := 0; i < 20; i++ {
		time.Sleep(100 * time.Millisecond)
		parent, _, _ = procFindWindowW.Call(0, uintptr(unsafe.Pointer(titleUTF16)))
		if parent != 0 {
			break
		}
	}

	if parent == 0 {
		fyne.Do(func() {
			c.statusLbl.SetText("[Error: Main window handle not found]")
			if c.startStopBtn != nil {
				c.startStopBtn.SetText("▶ Start Game")
				c.startStopBtn.Importance = widget.HighImportance
				c.startStopBtn.SetIcon(theme.MediaPlayIcon())
				c.startStopBtn.Refresh()
			}
		})
		c.mu.Lock()
		c.isMounted = false
		c.mu.Unlock()
		return
	}

	c.mu.Lock()
	c.parentHwnd = parent
	c.mu.Unlock()

	// Dedicated WebView2 user data directory in %LOCALAPPDATA%
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		localAppData = os.TempDir()
	}
	dataPath := filepath.Join(localAppData, "ProcessManager", "WebView2")
	_ = os.MkdirAll(dataPath, 0755)

	// Initialize WebView2
	opts := webview2.WebViewOptions{
		Debug:    false,
		DataPath: dataPath,
		WindowOptions: webview2.WindowOptions{
			Title:  "Training Hub Game Engine",
			Width:  800,
			Height: 600,
		},
	}

	wv := webview2.NewWithOptions(opts)
	if wv == nil {
		fyne.Do(func() {
			c.statusLbl.SetText("[Error: Failed to initialize WebView2]")
			if c.startStopBtn != nil {
				c.startStopBtn.SetText("▶ Start Game")
				c.startStopBtn.Importance = widget.HighImportance
				c.startStopBtn.SetIcon(theme.MediaPlayIcon())
				c.startStopBtn.Refresh()
			}
		})
		c.mu.Lock()
		c.isMounted = false
		c.mu.Unlock()
		return
	}

	child := uintptr(wv.Window())

	c.mu.Lock()
	if !c.isMounted || c.isDestroyed {
		c.mu.Unlock()
		wv.Destroy()
		return
	}
	c.wv = wv
	c.childHwnd = child
	active := c.isTabActive
	route := c.currentGameRoute
	c.mu.Unlock()

	// Reparent into Fyne window
	procSetParent.Call(child, parent)

	// Set Child Window Style
	procSetWindowLongW.Call(child, uintptr(gwlStyle), uintptr(wsChild|wsVisible|wsClipChildren|wsClipSiblings))

	// Navigate to selected route
	if route == "" {
		route = "game"
	}
	gameUrl := fmt.Sprintf("http://localhost:%d/%s", c.port, route)
	wv.Navigate(gameUrl)

	fyne.Do(func() {
		gameName := "LoL Dodge"
		if c.GetCurrentRoute() == "aim" {
			gameName = "Valorant Aim"
		}
		if c.statusLbl != nil {
			c.statusLbl.SetText(fmt.Sprintf("[⚡ Active: %s • Press Stop to Free Resources]", gameName))
		}
		if c.startStopBtn != nil {
			c.startStopBtn.SetText("⏹ Stop Game")
			c.startStopBtn.Importance = widget.DangerImportance
			c.startStopBtn.SetIcon(theme.MediaStopIcon())
			c.startStopBtn.Refresh()
		}
	})

	// Initial visibility: show if tab active, otherwise hide off-screen
	if active {
		procShowWindow.Call(child, uintptr(swShow))
		c.Reposition()
		c.Focus()
	} else {
		procSetWindowPos.Call(
			child,
			0,
			offscreenCoord,
			offscreenCoord,
			0,
			0,
			uintptr(swpNoZOrder|swpNoActivate|swpHideWindow),
		)
		procShowWindow.Call(child, uintptr(swHide))
	}

	// Message loop for WebView2 (blocks until wv.Destroy() is called)
	wv.Run()

	// Clean up after wv.Run() exits
	c.mu.Lock()
	c.wv = nil
	c.childHwnd = 0
	c.isMounted = false
	c.mu.Unlock()

	fyne.Do(func() {
		if c.statusLbl != nil {
			c.statusLbl.SetText("[🟢 Standby • 0% CPU • 0 MB]")
		}
		if c.startStopBtn != nil {
			c.startStopBtn.SetText("▶ Start Game")
			c.startStopBtn.Importance = widget.HighImportance
			c.startStopBtn.SetIcon(theme.MediaPlayIcon())
			c.startStopBtn.Refresh()
		}
		if c.refreshCard != nil {
			c.refreshCard()
		}
	})
}

// SetTabVisible handles tab switching to show/hide the WebView2 child window.
func (c *DodgeGameTabController) SetTabVisible(visible bool) {
	c.mu.Lock()
	c.isTabActive = visible
	mounted := c.isMounted
	child := c.childHwnd
	parent := c.parentHwnd
	c.mu.Unlock()

	if !mounted || child == 0 {
		return
	}

	if visible {
		procShowWindow.Call(child, uintptr(swShow))
		c.Reposition()
		c.Focus()
	} else {
		// Immediately move child window far off-screen and hide it so it cannot obscure other tabs
		procSetWindowPos.Call(
			child,
			0,
			offscreenCoord,
			offscreenCoord,
			0,
			0,
			uintptr(swpNoZOrder|swpNoActivate|swpHideWindow),
		)
		procShowWindow.Call(child, uintptr(swHide))
		if parent != 0 {
			procRedrawWindow.Call(parent, 0, 0, uintptr(rdwInvalidate|rdwErase|rdwAllChildren|rdwUpdateNow))
		}
		if c.fyneWindow != nil && c.fyneWindow.Canvas() != nil {
			c.fyneWindow.Canvas().Refresh(c.fyneWindow.Content())
		}
	}
}

// Reposition calculates the exact client coordinates and updates the child window geometry.
func (c *DodgeGameTabController) Reposition() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.childHwnd == 0 || c.parentHwnd == 0 || !c.isTabActive || !c.isMounted || c.isDestroyed {
		return
	}

	var rc winRect
	ret, _, _ := procGetClientRect.Call(c.parentHwnd, uintptr(unsafe.Pointer(&rc)))
	if ret == 0 {
		return
	}

	c.lastRect = rc

	// Determine DPI scale factor
	scale := float32(1.0)
	if c.fyneWindow != nil && c.fyneWindow.Canvas() != nil {
		scale = c.fyneWindow.Canvas().Scale()
		if scale <= 0 {
			scale = 1.0
		}
	}

	topOffset := int32(float32(100) * scale)

	x := int32(0)
	y := topOffset
	width := rc.Right
	height := rc.Bottom - topOffset
	if height < 100 {
		height = 100
	}

	procSetWindowPos.Call(
		c.childHwnd,
		0,
		uintptr(x),
		uintptr(y),
		uintptr(width),
		uintptr(height),
		uintptr(swpNoZOrder|swpNoActivate|swpShowWindow|swpFrameChanged),
	)
}

// GetCurrentRoute returns the active game route ("game" or "aim").
func (c *DodgeGameTabController) GetCurrentRoute() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.currentGameRoute == "" {
		return "game"
	}
	return c.currentGameRoute
}

// SwitchGame navigates between Dodge Training ("/game") and Aim Trainer ("/aim").
func (c *DodgeGameTabController) SwitchGame(gameType string) {
	c.mu.Lock()
	route := "game"
	if gameType == "aim" || gameType == "🎯 Aim Trainer" || gameType == "🎯 Aim" || gameType == "🎯 Valorant Aim" {
		route = "aim"
	}
	c.currentGameRoute = route
	wv := c.wv
	port := c.port
	mounted := c.isMounted
	c.mu.Unlock()

	if mounted && wv != nil {
		targetUrl := fmt.Sprintf("http://localhost:%d/%s", port, route)
		wv.Dispatch(func() {
			wv.Navigate(targetUrl)
		})
		fyne.Do(func() {
			gameName := "LoL Dodge"
			if route == "aim" {
				gameName = "Valorant Aim"
			}
			if c.statusLbl != nil {
				c.statusLbl.SetText(fmt.Sprintf("[⚡ Active: %s • Press Stop to Free Resources]", gameName))
			}
		})
	}
}

// Reload restarts the active training game inside WebView2.
func (c *DodgeGameTabController) Reload() {
	c.mu.Lock()
	route := c.currentGameRoute
	if route == "" {
		route = "game"
	}
	wv := c.wv
	port := c.port
	mounted := c.isMounted
	c.mu.Unlock()

	if mounted && wv != nil {
		gameUrl := fmt.Sprintf("http://localhost:%d/%s", port, route)
		wv.Dispatch(func() {
			wv.Navigate(gameUrl)
		})
	} else if !mounted {
		c.StartGame()
	}
}

// SetGameMode switches the game difficulty mode (Auto, Iron, Bronze, Gold, to Challenger).
func (c *DodgeGameTabController) SetGameMode(mode string) {
	c.mu.Lock()
	wv := c.wv
	c.mu.Unlock()

	if wv != nil {
		wv.Dispatch(func() {
			wv.Eval(fmt.Sprintf("if (window.setGameMode) window.setGameMode(%q);", mode))
		})
	}
}

// SetViewMode switches camera between Isometric 3D and 2D Top-Down.
func (c *DodgeGameTabController) SetViewMode(mode string) {
	c.mu.Lock()
	wv := c.wv
	c.mu.Unlock()

	if wv != nil {
		wv.Dispatch(func() {
			wv.Eval(fmt.Sprintf("if (window.setViewMode) window.setViewMode(%q);", mode))
		})
	}
}

// ToggleViewMode toggles between isometric and top-down camera views.
func (c *DodgeGameTabController) ToggleViewMode() {
	c.mu.Lock()
	wv := c.wv
	c.mu.Unlock()

	if wv != nil {
		wv.Dispatch(func() {
			wv.Eval("if (window.toggleViewMode) window.toggleViewMode();")
		})
	}
}

// Focus focuses the WebView2 window so keyboard input immediately responds.
func (c *DodgeGameTabController) Focus() {
	c.mu.Lock()
	child := c.childHwnd
	mounted := c.isMounted
	c.mu.Unlock()

	if mounted && child != 0 {
		procSetFocus.Call(child)
	}
}

// Destroy cleans up the WebView2 window.
func (c *DodgeGameTabController) Destroy() {
	c.mu.Lock()
	c.isDestroyed = true
	wv := c.wv
	c.mu.Unlock()

	if wv != nil {
		wv.Dispatch(func() {
			wv.Destroy()
		})
	}
}

func (c *DodgeGameTabController) startPositionMonitor() {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		active := c.isTabActive
		mounted := c.isMounted
		child := c.childHwnd
		parent := c.parentHwnd
		destroyed := c.isDestroyed
		last := c.lastRect
		c.mu.Unlock()

		if destroyed {
			return
		}

		if active && mounted && child != 0 && parent != 0 {
			var rc winRect
			ret, _, _ := procGetClientRect.Call(parent, uintptr(unsafe.Pointer(&rc)))
			if ret != 0 {
				if rc.Right != last.Right || rc.Bottom != last.Bottom {
					c.Reposition()
				}
			}
		}
	}
}
