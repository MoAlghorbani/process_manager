package main

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// parseShortcut parses a shortcut string like "Ctrl+1", "Ctrl+Shift+R", "F5" into a desktop.CustomShortcut.
func parseShortcut(shortcutStr string) (*desktop.CustomShortcut, error) {
	raw := strings.TrimSpace(shortcutStr)
	if raw == "" {
		return nil, fmt.Errorf("empty shortcut")
	}

	parts := strings.Split(raw, "+")
	var mod fyne.KeyModifier
	var keyName fyne.KeyName

	for _, p := range parts {
		part := strings.TrimSpace(strings.ToLower(p))
		switch part {
		case "ctrl", "control":
			mod |= fyne.KeyModifierControl
		case "shift":
			mod |= fyne.KeyModifierShift
		case "alt":
			mod |= fyne.KeyModifierAlt
		case "super", "win", "cmd":
			mod |= fyne.KeyModifierSuper
		default:
			switch strings.ToUpper(part) {
			case "0":
				keyName = fyne.Key0
			case "1":
				keyName = fyne.Key1
			case "2":
				keyName = fyne.Key2
			case "3":
				keyName = fyne.Key3
			case "4":
				keyName = fyne.Key4
			case "5":
				keyName = fyne.Key5
			case "6":
				keyName = fyne.Key6
			case "7":
				keyName = fyne.Key7
			case "8":
				keyName = fyne.Key8
			case "9":
				keyName = fyne.Key9
			case "A":
				keyName = fyne.KeyA
			case "B":
				keyName = fyne.KeyB
			case "C":
				keyName = fyne.KeyC
			case "D":
				keyName = fyne.KeyD
			case "E":
				keyName = fyne.KeyE
			case "F":
				keyName = fyne.KeyF
			case "G":
				keyName = fyne.KeyG
			case "H":
				keyName = fyne.KeyH
			case "I":
				keyName = fyne.KeyI
			case "J":
				keyName = fyne.KeyJ
			case "K":
				keyName = fyne.KeyK
			case "L":
				keyName = fyne.KeyL
			case "M":
				keyName = fyne.KeyM
			case "N":
				keyName = fyne.KeyN
			case "O":
				keyName = fyne.KeyO
			case "P":
				keyName = fyne.KeyP
			case "Q":
				keyName = fyne.KeyQ
			case "R":
				keyName = fyne.KeyR
			case "S":
				keyName = fyne.KeyS
			case "T":
				keyName = fyne.KeyT
			case "U":
				keyName = fyne.KeyU
			case "V":
				keyName = fyne.KeyV
			case "W":
				keyName = fyne.KeyW
			case "X":
				keyName = fyne.KeyX
			case "Y":
				keyName = fyne.KeyY
			case "Z":
				keyName = fyne.KeyZ
			case "F1":
				keyName = fyne.KeyF1
			case "F2":
				keyName = fyne.KeyF2
			case "F3":
				keyName = fyne.KeyF3
			case "F4":
				keyName = fyne.KeyF4
			case "F5":
				keyName = fyne.KeyF5
			case "F6":
				keyName = fyne.KeyF6
			case "F7":
				keyName = fyne.KeyF7
			case "F8":
				keyName = fyne.KeyF8
			case "F9":
				keyName = fyne.KeyF9
			case "F10":
				keyName = fyne.KeyF10
			case "F11":
				keyName = fyne.KeyF11
			case "F12":
				keyName = fyne.KeyF12
			case "SPACE":
				keyName = fyne.KeySpace
			case "RETURN", "ENTER":
				keyName = fyne.KeyReturn
			case "TAB":
				keyName = fyne.KeyTab
			case "ESC", "ESCAPE":
				keyName = fyne.KeyEscape
			case "DEL", "DELETE":
				keyName = fyne.KeyDelete
			case "BACKSPACE":
				keyName = fyne.KeyBackspace
			default:
				return nil, fmt.Errorf("unknown key: %s", p)
			}
		}
	}

	if keyName == "" {
		return nil, fmt.Errorf("no key specified in shortcut: %s", shortcutStr)
	}

	return &desktop.CustomShortcut{
		KeyName:  keyName,
		Modifier: mod,
	}, nil
}

func getServicesList(pm *ProcessManager) []map[string]interface{} {
	sum := pm.GetSummary()
	if svcs, ok := sum["services"].([]map[string]interface{}); ok {
		return svcs
	}
	return nil
}

type customDarkTheme struct {
	fyne.Theme
}

func newCustomDarkTheme() fyne.Theme {
	return &customDarkTheme{Theme: theme.DarkTheme()}
}

func (t *customDarkTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameForeground:
		return color.RGBA{R: 248, G: 250, B: 252, A: 255} // Ultra-crisp bright white (#f8fafc)
	case theme.ColorNameDisabled:
		return color.RGBA{R: 160, G: 174, B: 192, A: 255} // High-contrast readable slate-gray (#a0aec0)
	case theme.ColorNameInputBackground:
		return color.RGBA{R: 11, G: 15, B: 25, A: 255} // Deep terminal dark slate (#0b0f19)
	case theme.ColorNameBackground:
		return color.RGBA{R: 9, G: 13, B: 22, A: 255} // Deep dark background
	case theme.ColorNamePlaceHolder:
		return color.RGBA{R: 100, G: 116, B: 139, A: 255} // Slate-500
	case theme.ColorNameSelection:
		return color.RGBA{R: 2, G: 132, B: 199, A: 140} // Sky blue highlight
	default:
		return t.Theme.Color(name, variant)
	}
}

// readOnlyTerminalEntry is a high-contrast selectable and copyable terminal viewer that blocks editing.
type readOnlyTerminalEntry struct {
	widget.Entry
}

func newReadOnlyTerminalEntry() *readOnlyTerminalEntry {
	e := &readOnlyTerminalEntry{}
	e.MultiLine = true
	e.Wrapping = fyne.TextWrapWord
	e.TextStyle = fyne.TextStyle{Monospace: true}
	e.ExtendBaseWidget(e)
	return e
}

func (e *readOnlyTerminalEntry) TypedRune(rune) {}

func (e *readOnlyTerminalEntry) TypedKey(key *fyne.KeyEvent) {
	switch key.Name {
	case fyne.KeyUp, fyne.KeyDown, fyne.KeyLeft, fyne.KeyRight,
		fyne.KeyPageUp, fyne.KeyPageDown, fyne.KeyHome, fyne.KeyEnd:
		e.Entry.TypedKey(key)
	}
}

func (e *readOnlyTerminalEntry) TypedShortcut(shortcut fyne.Shortcut) {
	switch shortcut.(type) {
	case *fyne.ShortcutCopy, *fyne.ShortcutSelectAll:
		e.Entry.TypedShortcut(shortcut)
	}
}

// serviceRowLayout arranges the 7 columns in the process manager services table:
// Col 0: Name & Shortcut (Flexible flex width, takes remaining space, min 130)
// Col 1: Group (95)
// Col 2: Status (85)
// Col 3: Port / PID (95)
// Col 4: RAM / Rest (105)
// Col 5: Uptime (65)
// Col 6: Actions (180)
type serviceRowLayout struct{}

func newServiceRowLayout() fyne.Layout {
	return &serviceRowLayout{}
}

