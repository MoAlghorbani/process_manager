package main

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// buildJigglerTab constructs the native Fyne GUI tab for Mouse Jiggler & Anti-Idle Guard.
func buildJigglerTab(w fyne.Window, a fyne.App) (fyne.CanvasObject, func()) {
	je := GetJigglerEngine()
	cfg := je.GetConfig()

	// --- Top Header & Banner ---
	titleLbl := widget.NewLabelWithStyle("🐭 Windows Mouse Jiggler & Anti-Idle Suite", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	descLbl := widget.NewLabel("Keeps your screen active, display awake, and prevents 'Away' or 'Inactive' status in Microsoft Teams, Slack, and Zoom.")

	statusCanvas := canvas.NewText("● STOPPED / IDLE", color.RGBA{R: 239, G: 68, B: 68, A: 255})
	statusCanvas.TextStyle = fyne.TextStyle{Bold: true}
	statusCanvas.TextSize = 14

	var (
		startBtn *widget.Button
		stopBtn  *widget.Button
	)

	updateStatusBadge := func() {
		running := je.IsRunning()
		if running {
			statusCanvas.Text = "● ACTIVE - Jiggler Running"
			statusCanvas.Color = color.RGBA{R: 16, G: 185, B: 129, A: 255} // emerald
			startBtn.Disable()
			stopBtn.Enable()
		} else {
			statusCanvas.Text = "● STOPPED / IDLE"
			statusCanvas.Color = color.RGBA{R: 239, G: 68, B: 68, A: 255} // red
			startBtn.Enable()
			stopBtn.Disable()
		}
		statusCanvas.Refresh()
	}

	startBtn = widget.NewButtonWithIcon("▶ Start Jiggler", theme.MediaPlayIcon(), func() {
		je.Start()
		updateStatusBadge()
		a.SendNotification(&fyne.Notification{
			Title:   "Mouse Jiggler",
			Content: "⚡ Anti-Idle Jiggler started",
		})
	})
	startBtn.Importance = widget.HighImportance

	stopBtn = widget.NewButtonWithIcon("⏹ Stop Jiggler", theme.MediaStopIcon(), func() {
		je.Stop()
		updateStatusBadge()
		a.SendNotification(&fyne.Notification{
			Title:   "Mouse Jiggler",
			Content: "⏹ Anti-Idle Jiggler stopped",
		})
	})
	stopBtn.Disable()

	btnRow := container.NewHBox(statusCanvas, layout.NewSpacer(), startBtn, stopBtn)
	headerCard := widget.NewCard("", "", container.NewVBox(titleLbl, descLbl, widget.NewSeparator(), btnRow))

	// --- Left Panel: Settings Form ---
	// Shortcut Settings
	shortcutEntry := widget.NewEntry()
	shortcutEntry.SetPlaceHolder("e.g. Ctrl+Alt+J, F8, Ctrl+Shift+M")
	shortcutEntry.SetText(cfg.Shortcut)

	clearShortcutBtn := widget.NewButtonWithIcon("", theme.ContentClearIcon(), func() {
		shortcutEntry.SetText("")
	})
	shortcutRow := container.NewBorder(nil, nil, nil, clearShortcutBtn, shortcutEntry)

	// Mouse Settings
	mouseCheck := widget.NewCheck("Enable Periodic Mouse Movement", nil)
	mouseCheck.SetChecked(cfg.MouseEnabled)

	mouseIntervalEntry := widget.NewEntry()
	mouseIntervalEntry.SetText(strconv.Itoa(cfg.MouseIntervalSec))

	modeSelect := widget.NewSelect([]string{
		"Stealth Zen (1px Micro-nudge)",
		"Gentle Nudge (Visible Step)",
		"Orbit Loop (Circular Return)",
		"Sleep Guard Only (No Mouse Movement)",
	}, nil)

	switch cfg.MouseMoveMode {
	case "gentle":
		modeSelect.SetSelected("Gentle Nudge (Visible Step)")
	case "orbit":
		modeSelect.SetSelected("Orbit Loop (Circular Return)")
	case "api_only":
		modeSelect.SetSelected("Sleep Guard Only (No Mouse Movement)")
	default:
		modeSelect.SetSelected("Orbit Loop (Circular Return)")
	}

	// Keyboard Settings
	keyCheck := widget.NewCheck("Enable Periodic Anti-Idle Keystroke", nil)
	keyCheck.SetChecked(cfg.KeyEnabled)

	keyIntervalEntry := widget.NewEntry()
	keyIntervalEntry.SetText(strconv.Itoa(cfg.KeyIntervalSec))

	keySelect := widget.NewSelect([]string{
		"F15 (Ghost Key - Invisible & Non-Intrusive)",
		"Shift Key (Modifier Tap)",
		"Control Key (Modifier Tap)",
		"Scroll Lock (Toggle)",
	}, nil)

	switch cfg.KeyChoice {
	case "shift":
		keySelect.SetSelected("Shift Key (Modifier Tap)")
	case "control":
		keySelect.SetSelected("Control Key (Modifier Tap)")
	case "scroll":
		keySelect.SetSelected("Scroll Lock (Toggle)")
	default:
		keySelect.SetSelected("F15 (Ghost Key - Invisible & Non-Intrusive)")
	}

	// Smart User-Presence Guard
	guardCheck := widget.NewCheck("Pause jiggling while user is actively working", nil)
	guardCheck.SetChecked(cfg.IdleGuardEnabled)

	thresholdEntry := widget.NewEntry()
	thresholdEntry.SetText(strconv.Itoa(cfg.IdleThresholdSec))

	keepAwakeCheck := widget.NewCheck("Keep Windows Display & System Awake (Win32 API)", nil)
	keepAwakeCheck.SetChecked(cfg.KeepAwakeEnabled)

	applyConfig := func() {
		mInt, _ := strconv.Atoi(strings.TrimSpace(mouseIntervalEntry.Text))
		if mInt <= 0 {
			mInt = 5
		}
		kInt, _ := strconv.Atoi(strings.TrimSpace(keyIntervalEntry.Text))
		if kInt <= 0 {
			kInt = 30
		}
		thInt, _ := strconv.Atoi(strings.TrimSpace(thresholdEntry.Text))
		if thInt <= 0 {
			thInt = 15
		}

		mMode := "orbit"
		switch modeSelect.Selected {
		case "Stealth Zen (1px Micro-nudge)":
			mMode = "zen"
		case "Gentle Nudge (Visible Step)":
			mMode = "gentle"
		case "Orbit Loop (Circular Return)":
			mMode = "orbit"
		case "Sleep Guard Only (No Mouse Movement)":
			mMode = "api_only"
		default:
			mMode = "orbit"
		}

		kChoice := "f15"
		switch keySelect.Selected {
		case "Shift Key (Modifier Tap)":
			kChoice = "shift"
		case "Control Key (Modifier Tap)":
			kChoice = "control"
		case "Scroll Lock (Toggle)":
			kChoice = "scroll"
		default:
			kChoice = "f15"
		}

		newCfg := JigglerConfig{
			Shortcut:         strings.TrimSpace(shortcutEntry.Text),
			MouseEnabled:     mouseCheck.Checked,
			MouseIntervalSec: mInt,
			MouseMoveMode:    mMode,
			KeyEnabled:       keyCheck.Checked,
			KeyIntervalSec:   kInt,
			KeyChoice:        kChoice,
			IdleGuardEnabled: guardCheck.Checked,
			IdleThresholdSec: thInt,
			KeepAwakeEnabled: keepAwakeCheck.Checked,
		}

		je.UpdateConfig(newCfg)
		if ghm := GetGlobalHotkeyManager(nil); ghm != nil {
			ghm.Reload()
		}
	}

	// Real-time live updates
	shortcutEntry.OnChanged = func(string) { applyConfig() }
	mouseCheck.OnChanged = func(bool) { applyConfig() }
	mouseIntervalEntry.OnChanged = func(string) { applyConfig() }
	modeSelect.OnChanged = func(string) { applyConfig() }
	keyCheck.OnChanged = func(bool) { applyConfig() }
	keyIntervalEntry.OnChanged = func(string) { applyConfig() }
	keySelect.OnChanged = func(string) { applyConfig() }
	guardCheck.OnChanged = func(bool) { applyConfig() }
	thresholdEntry.OnChanged = func(string) { applyConfig() }
	keepAwakeCheck.OnChanged = func(bool) { applyConfig() }

	saveBtn := widget.NewButtonWithIcon("💾 Save Settings", theme.DocumentSaveIcon(), func() {
		applyConfig()
		dialog.ShowInformation("Settings Saved", "Mouse Jiggler configuration updated and saved!", w)
	})

	settingsForm := widget.NewForm(
		widget.NewFormItem("Toggle Shortcut", shortcutRow),
		widget.NewFormItem("Mouse Jiggler", mouseCheck),
		widget.NewFormItem("Movement Mode", modeSelect),
		widget.NewFormItem("Move Every (seconds)", mouseIntervalEntry),
		widget.NewFormItem("Keystroke Anti-Idle", keyCheck),
		widget.NewFormItem("Key to Tap", keySelect),
		widget.NewFormItem("Tap Every (seconds)", keyIntervalEntry),
		widget.NewFormItem("User-Presence Guard", guardCheck),
		widget.NewFormItem("Idle Wait (seconds)", thresholdEntry),
		widget.NewFormItem("Win32 Sleep Guard", keepAwakeCheck),
	)

	settingsCard := widget.NewCard("⚙️ Configuration", "Customize intervals, movement behavior, and keys", container.NewVBox(
		settingsForm,
		widget.NewSeparator(),
		saveBtn,
	))

	// --- Right Panel: Telemetry & Manual Tests ---
	teleStatusVal := widget.NewLabelWithStyle("Stopped", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	teleNextMoveVal := widget.NewLabel("-")
	teleNextKeyVal := widget.NewLabel("-")
	teleTotalMovesVal := widget.NewLabel("0")
	teleTotalKeysVal := widget.NewLabel("0")
	teleIdleVal := widget.NewLabel("0.0s")
	teleLastActionVal := widget.NewLabel("Ready")

	testMouseBtn := widget.NewButtonWithIcon("🖱️ Test Mouse Jiggle Now", theme.MoveDownIcon(), func() {
		mMode := "zen"
		switch modeSelect.Selected {
		case "Gentle Nudge (4px Oscillation)":
			mMode = "gentle"
		case "Orbit Loop (Circular Return)":
			mMode = "orbit"
		case "Sleep Guard Only (No Mouse Movement)":
			mMode = "api_only"
		default:
			mMode = "zen"
		}
		je.performMouseJiggle(mMode)
		a.SendNotification(&fyne.Notification{
			Title:   "Jiggler Test",
			Content: fmt.Sprintf("Fired test mouse jiggle (%s)", mMode),
		})
	})

	testKeyBtn := widget.NewButtonWithIcon("⌨️ Test Key Tap Now", theme.ContentAddIcon(), func() {
		kChoice := "f15"
		switch keySelect.Selected {
		case "Shift Key (Modifier Tap)":
			kChoice = "shift"
		case "Control Key (Modifier Tap)":
			kChoice = "control"
		case "Scroll Lock (Toggle)":
			kChoice = "scroll"
		default:
			kChoice = "f15"
		}
		je.performKeyTap(kChoice)
		a.SendNotification(&fyne.Notification{
			Title:   "Jiggler Test",
			Content: fmt.Sprintf("Fired test key tap (%s)", kChoice),
		})
	})

	telemetryForm := widget.NewForm(
		widget.NewFormItem("Engine Status", teleStatusVal),
		widget.NewFormItem("User Inactive For", teleIdleVal),
		widget.NewFormItem("Next Mouse Move In", teleNextMoveVal),
		widget.NewFormItem("Next Key Tap In", teleNextKeyVal),
		widget.NewFormItem("Total Mouse Jiggles", teleTotalMovesVal),
		widget.NewFormItem("Total Keystrokes", teleTotalKeysVal),
		widget.NewFormItem("Latest Action", teleLastActionVal),
	)

	telemetryCard := widget.NewCard("📊 Live Telemetry & Test", "Real-time activity counters & manual test triggers", container.NewVBox(
		telemetryForm,
		widget.NewSeparator(),
		container.NewGridWithColumns(2, testMouseBtn, testKeyBtn),
	))

	// Main 2-column layout
	columns := container.NewGridWithColumns(2, settingsCard, telemetryCard)
	content := container.NewVBox(headerCard, widget.NewSeparator(), columns)
	scrollable := container.NewScroll(content)

	// In-place telemetry updater called from the main background ticker
	updateTelemetry := func() {
		stats := je.GetStats()
		if stats.Running {
			teleStatusVal.SetText("Running")
			teleNextMoveVal.SetText(fmt.Sprintf("%ds", stats.NextMouseMoveSec))
			teleNextKeyVal.SetText(fmt.Sprintf("%ds", stats.NextKeyTapSec))
		} else {
			teleStatusVal.SetText("Stopped")
			teleNextMoveVal.SetText("-")
			teleNextKeyVal.SetText("-")
		}
		teleTotalMovesVal.SetText(strconv.Itoa(stats.TotalMouseMoves))
		teleTotalKeysVal.SetText(strconv.Itoa(stats.TotalKeyTaps))
		teleIdleVal.SetText(fmt.Sprintf("%.1fs", stats.CurrentIdleSec))
		if stats.LastActionMsg != "" {
			teleLastActionVal.SetText(stats.LastActionMsg)
		}
		updateStatusBadge()
	}

	return scrollable, updateTelemetry
}