func (l *serviceRowLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) == 0 {
		return fyne.NewSize(0, 0)
	}

	var maxH float32 = 32
	for _, obj := range objects {
		if !obj.Visible() {
			continue
		}
		if h := obj.MinSize().Height; h > maxH {
			maxH = h
		}
	}

	// Sum of column base widths: 130 + 95 + 85 + 95 + 105 + 65 + 180 = 755
	return fyne.NewSize(755, maxH)
}

func (l *serviceRowLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) < 7 {
		return
	}

	const (
		groupW   float32 = 95
		statusW  float32 = 85
		portW    float32 = 95
		ramW     float32 = 105
		uptimeW  float32 = 65
		actionsW float32 = 180
		minNameW float32 = 130
	)

	fixedW := groupW + statusW + portW + ramW + uptimeW + actionsW
	nameW := size.Width - fixedW
	if nameW < minNameW {
		nameW = minNameW
	}

	colWidths := []float32{nameW, groupW, statusW, portW, ramW, uptimeW, actionsW}

	var x float32 = 0
	rowH := size.Height

	for i := 0; i < 7 && i < len(objects); i++ {
		obj := objects[i]
		if !obj.Visible() {
			continue
		}
		w := colWidths[i]
		obj.Move(fyne.NewPos(x, 0))
		obj.Resize(fyne.NewSize(w, rowH))
		x += w
	}
}

// ensureWindowInBounds checks the actual Windows desktop work area (excluding taskbars)
// and ensures the application window is never spawned off-screen or with negative coordinates.
func ensureWindowInBounds(title string, targetW, targetH int32) {
	go func() {
		user32 := syscall.NewLazyDLL("user32.dll")
		kernel32 := syscall.NewLazyDLL("kernel32.dll")
		procFindWindowW := user32.NewProc("FindWindowW")
		procGetWindowRect := user32.NewProc("GetWindowRect")
		procSetWindowPos := user32.NewProc("SetWindowPos")
		procSPI := user32.NewProc("SystemParametersInfoW")
		procEnumWindows := user32.NewProc("EnumWindows")
		procGetWindowThreadProcessId := user32.NewProc("GetWindowThreadProcessId")
		procIsWindowVisible := user32.NewProc("IsWindowVisible")
		procGetCurrentProcessId := kernel32.NewProc("GetCurrentProcessId")

		titlePtr, _ := syscall.UTF16PtrFromString(title)
		myPid, _, _ := procGetCurrentProcessId.Call()

		// Loop up to 30 times (3.0s) to allow GLFW / Fyne to instantiate the native window
		for i := 0; i < 30; i++ {
			time.Sleep(100 * time.Millisecond)

			var hwnd uintptr
			hwnd, _, _ = procFindWindowW.Call(0, uintptr(unsafe.Pointer(titlePtr)))

			if hwnd == 0 {
				// Fallback: enumerate visible windows belonging to this process
				cb := syscall.NewCallback(func(h uintptr, lParam uintptr) uintptr {
					var p uint32
					procGetWindowThreadProcessId.Call(h, uintptr(unsafe.Pointer(&p)))
					if uintptr(p) == myPid {
						vis, _, _ := procIsWindowVisible.Call(h)
						if vis != 0 {
							hwnd = h
							return 0 // stop enumeration
						}
					}
					return 1
				})
				procEnumWindows.Call(cb, 0)
			}

			if hwnd == 0 {
				continue
			}

			type rect struct {
				Left, Top, Right, Bottom int32
			}
			var workArea rect
			procSPI.Call(0x0030, 0, uintptr(unsafe.Pointer(&workArea)), 0)

			var winRect rect
			procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&winRect)))

			winW := winRect.Right - winRect.Left
			winH := winRect.Bottom - winRect.Top

			areaW := workArea.Right - workArea.Left
			areaH := workArea.Bottom - workArea.Top

			// If target dimensions are specified, clamp to target
			if targetW > 0 && winW > targetW {
				winW = targetW
			}
			if targetH > 0 && winH > targetH {
				winH = targetH
			}

			// Ensure window never exceeds available monitor work area
			if winW > areaW-30 {
				winW = areaW - 30
			}
			if winH > areaH-30 {
				winH = areaH - 30
			}

			// Center window on screen within work area
			targetX := workArea.Left + (areaW-winW)/2
			targetY := workArea.Top + (areaH-winH)/2

			if targetX < workArea.Left+10 {
				targetX = workArea.Left + 10
			}
			if targetY < workArea.Top+10 {
				targetY = workArea.Top + 10
			}

			const swpNoZOrder = 0x0004
			const swpShowWindow = 0x0040
			procSetWindowPos.Call(hwnd, 0, uintptr(targetX), uintptr(targetY), uintptr(winW), uintptr(winH), uintptr(swpNoZOrder|swpShowWindow))
			return
		}
	}()
}

// StartDesktopGUI launches a 100% native Fyne Windows desktop application with embedded tools and games.
func StartDesktopGUI(port int) error {
	pm := GetManager()

	// Ensure background web server is running so /game and web API are active
	go func() {
		_ = StartWebServer(port, false)
	}()

	a := app.NewWithID("com.processmanager.native.desktop")
	a.SetIcon(resourceIconPng)
	a.Settings().SetTheme(newCustomDarkTheme())

	w := a.NewWindow("Windows Process Manager & Service Supervisor (Native Go)")
	w.SetIcon(resourceIconPng)
	w.Resize(fyne.NewSize(980, 580))

	// State trackers
	var (
		selectedServiceLock sync.Mutex
		selectedService     = ""

		selectedGroupLock sync.Mutex
		selectedGroup     = "ALL"
	)

	// --- Top Header & Toolbar ---
	titleLbl := widget.NewLabelWithStyle("⚡ Process Manager", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	var adminBadge *widget.Label
	var elevateBtn *widget.Button
	if IsAdmin() {
		adminBadge = widget.NewLabelWithStyle("[ADMIN]", fyne.TextAlignLeading, fyne.TextStyle{Bold: true, Italic: true})
	} else {
		adminBadge = widget.NewLabelWithStyle("[STANDARD]", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
		elevateBtn = widget.NewButtonWithIcon("Elevate", theme.WarningIcon(), func() {
			dialog.ShowConfirm("Admin Elevation", "Restart Process Manager with Windows Administrator privileges?", func(confirm bool) {
				if confirm {
					if err := RequestAdminElevation([]string{"gui"}); err == nil {
						w.Close()
					} else {
						dialog.ShowError(err, w)
					}
				}
			}, w)
		})
	}

	headerLeft := container.NewHBox(titleLbl, adminBadge)
	if elevateBtn != nil {
		headerLeft.Add(elevateBtn)
	}

	// Service container for list
	servicesListContainer := container.NewVBox()

	// Group filter bar containers: pills scrollable on left, group actions pinned on right
	groupPillsContainer := container.NewHBox()
	groupActionContainer := container.NewHBox()
	groupBarContainer := container.NewBorder(nil, nil, nil, groupActionContainer, container.NewHScroll(groupPillsContainer))

	// Logs viewer widgets
	logTitle := widget.NewLabelWithStyle("📜 Select a service to inspect logs", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	logsEntry := newReadOnlyTerminalEntry()

	var (
		lastLoggedService  string
		lastLogSize        int64
		lastLogModTime     time.Time
		selectedLinesCount = 250
		autoScrollCheck    *widget.Check
	)

	// logScroll is declared here so refreshLogs can manage scroll positioning.
	var logScroll *container.Scroll

	refreshLogs := func() {
		selectedServiceLock.Lock()
		target := selectedService
		selectedServiceLock.Unlock()

		if target == "" {
			if logsEntry.Text != "No service selected. Click 'Logs' on any service above to view real-time output." {
				logsEntry.SetText("No service selected. Click 'Logs' on any service above to view real-time output.")
				if logScroll != nil {
					logScroll.ScrollToBottom()
				}
			}
			return
		}

		logPath := filepath.Join(pm.LogsDir, fmt.Sprintf("%s.log", target))
		fi, err := os.Stat(logPath)
		isServiceSwitch := (target != lastLoggedService)

		// 1. If user explicitly turned off Auto-Scroll, FREEZE the log viewer completely!
		// Do not reload logs, do not touch text, do not move cursor/scroll.
		if autoScrollCheck != nil && !autoScrollCheck.Checked && !isServiceSwitch {
			return
		}

		// 2. Check if user is scrolled away from bottom. If reading history, DO NOT update text!
		isNearBottom := true
		if logScroll != nil && logScroll.Content != nil {
			contentH := logScroll.Content.Size().Height
			viewH := logScroll.Size().Height
			if contentH > viewH && viewH > 0 {
				maxOffset := contentH - viewH
				if maxOffset > 40 {
					// User scrolled up >40px away from bottom: actively reading previous logs!
					isNearBottom = (logScroll.Offset.Y >= maxOffset-40)
				}
			}
		}

		// If user has scrolled up to inspect previous logs, do NOT update Entry text buffer!
		// In Fyne, calling Entry.SetText() resets cursor and triggers internal auto-scrolling.
		if !isNearBottom && !isServiceSwitch {
			return
		}

		if err == nil && !isServiceSwitch {
			if fi.Size() == lastLogSize && fi.ModTime().Equal(lastLogModTime) {
				return
			}
			lastLogSize = fi.Size()
			lastLogModTime = fi.ModTime()
		} else if err == nil {
			lastLoggedService = target
			lastLogSize = fi.Size()
			lastLogModTime = fi.ModTime()
		} else {
			lastLoggedService = target
			lastLogSize = 0
		}

		output := pm.GetLogs(target, selectedLinesCount)
		if strings.TrimSpace(output) == "" {
			logsEntry.SetText("(No log entries recorded yet)")
		} else {
			logsEntry.SetText(output)
		}

		if logScroll != nil {
			logScroll.ScrollToBottom()
		}
	}

	clearLogsBtn := widget.NewButtonWithIcon("Clear Logs", theme.DeleteIcon(), func() {
		selectedServiceLock.Lock()
		target := selectedService
		selectedServiceLock.Unlock()
		if target != "" {
			_ = pm.ClearLogs(target)
			lastLogSize = 0
			lastLoggedService = ""
			refreshLogs()
			if logScroll != nil {
				logScroll.ScrollToBottom()
			}
		}
	})

	copyLogsBtn := widget.NewButtonWithIcon("Copy Logs", theme.ContentCopyIcon(), func() {
		selectedServiceLock.Lock()
		target := selectedService
		selectedServiceLock.Unlock()
		if target != "" && logsEntry.Text != "" {
			w.Clipboard().SetContent(logsEntry.Text)
			a.SendNotification(&fyne.Notification{
				Title:   "Logs Copied",
				Content: fmt.Sprintf("📋 Copied %s logs to clipboard", target),
			})
		}
	})

	autoScrollCheck = widget.NewCheck("Auto-Scroll", func(checked bool) {
		if checked {
			lastLogSize = 0
			lastLoggedService = ""
			refreshLogs()
			if logScroll != nil {
				logScroll.ScrollToBottom()
			}
		}
	})
	autoScrollCheck.SetChecked(true)

	toBottomBtn := widget.NewButtonWithIcon("Bottom", theme.MoveDownIcon(), func() {
		lastLogSize = 0
		lastLoggedService = ""
		if autoScrollCheck != nil {
			autoScrollCheck.SetChecked(true)
		}
		refreshLogs()
		if logScroll != nil {
			logScroll.ScrollToBottom()
		}
	})

	linesSelect := widget.NewSelect([]string{"100 lines", "250 lines", "500 lines", "1000 lines"}, func(val string) {
		switch val {
		case "100 lines":
			selectedLinesCount = 100
		case "250 lines":
			selectedLinesCount = 250
		case "500 lines":
			selectedLinesCount = 500
		case "1000 lines":
			selectedLinesCount = 1000
		}
		lastLogSize = 0 // Force reload with new line count
		refreshLogs()
	})
	linesSelect.SetSelected("250 lines")

	logControls := container.NewHBox(autoScrollCheck, toBottomBtn, linesSelect, copyLogsBtn, clearLogsBtn)
	logToolbar := container.NewBorder(nil, nil, logTitle, logControls)
	logScroll = container.NewScroll(logsEntry)
	logScroll.SetMinSize(fyne.NewSize(300, 120))
	logPanel := container.NewBorder(logToolbar, nil, nil, nil, logScroll)

	// Forward declarations
	var renderServices func()
	var registerShortcuts func()

	// Global Hotkey Manager (System-Wide Windows Hotkeys usable outside the app)
	ghm := GetGlobalHotkeyManager(pm)
	ghm.Start(func(serviceName string, actionMsg string) {
		a.SendNotification(&fyne.Notification{
			Title:   "Process Manager",
			Content: fmt.Sprintf("⚡ %s", actionMsg),
		})
		fyne.Do(func() {
			renderServices()
			refreshLogs()
		})
	})
	cw := GetClipboardWatcher()
	cw.Start()

	w.SetOnClosed(func() {
		ghm.Stop()
		GetJigglerEngine().Stop()
		GetClipboardWatcher().Stop()
	})

	// Caching state to avoid thrashing UI objects on every tick
	type serviceRowWidgets struct {
		statusCanvas     *canvas.Text
		portPidLabel     *widget.Label
		ramRestartsLabel *widget.Label
		uptimeLabel      *widget.Label
		toggleBtn        *widget.Button
		lastStatus       string
	}

	var (
		renderedRows      = make(map[string]*serviceRowWidgets)
		lastRenderedGroup = ""
		lastServiceSig    = ""
		lastGroupBarSig   = ""

		openManageGroupsDialog func()
	)

	// Function to render group filter tabs
	renderGroupBar := func(allServices []map[string]interface{}) {
		selectedGroupLock.Lock()
		activeGrp := selectedGroup
		selectedGroupLock.Unlock()

		groups := pm.GetGroups()

		// Signature to avoid re-rendering group buttons if unchanged
		var sb strings.Builder
		sb.WriteString(activeGrp)
		sb.WriteString(";")
		for _, g := range groups {
			sb.WriteString(g)
			sb.WriteString(":")
			c := 0
			for _, s := range allServices {
				if MatchGroup(fmt.Sprintf("%v", s["group"]), g) {
					c++
				}
			}
			sb.WriteString(strconv.Itoa(c))
			sb.WriteString(";")
		}
		sb.WriteString(strconv.Itoa(len(allServices)))
		barSig := sb.String()
		if barSig == lastGroupBarSig {
			return
		}
		lastGroupBarSig = barSig

		groupPillsContainer.Objects = nil
		groupActionContainer.Objects = nil

		// "ALL" Filter Button
		allLabel := fmt.Sprintf("🌐 ALL (%d)", len(allServices))
		allBtn := widget.NewButton(allLabel, func() {
			selectedGroupLock.Lock()
			selectedGroup = "ALL"
			selectedGroupLock.Unlock()
			renderServices()
		})
		if activeGrp == "ALL" {
			allBtn.Importance = widget.HighImportance
		}
		groupPillsContainer.Add(allBtn)

		// Group Buttons
		for _, g := range groups {
			grpName := g
			count := 0
			for _, s := range allServices {
				if MatchGroup(fmt.Sprintf("%v", s["group"]), grpName) {
					count++
				}
			}

			gBtn := widget.NewButton(fmt.Sprintf("📁 %s (%d)", grpName, count), func() {
				selectedGroupLock.Lock()
				selectedGroup = grpName
				selectedGroupLock.Unlock()
				renderServices()
			})
			if activeGrp == grpName {
				gBtn.Importance = widget.HighImportance
			}
			groupPillsContainer.Add(gBtn)
		}

		// Manage Groups quick button on group filter bar
		manageGrpBarBtn := widget.NewButtonWithIcon("⚙️ Manage Groups", theme.SettingsIcon(), func() {
			if openManageGroupsDialog != nil {
				openManageGroupsDialog()
			}
		})
		groupPillsContainer.Add(manageGrpBarBtn)

		// If a specific group is selected, show Group Batch Actions on the right!
		if activeGrp != "ALL" {
			grpTitle := widget.NewLabelWithStyle(fmt.Sprintf("Group '%s':", activeGrp), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

			startGrpBtn := widget.NewButtonWithIcon("Start", theme.MediaPlayIcon(), func() {
				pm.StartGroup(activeGrp)
				renderServices()
			})
			stopGrpBtn := widget.NewButtonWithIcon("Stop", theme.MediaStopIcon(), func() {
				pm.StopGroup(activeGrp)
				renderServices()
			})
			restartGrpBtn := widget.NewButtonWithIcon("Restart", theme.ViewRefreshIcon(), func() {
				pm.RestartGroup(activeGrp)
				renderServices()
			})

			groupActionContainer.Add(grpTitle)
			groupActionContainer.Add(startGrpBtn)
			groupActionContainer.Add(stopGrpBtn)
			groupActionContainer.Add(restartGrpBtn)
		}

		groupPillsContainer.Refresh()
		groupActionContainer.Refresh()
		groupBarContainer.Refresh()
	}

	// Helper to render service rows
	renderServices = func() {
		allServices := getServicesList(pm)

		// Render group filter bar with counts
		renderGroupBar(allServices)

		// Filter services according to selectedGroup
		selectedGroupLock.Lock()
		activeGrp := selectedGroup
		selectedGroupLock.Unlock()

		var services []map[string]interface{}
		if activeGrp == "ALL" {
			services = allServices
		} else {
			for _, s := range allServices {
				if MatchGroup(fmt.Sprintf("%v", s["group"]), activeGrp) {
					services = append(services, s)
				}
			}
		}

		// Compute structural signature
		var sb strings.Builder
		sb.WriteString(activeGrp)
		sb.WriteString(";")
		for _, s := range services {
			sb.WriteString(fmt.Sprintf("%v:%v:%v;", s["name"], s["group"], s["shortcut"]))
		}
		curSig := sb.String()

		// Fast Path: in-place metric and status updates without destroying Fyne widgets
		if activeGrp == lastRenderedGroup && curSig == lastServiceSig && len(renderedRows) == len(services) && len(services) > 0 {
			for _, s := range services {
				sName := fmt.Sprintf("%v", s["name"])
				row, exists := renderedRows[sName]
				if !exists {
					continue
				}

				sStatus := strings.ToLower(fmt.Sprintf("%v", s["status"]))
				sPID := fmt.Sprintf("%v", s["pid"])
				sRestarts := fmt.Sprintf("%v", s["restarts"])

				portStr := "-"
				switch p := s["port"].(type) {
				case *int:
					if p != nil && *p > 0 {
						portStr = fmt.Sprintf(":%d", *p)
					}
				case int:
					if p > 0 {
						portStr = fmt.Sprintf(":%d", p)
					}
				case float64:
					if p > 0 {
						portStr = fmt.Sprintf(":%d", int(p))
					}
				}

				ramStr := "-"
				if mem, ok := s["memory_mb"].(float64); ok && mem > 0 {
					ramStr = fmt.Sprintf("%.1f MB", mem)
				}

				uptimeStr := "-"
				if sStatus == "running" {
					if up, ok := s["uptime_seconds"].(int64); ok && up > 0 {
						mins := up / 60
						secs := up % 60
						if mins > 0 {
							uptimeStr = fmt.Sprintf("%dm %ds", mins, secs)
						} else {
							uptimeStr = fmt.Sprintf("%ds", secs)
						}
					}
				}

				if sStatus != row.lastStatus {
					row.lastStatus = sStatus
					var statusColor color.Color = color.RGBA{R: 239, G: 68, B: 68, A: 255} // red
					statusText := "● STOPPED"
					if sStatus == "running" {
						statusColor = color.RGBA{R: 16, G: 185, B: 129, A: 255} // emerald
						statusText = "● RUNNING"
						row.toggleBtn.SetIcon(theme.MediaStopIcon())
					} else if sStatus == "restarting" || sStatus == "crashed" {
						statusColor = color.RGBA{R: 245, G: 158, B: 11, A: 255} // amber
						statusText = fmt.Sprintf("● %s", strings.ToUpper(sStatus))
						row.toggleBtn.SetIcon(theme.MediaPlayIcon())
					} else {
						row.toggleBtn.SetIcon(theme.MediaPlayIcon())
					}
					row.statusCanvas.Text = statusText
					row.statusCanvas.Color = statusColor
					row.statusCanvas.Refresh()
				}

				newPortPid := fmt.Sprintf("%s / %s", portStr, sPID)
				if row.portPidLabel.Text != newPortPid {
					row.portPidLabel.SetText(newPortPid)
				}

				newRamRestarts := fmt.Sprintf("%s (R: %s)", ramStr, sRestarts)
				if row.ramRestartsLabel.Text != newRamRestarts {
					row.ramRestartsLabel.SetText(newRamRestarts)
				}

				if row.uptimeLabel.Text != uptimeStr {
					row.uptimeLabel.SetText(uptimeStr)
				}
			}
			return
		}

		// Slow Path: Structural change (service added/removed/filtered). Rebuild rows.
		lastRenderedGroup = activeGrp
		lastServiceSig = curSig
		servicesListContainer.Objects = nil
		renderedRows = make(map[string]*serviceRowWidgets)

		if len(services) == 0 {
			emptyMsg := "No managed services registered yet. Click '➕ Add Service' above to configure one."
			if activeGrp != "ALL" {
				emptyMsg = fmt.Sprintf("No services in group '%s'.", activeGrp)
			}
			emptyCard := widget.NewCard("", "", container.NewCenter(
				widget.NewLabel(emptyMsg),
			))
			servicesListContainer.Add(emptyCard)
			servicesListContainer.Refresh()
			return
		}

		// Header Row
		colHeader := container.New(newServiceRowLayout(),
			widget.NewLabelWithStyle("NAME / SHORTCUT", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("GROUP", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("STATUS", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("PORT / PID", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("RAM / REST", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("UPTIME", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("ACTIONS", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
		)
		servicesListContainer.Add(colHeader)
		servicesListContainer.Add(widget.NewSeparator())

		for _, s := range services {
			sName := fmt.Sprintf("%v", s["name"])
			sGroup := fmt.Sprintf("%v", s["group"])
			sStatus := strings.ToLower(fmt.Sprintf("%v", s["status"]))
			sPID := fmt.Sprintf("%v", s["pid"])
			sRestarts := fmt.Sprintf("%v", s["restarts"])
			sShortcut := ""
			if sc, ok := s["shortcut"].(string); ok {
				sShortcut = strings.TrimSpace(sc)
			}

			portStr := "-"
			switch p := s["port"].(type) {
			case *int:
				if p != nil && *p > 0 {
					portStr = fmt.Sprintf(":%d", *p)
				}
			case int:
				if p > 0 {
					portStr = fmt.Sprintf(":%d", p)
				}
			case float64:
				if p > 0 {
					portStr = fmt.Sprintf(":%d", int(p))
				}
			}

			ramStr := "-"
			if mem, ok := s["memory_mb"].(float64); ok && mem > 0 {
				ramStr = fmt.Sprintf("%.1f MB", mem)
			}

			uptimeStr := "-"
			if sStatus == "running" {
				if up, ok := s["uptime_seconds"].(int64); ok && up > 0 {
					mins := up / 60
					secs := up % 60
					if mins > 0 {
						uptimeStr = fmt.Sprintf("%dm %ds", mins, secs)
					} else {
						uptimeStr = fmt.Sprintf("%ds", secs)
					}
				}
			}

			// Status indicator
			var statusColor color.Color = color.RGBA{R: 239, G: 68, B: 68, A: 255} // red
			statusText := "● STOPPED"
			if sStatus == "running" {
				statusColor = color.RGBA{R: 16, G: 185, B: 129, A: 255} // emerald
				statusText = "● RUNNING"
			} else if sStatus == "restarting" || sStatus == "crashed" {
				statusColor = color.RGBA{R: 245, G: 158, B: 11, A: 255} // amber
				statusText = fmt.Sprintf("● %s", strings.ToUpper(sStatus))
			}

			statusCanvas := canvas.NewText(statusText, statusColor)
			statusCanvas.TextStyle = fyne.TextStyle{Bold: true}

			nameLabel := widget.NewLabelWithStyle(sName, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			nameLabel.Truncation = fyne.TextTruncateEllipsis
			var nameWidget fyne.CanvasObject = nameLabel
			if sShortcut != "" && sShortcut != "<nil>" {
				scBadge := canvas.NewText(fmt.Sprintf("⌨️ %s", sShortcut), color.RGBA{R: 56, G: 189, B: 248, A: 255})
				scBadge.TextStyle = fyne.TextStyle{Monospace: true, Bold: true}
				scBadge.TextSize = 11
				nameWidget = container.NewVBox(nameLabel, scBadge)
			}

			groupLabel := widget.NewLabel(sGroup)
			groupLabel.Truncation = fyne.TextTruncateEllipsis
			portPidLabel := widget.NewLabel(fmt.Sprintf("%s / %s", portStr, sPID))
			portPidLabel.Truncation = fyne.TextTruncateEllipsis
			ramRestartsLabel := widget.NewLabel(fmt.Sprintf("%s (R: %s)", ramStr, sRestarts))
			ramRestartsLabel.Truncation = fyne.TextTruncateEllipsis
			uptimeLabel := widget.NewLabel(uptimeStr)
			uptimeLabel.Truncation = fyne.TextTruncateEllipsis

			// Action Buttons
			currName := sName
			toggleIcon := theme.MediaPlayIcon()
			if sStatus == "running" {
				toggleIcon = theme.MediaStopIcon()
			}
			toggleBtn := widget.NewButtonWithIcon("", toggleIcon, func() {
				pm.lock.RLock()
				ms, exists := pm.services[currName]
				isRunning := false
				if exists {
					isRunning = ms.IsRunning()
				}
				pm.lock.RUnlock()

				if isRunning {
					_ = pm.StopService(currName)
				} else {
					_ = pm.StartService(currName)
				}
				renderServices()
				refreshLogs()
			})

			restartBtn := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
				_ = pm.RestartService(currName)
				renderServices()
				refreshLogs()
			})

			// ✏️ Edit Service Button
			editBtn := widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), func() {
				cfg, ok := pm.GetServiceConfig(currName)
				if !ok {
					dialog.ShowError(fmt.Errorf("service '%s' not found", currName), w)
					return
				}

				nameEntry := widget.NewEntry()
				nameEntry.SetText(cfg.Name)

				groupEntry := widget.NewEntry()
				groupEntry.SetText(cfg.Group)

				cmdEntry := widget.NewEntry()
				cmdEntry.SetText(cfg.Command)

				cwdEntry := widget.NewEntry()
				cwdEntry.SetText(cfg.Cwd)

				portEntry := widget.NewEntry()
				if cfg.Port != nil && *cfg.Port > 0 {
					portEntry.SetText(strconv.Itoa(*cfg.Port))
				}

				shortcutEntry := widget.NewEntry()
				shortcutEntry.SetPlaceHolder("e.g. Ctrl+1, F5, Ctrl+Shift+R")
				shortcutEntry.SetText(cfg.Shortcut)

				venvEntry := widget.NewEntry()
				venvEntry.SetText(cfg.Venv)

				autoRestartCheck := widget.NewCheck("Enable Auto-Restart on Crash", nil)
				autoRestartCheck.SetChecked(cfg.AutoRestart)

				restartIfRunningCheck := widget.NewCheck("Restart process if currently running", nil)
				restartIfRunningCheck.SetChecked(true)

				items := []*widget.FormItem{
					{Text: "Service Name", Widget: nameEntry},
					{Text: "Service Group", Widget: groupEntry},
					{Text: "Command Line", Widget: cmdEntry},
					{Text: "Working Directory", Widget: cwdEntry},
					{Text: "Port (Optional)", Widget: portEntry},
					{Text: "Keyboard Shortcut", Widget: shortcutEntry},
					{Text: "Python Venv Path", Widget: venvEntry},
					{Text: "Auto-Restart", Widget: autoRestartCheck},
					{Text: "Apply & Restart", Widget: restartIfRunningCheck},
				}

				d := dialog.NewForm(fmt.Sprintf("✏️ Edit Service: %s", currName), "Save Changes", "Cancel", items, func(confirmed bool) {
					if !confirmed {
						return
					}
					newName := strings.TrimSpace(nameEntry.Text)
					newCmd := strings.TrimSpace(cmdEntry.Text)
					if newName == "" || newCmd == "" {
						dialog.ShowInformation("Validation Error", "Service Name and Command Line cannot be empty.", w)
						return
					}

					var newPort *int
					if pStr := strings.TrimSpace(portEntry.Text); pStr != "" {
						if pVal, err := strconv.Atoi(pStr); err == nil && pVal > 0 {
							newPort = &pVal
						}
					}

					updatedCfg := ServiceConfig{
						Name:        newName,
						Command:     newCmd,
						Cwd:         strings.TrimSpace(cwdEntry.Text),
						Port:        newPort,
						AutoRestart: autoRestartCheck.Checked,
						Venv:        strings.TrimSpace(venvEntry.Text),
						Group:       strings.TrimSpace(groupEntry.Text),
						Shortcut:    strings.TrimSpace(shortcutEntry.Text),
						Env:         cfg.Env,
					}

					pm.UpdateService(currName, updatedCfg, restartIfRunningCheck.Checked)
					if registerShortcuts != nil {
						registerShortcuts()
					}
					renderServices()
					refreshLogs()
				}, w)
				d.Resize(fyne.NewSize(620, 560))
				d.Show()
			})

			logBtn := widget.NewButtonWithIcon("", theme.DocumentIcon(), func() {
				selectedServiceLock.Lock()
				selectedService = currName
				selectedServiceLock.Unlock()
				logTitle.SetText(fmt.Sprintf("📜 Logs for '%s'", currName))
				lastLoggedService = "" // Force log reload
				refreshLogs()
			})

			deleteBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
				dialog.ShowConfirm("Delete Service", fmt.Sprintf("Are you sure you want to delete '%s'?", currName), func(ok bool) {
					if ok {
						_ = pm.DeleteService(currName)
						if registerShortcuts != nil {
							registerShortcuts()
						}
						renderServices()
					}
				}, w)
			})

			actionBox := container.NewHBox(layout.NewSpacer(), toggleBtn, restartBtn, editBtn, logBtn, deleteBtn)

			row := container.New(newServiceRowLayout(),
				nameWidget,
				groupLabel,
				container.NewCenter(statusCanvas),
				portPidLabel,
				ramRestartsLabel,
				uptimeLabel,
				actionBox,
			)

			renderedRows[sName] = &serviceRowWidgets{
				statusCanvas:     statusCanvas,
				portPidLabel:     portPidLabel,
				ramRestartsLabel: ramRestartsLabel,
				uptimeLabel:      uptimeLabel,
				toggleBtn:        toggleBtn,
				lastStatus:       sStatus,
			}

			servicesListContainer.Add(row)
			servicesListContainer.Add(widget.NewSeparator())
		}
		servicesListContainer.Refresh()
	}

	var (
		activeShortcutsLock sync.Mutex
		activeShortcuts     []fyne.Shortcut
	)

	registerShortcuts = func() {
		activeShortcutsLock.Lock()
		defer activeShortcutsLock.Unlock()

		for _, sc := range activeShortcuts {
			w.Canvas().RemoveShortcut(sc)
		}
		activeShortcuts = nil

		// Register in-app shortcut for Mouse Jiggler
		jigglerSC := strings.TrimSpace(GetJigglerEngine().GetConfig().Shortcut)
		if jigglerSC != "" && jigglerSC != "<nil>" {
			if parsed, err := parseShortcut(jigglerSC); err == nil {
				targetSC := jigglerSC
				w.Canvas().AddShortcut(parsed, func(shortcut fyne.Shortcut) {
					je := GetJigglerEngine()
					actionMsg := ""
					if je.IsRunning() {
						je.Stop()
						actionMsg = fmt.Sprintf("⏹ Mouse Jiggler: Stopped (%s)", targetSC)
					} else {
						je.Start()
						actionMsg = fmt.Sprintf("⚡ Mouse Jiggler: Started (%s)", targetSC)
					}
					a.SendNotification(&fyne.Notification{
						Title:   "Mouse Jiggler",
						Content: actionMsg,
					})
				})
				activeShortcuts = append(activeShortcuts, parsed)
			}
		}

		allServices := getServicesList(pm)
		for _, s := range allServices {
			sName := fmt.Sprintf("%v", s["name"])
			sShortcut := ""
			if sc, ok := s["shortcut"].(string); ok {
				sShortcut = strings.TrimSpace(sc)
			}
			if sShortcut == "" || sShortcut == "<nil>" {
				continue
			}

			parsed, err := parseShortcut(sShortcut)
			if err != nil {
				continue
			}

			targetName := sName
			targetShortcut := sShortcut

			w.Canvas().AddShortcut(parsed, func(shortcut fyne.Shortcut) {
				pm.lock.RLock()
				ms, exists := pm.services[targetName]
				isRunning := false
				if exists {
					isRunning = ms.IsRunning()
				}
				pm.lock.RUnlock()

				actionMsg := ""
				if isRunning {
					_ = pm.StopService(targetName)
					actionMsg = fmt.Sprintf("Stopped '%s' (%s)", targetName, targetShortcut)
				} else {
					_ = pm.StartService(targetName)
					actionMsg = fmt.Sprintf("Started '%s' (%s)", targetName, targetShortcut)
				}

				a.SendNotification(&fyne.Notification{
					Title:   "Process Manager Shortcut",
					Content: actionMsg,
				})

				renderServices()
				refreshLogs()
			})

			activeShortcuts = append(activeShortcuts, parsed)
		}

		if ghm != nil {
			ghm.Reload()
		}
	}

	// Initial registration of shortcuts
	registerShortcuts()

	// --- Global Action Handlers ---
	startAllBtn := widget.NewButtonWithIcon("Start All", theme.MediaPlayIcon(), func() {
		pm.StartAll()
		renderServices()
	})

	stopAllBtn := widget.NewButtonWithIcon("Stop All", theme.MediaStopIcon(), func() {
		pm.StopAll()
		renderServices()
	})

	resurrectBtn := widget.NewButtonWithIcon("Resurrect", theme.ViewRefreshIcon(), func() {
		pm.Resurrect()
		renderServices()
	})

	webBtn := widget.NewButtonWithIcon("Web UI", theme.NavigateNextIcon(), func() {
		go func() {
			_ = StartWebServer(port, true)
		}()
	})

	addServiceBtn := widget.NewButtonWithIcon("➕ Add Service", theme.ContentAddIcon(), func() {
		nameEntry := widget.NewEntry()
		cmdEntry := widget.NewEntry()
		cwdEntry := widget.NewEntry()
		cwdEntry.SetText(pm.BaseDir)
		portEntry := widget.NewEntry()
		venvEntry := widget.NewEntry()
		groupEntry := widget.NewEntry()

		// Pre-populate with currently selected group if not ALL
		selectedGroupLock.Lock()
		curGroup := selectedGroup
		selectedGroupLock.Unlock()
		if curGroup != "ALL" {
			groupEntry.SetText(curGroup)
		} else {
			groupEntry.SetText("default")
		}

		shortcutEntry := widget.NewEntry()
		shortcutEntry.SetPlaceHolder("e.g. Ctrl+1, F5, Alt+1")

		autoRestartCheck := widget.NewCheck("Enable Auto-Restart on Crash", nil)
		autoRestartCheck.SetChecked(true)

		items := []*widget.FormItem{
			{Text: "Service Name", Widget: nameEntry},
			{Text: "Service Group", Widget: groupEntry},
			{Text: "Command Line", Widget: cmdEntry},
			{Text: "Working Directory", Widget: cwdEntry},
			{Text: "Port (Optional)", Widget: portEntry},
			{Text: "Keyboard Shortcut", Widget: shortcutEntry},
			{Text: "Python Venv Path", Widget: venvEntry},
			{Text: "Auto-Restart", Widget: autoRestartCheck},
		}

		d := dialog.NewForm("Add New Background Service", "Save & Start", "Cancel", items, func(confirmed bool) {
			if !confirmed {
				return
			}
			sName := strings.TrimSpace(nameEntry.Text)
			sCmd := strings.TrimSpace(cmdEntry.Text)
			if sName == "" || sCmd == "" {
				dialog.ShowInformation("Validation Error", "Service Name and Command Line are required.", w)
				return
			}

			var sPort *int
			if pStr := strings.TrimSpace(portEntry.Text); pStr != "" {
				if pVal, err := strconv.Atoi(pStr); err == nil && pVal > 0 {
					sPort = &pVal
				}
			}

			cfg := ServiceConfig{
				Name:        sName,
				Command:     sCmd,
				Cwd:         strings.TrimSpace(cwdEntry.Text),
				Port:        sPort,
				AutoRestart: autoRestartCheck.Checked,
				Venv:        strings.TrimSpace(venvEntry.Text),
				Group:       strings.TrimSpace(groupEntry.Text),
				Shortcut:    strings.TrimSpace(shortcutEntry.Text),
			}

			pm.AddService(cfg, true)
			pm.SaveState()
			if registerShortcuts != nil {
				registerShortcuts()
			}
			renderServices()
		}, w)
		d.Resize(fyne.NewSize(620, 560))
		d.Show()
	})

	shortcutsBtn := widget.NewButtonWithIcon("Hotkeys", theme.SettingsIcon(), func() {
		allServices := getServicesList(pm)
		je := GetJigglerEngine()

		entries := make(map[string]*widget.Entry)
		grid := container.NewVBox()

		// --- Section: Mouse Jiggler (Anti-Idle Guard) ---
		jigglerEntry := widget.NewEntry()
		jigglerEntry.SetPlaceHolder("e.g. Ctrl+Alt+J, F8, Ctrl+Shift+M")
		jigglerEntry.SetText(je.GetConfig().Shortcut)

		jigglerClearBtn := widget.NewButtonWithIcon("", theme.ContentClearIcon(), func() {
			jigglerEntry.SetText("")
		})

		jigglerNameLbl := widget.NewLabelWithStyle("🐭 Mouse Jiggler (Toggle)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		jigglerGrpLbl := widget.NewLabel("[Anti-Idle Guard]")
		jigglerLeftBox := container.NewHBox(jigglerNameLbl, jigglerGrpLbl)
		jigglerEntryRow := container.NewBorder(nil, nil, nil, jigglerClearBtn, jigglerEntry)
		jigglerRow := container.NewGridWithColumns(2, jigglerLeftBox, jigglerEntryRow)

		grid.Add(jigglerRow)
		grid.Add(widget.NewSeparator())

		for _, s := range allServices {
			sName := fmt.Sprintf("%v", s["name"])
			sGroup := fmt.Sprintf("%v", s["group"])
			currentSC := ""
			if sc, ok := s["shortcut"].(string); ok {
				currentSC = strings.TrimSpace(sc)
			}
			if currentSC == "<nil>" {
				currentSC = ""
			}

			nameLbl := widget.NewLabelWithStyle(sName, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			grpLbl := widget.NewLabel(fmt.Sprintf("[%s]", sGroup))
			leftBox := container.NewHBox(nameLbl, grpLbl)

			entry := widget.NewEntry()
			entry.SetPlaceHolder("e.g. Ctrl+Alt+1, F5, Ctrl+Shift+C")
			entry.SetText(currentSC)
			entries[sName] = entry

			clearBtn := widget.NewButtonWithIcon("", theme.ContentClearIcon(), func(e *widget.Entry) func() {
				return func() {
					e.SetText("")
				}
			}(entry))

			entryRow := container.NewBorder(nil, nil, nil, clearBtn, entry)
			row := container.NewGridWithColumns(2, leftBox, entryRow)
			grid.Add(row)
			grid.Add(widget.NewSeparator())
		}

		scroll := container.NewVScroll(grid)
		scroll.SetMinSize(fyne.NewSize(620, 360))

		infoLbl := widget.NewLabelWithStyle("Assign system-wide hotkeys to toggle services and tools on or off. Hotkeys work globally from any window.", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
		modalContent := container.NewBorder(infoLbl, nil, nil, nil, scroll)

		d := dialog.NewCustomConfirm("⌨️ Customize Global Hotkeys", "Save & Apply", "Cancel", modalContent, func(confirmed bool) {
			if !confirmed {
				return
			}

			updatedCount := 0

			// Save Jiggler shortcut
			newJigglerSC := strings.TrimSpace(jigglerEntry.Text)
			jCfg := je.GetConfig()
			if jCfg.Shortcut != newJigglerSC {
				jCfg.Shortcut = newJigglerSC
				je.UpdateConfig(jCfg)
				updatedCount++
			}

			for sName, entry := range entries {
				cfg, ok := pm.GetServiceConfig(sName)
				if !ok {
					continue
				}
				newSC := strings.TrimSpace(entry.Text)
				if cfg.Shortcut != newSC {
					cfg.Shortcut = newSC
					pm.UpdateService(sName, cfg, false)
					updatedCount++
				}
			}

			if updatedCount > 0 {
				pm.SaveState()
				if registerShortcuts != nil {
					registerShortcuts()
				}
				renderServices()
				a.SendNotification(&fyne.Notification{
					Title:   "Hotkeys Updated",
					Content: fmt.Sprintf("Updated %d hotkey(s) successfully!", updatedCount),
				})
			}
		}, w)
		d.Resize(fyne.NewSize(680, 500))
		d.Show()
	})

	openManageGroupsDialog = func() {
		allServices := getServicesList(pm)
		groups := pm.GetGroups()

		mainBox := container.NewVBox()

		mainBox.Add(widget.NewLabelWithStyle("📁 Manage Service Groups", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		mainBox.Add(widget.NewLabel("Rename groups, batch control, or reassign background services."))
		mainBox.Add(widget.NewSeparator())

		// Group Overview cards
		for _, grp := range groups {
			gName := grp
			var memberNames []string
			for _, s := range allServices {
				if MatchGroup(fmt.Sprintf("%v", s["group"]), gName) {
					memberNames = append(memberNames, fmt.Sprintf("%v", s["name"]))
				}
			}

			titleText := canvas.NewText(fmt.Sprintf("📁 %s (%d services)", gName, len(memberNames)), color.RGBA{R: 56, G: 189, B: 248, A: 255})
			titleText.TextSize = 13
			titleText.TextStyle = fyne.TextStyle{Bold: true}

			membersList := strings.Join(memberNames, ", ")
			if membersList == "" {
				membersList = "(No services in group)"
			}
			membersLbl := widget.NewLabel(membersList)
			membersLbl.Wrapping = fyne.TextWrapWord
			membersLbl.TextStyle = fyne.TextStyle{Italic: true}

			// Rename button
			renameBtn := widget.NewButtonWithIcon("Rename", theme.DocumentCreateIcon(), func() {
				entry := widget.NewEntry()
				entry.SetText(gName)
				d := dialog.NewForm(fmt.Sprintf("✏️ Rename Group: %s", gName), "Save", "Cancel", []*widget.FormItem{
					{Text: "New Group Name", Widget: entry},
				}, func(ok bool) {
					if !ok {
						return
					}
					newName := strings.TrimSpace(entry.Text)
					if newName == "" || newName == gName {
						return
					}
					count := pm.RenameGroup(gName, newName)
					GetTodoStore().RenameProject(gName, newName)
					renderServices()
					a.SendNotification(&fyne.Notification{
						Title:   "Group Renamed",
						Content: fmt.Sprintf("Updated %d service(s) to group '%s'", count, newName),
					})
					openManageGroupsDialog()
				}, w)
				d.Resize(fyne.NewSize(420, 180))
				d.Show()
			})

			// Dissolve/Ungroup button
			dissolveBtn := widget.NewButtonWithIcon("Ungroup", theme.DeleteIcon(), func() {
				dialog.ShowConfirm("Ungroup Services", fmt.Sprintf("Remove group '%s'? (Member services will move to 'default')", gName), func(ok bool) {
					if ok {
						pm.DeleteGroup(gName, "default")
						renderServices()
						openManageGroupsDialog()
					}
				}, w)
			})

			startBtn := widget.NewButtonWithIcon("Start All", theme.MediaPlayIcon(), func() {
				pm.StartGroup(gName)
				renderServices()
			})

			stopBtn := widget.NewButtonWithIcon("Stop All", theme.MediaStopIcon(), func() {
				pm.StopGroup(gName)
				renderServices()
			})

			restartBtn := widget.NewButtonWithIcon("Restart", theme.ViewRefreshIcon(), func() {
				pm.RestartGroup(gName)
				renderServices()
			})

			btnRow := container.NewHBox(startBtn, stopBtn, restartBtn, renameBtn, dissolveBtn)
			groupCard := container.NewVBox(
				titleText,
				membersLbl,
				btnRow,
				widget.NewSeparator(),
			)
			mainBox.Add(groupCard)
		}

		// Section: Move / Reassign Service to another/new group
		mainBox.Add(widget.NewLabelWithStyle("🔄 Move Service to Another Group", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))

		var svcNames []string
		for _, s := range allServices {
			svcNames = append(svcNames, fmt.Sprintf("%v", s["name"]))
		}

		svcSelect := widget.NewSelect(svcNames, nil)
		if len(svcNames) > 0 {
			svcSelect.SetSelected(svcNames[0])
		}

		targetGroupOptions := append([]string{"default"}, groups...)
		targetGroupSelect := widget.NewSelect(targetGroupOptions, nil)
		if len(targetGroupOptions) > 0 {
			targetGroupSelect.SetSelected(targetGroupOptions[0])
		}

		newGroupEntry := widget.NewEntry()
		newGroupEntry.SetPlaceHolder("Or enter a brand new group name...")

		reassignBtn := widget.NewButtonWithIcon("Move Service", theme.NavigateNextIcon(), func() {
			sName := svcSelect.Selected
			if sName == "" {
				return
			}
			targetGrp := targetGroupSelect.Selected
			if custom := strings.TrimSpace(newGroupEntry.Text); custom != "" {
				targetGrp = custom
			}
			if targetGrp == "" {
				return
			}
			pm.MoveServiceToGroup(sName, targetGrp)
			renderServices()
			a.SendNotification(&fyne.Notification{
				Title:   "Service Reassigned",
				Content: fmt.Sprintf("Moved '%s' to group '%s'", sName, targetGrp),
			})
			openManageGroupsDialog()
		})
		reassignBtn.Importance = widget.HighImportance

		reassignForm := widget.NewForm(
			widget.NewFormItem("Select Service", svcSelect),
			widget.NewFormItem("Existing Group", targetGroupSelect),
			widget.NewFormItem("Create New Group", newGroupEntry),
		)

		mainBox.Add(reassignForm)
		mainBox.Add(reassignBtn)

		scroll := container.NewVScroll(mainBox)
		scroll.SetMinSize(fyne.NewSize(620, 440))

		d := dialog.NewCustom("📁 Manage Groups", "Close", scroll, w)
		d.Resize(fyne.NewSize(660, 480))
		d.Show()
	}

	headerRightBox := container.NewHBox(layout.NewSpacer(), shortcutsBtn, addServiceBtn, startAllBtn, stopAllBtn, resurrectBtn, webBtn)
	headerRightScroll := container.NewHScroll(headerRightBox)
	topBar := container.NewBorder(nil, nil, headerLeft, nil, headerRightScroll)

	// Top container combining Top Bar and Group Selection / Filter Bar
	topContainer := container.NewVBox(
		topBar,
		widget.NewSeparator(),
		groupBarContainer,
		widget.NewSeparator(),
	)

	// Scroll container for service list (vertical only, so column 0 is never scrolled away)
	servicesScroll := container.NewVScroll(servicesListContainer)

	// Master layout: Split between Service List and Logs Panel
	split := container.NewVSplit(servicesScroll, logPanel)
	split.SetOffset(0.65) // 65% services, 35% logs

	mainLayout := container.NewBorder(topContainer, nil, nil, nil, split)

	// --- App Tabs: Tab 1 (Process Manager), Tab 2 (Mouse Jiggler), Tab 3 (Credentials & Clipboard), Tab 4 (Tasks & Todo), Tab 5 (Downloader), Tab 6 (Training Hub) ---
	pmTab := container.NewTabItemWithIcon("⚡ Process Manager", theme.ComputerIcon(), mainLayout)
	jigglerContent, updateJigglerTelemetry := buildJigglerTab(w, a)
	jigglerTab := container.NewTabItemWithIcon("🐭 Mouse Jiggler", theme.MediaPlayIcon(), jigglerContent)
	credContent, _ := buildCredentialsTab(w, a)
	credTab := container.NewTabItemWithIcon("🔐 Vault & Clipboard", theme.AccountIcon(), credContent)
	todoContent, _ := buildTodoTab(w, a)
	todoTab := container.NewTabItemWithIcon("📝 Tasks", theme.CheckButtonCheckedIcon(), todoContent)
	ytdlContent, refreshYtdl := buildYtDlpTab(w, a)
	ytdlTab := container.NewTabItemWithIcon("📥 Downloader", theme.DownloadIcon(), ytdlContent)
	gameTab, gameCtrl := buildDodgeGameTab(w, a, port)

	appTabs := container.NewAppTabs(pmTab, jigglerTab, credTab, todoTab, ytdlTab, gameTab)
	appTabs.SetTabLocation(container.TabLocationTop)

	var currentSelectedTab *container.TabItem = pmTab
	appTabs.OnSelected = func(tab *container.TabItem) {
		currentSelectedTab = tab
		if tab == pmTab {
			renderServices()
			refreshLogs()
		}
		if tab == ytdlTab && refreshYtdl != nil {
			refreshYtdl()
		}
		if gameCtrl != nil {
			gameCtrl.SetTabVisible(tab == gameTab)
		}
	}
	w.SetOnClosed(func() {
		if gameCtrl != nil {
			gameCtrl.Destroy()
		}
	})
	w.SetContent(appTabs)

	// Enforce compact geometry, center on screen, and run Win32 desktop bounds clamp
	w.Resize(fyne.NewSize(980, 580))
	w.CenterOnScreen()
	ensureWindowInBounds("Windows Process Manager & Service Supervisor (Native Go)", 980, 580)

	// Initial render
	renderServices()

	// Background ticker for live metric updates & anti-idle countdowns
	go func() {
		ticker := time.NewTicker(1000 * time.Millisecond)
		defer ticker.Stop()

		tickCount := 0
		for range ticker.C {
			tickCount++
			fyne.Do(func() {
				if currentSelectedTab == jigglerTab && updateJigglerTelemetry != nil {
					updateJigglerTelemetry()
				}
				if currentSelectedTab == pmTab && tickCount%2 == 0 {
					renderServices()
					refreshLogs()
				}
			})
		}
	}()

	w.ShowAndRun()
	return nil
}
