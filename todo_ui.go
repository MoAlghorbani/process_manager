package main

import (
	"fmt"
	"image/color"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// parseNotificationPreset calculates a time.Time based on preset choice or custom string.
func parseNotificationPreset(choice, customStr string) *time.Time {
	now := time.Now()
	switch choice {
	case "In 5 minutes":
		t := now.Add(5 * time.Minute)
		return &t
	case "In 15 minutes":
		t := now.Add(15 * time.Minute)
		return &t
	case "In 30 minutes":
		t := now.Add(30 * time.Minute)
		return &t
	case "In 1 hour":
		t := now.Add(1 * time.Hour)
		return &t
	case "In 2 hours":
		t := now.Add(2 * time.Hour)
		return &t
	case "Today at 18:00":
		t := time.Date(now.Year(), now.Month(), now.Day(), 18, 0, 0, 0, now.Location())
		if t.Before(now) {
			t = t.Add(24 * time.Hour)
		}
		return &t
	case "Tomorrow at 09:00":
		tomorrow := now.Add(24 * time.Hour)
		t := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 9, 0, 0, 0, now.Location())
		return &t
	case "Custom (YYYY-MM-DD HH:MM)":
		clean := strings.TrimSpace(customStr)
		if clean == "" {
			return nil
		}
		layouts := []string{
			"2006-01-02 15:04",
			"2006/01/02 15:04",
			"15:04",
			"01/02 15:04",
		}
		for _, layout := range layouts {
			if parsed, err := time.ParseInLocation(layout, clean, now.Location()); err == nil {
				if layout == "15:04" {
					parsed = time.Date(now.Year(), now.Month(), now.Day(), parsed.Hour(), parsed.Minute(), 0, 0, now.Location())
					if parsed.Before(now) {
						parsed = parsed.Add(24 * time.Hour)
					}
				}
				return &parsed
			}
		}
		return nil
	default:
		return nil
	}
}

// buildTodoTab constructs the full Kanban Task Board with modal creation, project tagging, and notifications.
func buildTodoTab(w fyne.Window, a fyne.App) (fyne.CanvasObject, func()) {
	ts := GetTodoStore()

	var (
		searchQuery      string
		projectFilter    = "All Projects"
		searchMu         sync.Mutex
		searchTimer      *time.Timer
		projFilterSelect *widget.Select
	)

	// Summary telemetry
	summaryLbl := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})

	// Column card containers
	todoCards := container.NewVBox()
	inProgCards := container.NewVBox()
	doneCards := container.NewVBox()

	// Column headers with dynamic badges
	todoHeaderLbl := widget.NewLabelWithStyle("📋 TO DO (0)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	inProgHeaderLbl := widget.NewLabelWithStyle("🚧 IN PROGRESS (0)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	doneHeaderLbl := widget.NewLabelWithStyle("✅ DONE (0)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	var renderKanban func()

	// Modal Dialog: Add Task with Project and Notification
	openAddTaskModal := func(defaultColumn string) {
		titleEntry := widget.NewEntry()
		titleEntry.SetPlaceHolder("What needs to be done?")

		// Projects dropdown + custom entry
		projects := ts.GetProjects()
		projectSelect := widget.NewSelect(projects, nil)
		if len(projects) > 0 {
			projectSelect.SetSelected("General")
		}

		customProjectEntry := widget.NewEntry()
		customProjectEntry.SetPlaceHolder("Or type a new project name...")

		// Column selector
		colSelect := widget.NewSelect([]string{"To Do", "In Progress", "Done"}, nil)
		switch defaultColumn {
		case StatusInProgress:
			colSelect.SetSelected("In Progress")
		case StatusDone:
			colSelect.SetSelected("Done")
		default:
			colSelect.SetSelected("To Do")
		}

		// Priority selector
		prioritySelect := widget.NewSelect([]string{"High", "Normal", "Low"}, nil)
		prioritySelect.SetSelected("Normal")

		// Notification reminder selector
		notifyOptions := []string{
			"No Notification",
			"In 5 minutes",
			"In 15 minutes",
			"In 30 minutes",
			"In 1 hour",
			"In 2 hours",
			"Today at 18:00",
			"Tomorrow at 09:00",
			"Custom (YYYY-MM-DD HH:MM)",
		}
		notifySelect := widget.NewSelect(notifyOptions, nil)
		notifySelect.SetSelected("No Notification")

		customNotifyEntry := widget.NewEntry()
		customNotifyEntry.SetPlaceHolder("e.g. 2026-10-12 18:30 or 18:30")

		formItems := []*widget.FormItem{
			{Text: "Task Title *", Widget: titleEntry},
			{Text: "Select Project", Widget: projectSelect},
			{Text: "New Project (Optional)", Widget: customProjectEntry},
			{Text: "Initial Column", Widget: colSelect},
			{Text: "Priority", Widget: prioritySelect},
			{Text: "⏰ Notification Reminder", Widget: notifySelect},
			{Text: "Custom Reminder Time", Widget: customNotifyEntry},
		}

		d := dialog.NewForm("➕ Add New Task", "Create Task", "Cancel", formItems, func(ok bool) {
			if !ok {
				return
			}
			title := strings.TrimSpace(titleEntry.Text)
			if title == "" {
				dialog.ShowInformation("Validation Error", "Task Title is required.", w)
				return
			}

			// Determine project
			proj := projectSelect.Selected
			if proj == "" {
				proj = "General"
			}
			if custom := strings.TrimSpace(customProjectEntry.Text); custom != "" {
				proj = custom
			}

			// Determine status
			var targetCol string
			switch colSelect.Selected {
			case "In Progress":
				targetCol = StatusInProgress
			case "Done":
				targetCol = StatusDone
			default:
				targetCol = StatusTodo
			}

			// Parse notification
			notifyAt := parseNotificationPreset(notifySelect.Selected, customNotifyEntry.Text)

			item, err := ts.AddWithDetails(title, proj, strings.ToLower(prioritySelect.Selected), targetCol, notifyAt)
			if err != nil {
				dialog.ShowError(err, w)
				return
			}

			// Feedback notification if reminder was set
			if notifyAt != nil {
				a.SendNotification(&fyne.Notification{
					Title:   "Reminder Scheduled",
					Content: fmt.Sprintf("Will notify you for task #%s at %s", item.ID, notifyAt.Format("15:04")),
				})
			}

			// Ensure current filter will show the new task
			if !isAllProjects(projectFilter) && !strings.EqualFold(projectFilter, proj) {
				projectFilter = "All Projects"
			}
			if projFilterSelect != nil {
				allProjOptions := append([]string{"All Projects"}, ts.GetProjects()...)
				projFilterSelect.Options = allProjOptions
				projFilterSelect.SetSelected(projectFilter)
				projFilterSelect.Refresh()
			}

			renderKanban()
		}, w)

		d.Resize(fyne.NewSize(520, 420))
		d.Show()
	}

	// Modal Dialog: Edit Task
	openEditDialog := func(it TodoItem) {
		titleEntry := widget.NewEntry()
		titleEntry.SetText(it.Title)

		projects := ts.GetProjects()
		projectSelect := widget.NewSelect(projects, nil)
		projectSelect.SetSelected(it.Project)

		customProjectEntry := widget.NewEntry()
		customProjectEntry.SetPlaceHolder("Or type a new project name...")

		colSelect := widget.NewSelect([]string{"To Do", "In Progress", "Done"}, nil)
		switch it.Status {
		case StatusInProgress:
			colSelect.SetSelected("In Progress")
		case StatusDone:
			colSelect.SetSelected("Done")
		default:
			colSelect.SetSelected("To Do")
		}

		pSelect := widget.NewSelect([]string{"High", "Normal", "Low"}, nil)
		switch strings.ToLower(it.Priority) {
		case "high":
			pSelect.SetSelected("High")
		case "low":
			pSelect.SetSelected("Low")
		default:
			pSelect.SetSelected("Normal")
		}

		notifyOptions := []string{
			"Keep Current",
			"No Notification",
			"In 5 minutes",
			"In 15 minutes",
			"In 30 minutes",
			"In 1 hour",
			"Today at 18:00",
			"Tomorrow at 09:00",
			"Custom (YYYY-MM-DD HH:MM)",
		}
		notifySelect := widget.NewSelect(notifyOptions, nil)
		notifySelect.SetSelected("Keep Current")

		customNotifyEntry := widget.NewEntry()
		if it.NotifyAt != nil {
			customNotifyEntry.SetText(it.NotifyAt.Format("2006-01-02 15:04"))
		}
		customNotifyEntry.SetPlaceHolder("YYYY-MM-DD HH:MM")
		customNotifyEntry.Hide()

		notifySelect.OnChanged = func(s string) {
			if s == "Custom (YYYY-MM-DD HH:MM)" {
				customNotifyEntry.Show()
			} else {
				customNotifyEntry.Hide()
			}
		}

		formItems := []*widget.FormItem{
			{Text: "Task Title *", Widget: titleEntry},
			{Text: "Project", Widget: projectSelect},
			{Text: "Rename / New Project", Widget: customProjectEntry},
			{Text: "Column", Widget: colSelect},
			{Text: "Priority", Widget: pSelect},
			{Text: "⏰ Reminder", Widget: notifySelect},
			{Text: "Custom Time", Widget: customNotifyEntry},
		}

		d := dialog.NewForm("✏️ Edit Task", "Save Changes", "Cancel", formItems, func(ok bool) {
			if !ok {
				return
			}
			newTitle := strings.TrimSpace(titleEntry.Text)
			if newTitle == "" {
				return
			}
			proj := projectSelect.Selected
			if custom := strings.TrimSpace(customProjectEntry.Text); custom != "" {
				proj = custom
			}

			var newCol string
			switch colSelect.Selected {
			case "In Progress":
				newCol = StatusInProgress
			case "Done":
				newCol = StatusDone
			default:
				newCol = StatusTodo
			}

			updateNotify := false
			var newNotifyAt *time.Time
			if notifySelect.Selected != "Keep Current" {
				updateNotify = true
				newNotifyAt = parseNotificationPreset(notifySelect.Selected, customNotifyEntry.Text)
			}

			_, _ = ts.UpdateDetails(it.ID, newTitle, proj, strings.ToLower(pSelect.Selected), newCol, newNotifyAt, updateNotify)
			renderKanban()
		}, w)

		d.Resize(fyne.NewSize(520, 420))
		d.Show()
	}

	// Helper to build a single card widget
	buildCard := func(it TodoItem) fyne.CanvasObject {
		// 1. Top row: #ID + Project badge + Priority badge
		idText := canvas.NewText(fmt.Sprintf("#%s", it.ID), color.RGBA{R: 100, G: 116, B: 139, A: 255})
		idText.TextSize = 11
		idText.TextStyle = fyne.TextStyle{Monospace: true}

		projText := it.Project
		if len(projText) > 25 {
			projText = projText[:22] + "..."
		}
		projBadge := canvas.NewText(fmt.Sprintf("📁 %s", projText), color.RGBA{R: 56, G: 189, B: 248, A: 255})
		projBadge.TextSize = 11
		projBadge.TextStyle = fyne.TextStyle{Bold: true}

		var priBadge fyne.CanvasObject
		switch strings.ToLower(it.Priority) {
		case "high":
			t := canvas.NewText("🔥 HIGH", color.RGBA{R: 249, G: 115, B: 22, A: 255})
			t.TextSize = 11
			t.TextStyle = fyne.TextStyle{Bold: true}
			priBadge = t
		case "low":
			t := canvas.NewText("💤 LOW", color.RGBA{R: 148, G: 163, B: 184, A: 255})
			t.TextSize = 11
			priBadge = t
		default:
			t := canvas.NewText("• Normal", color.RGBA{R: 100, G: 116, B: 139, A: 255})
			t.TextSize = 11
			priBadge = t
		}

		timeStr := it.CreatedAt.Format("01/02 15:04")
		if it.Status == StatusDone && it.CompletedAt != nil {
			timeStr = fmt.Sprintf("Done %s", it.CompletedAt.Format("15:04"))
		}
		timeText := canvas.NewText(timeStr, color.RGBA{R: 100, G: 116, B: 139, A: 255})
		timeText.TextSize = 10

		topRow := container.NewBorder(nil, nil, container.NewHBox(idText, projBadge, priBadge), timeText)

		// 2. Notification badge (if scheduled or overdue)
		var notifyBadge fyne.CanvasObject
		if it.NotifyAt != nil && it.Status != StatusDone {
			now := time.Now()
			isOverdue := now.After(*it.NotifyAt)
			formattedTime := it.NotifyAt.Format("01/02 15:04")
			if it.NotifyAt.Day() == now.Day() {
				formattedTime = it.NotifyAt.Format("Today 15:04")
			}

			if isOverdue {
				nb := canvas.NewText(fmt.Sprintf("⏰ OVERDUE: %s", formattedTime), color.RGBA{R: 239, G: 68, B: 68, A: 255})
				nb.TextSize = 11
				nb.TextStyle = fyne.TextStyle{Bold: true}
				notifyBadge = nb
			} else {
				nb := canvas.NewText(fmt.Sprintf("⏰ Reminder: %s", formattedTime), color.RGBA{R: 245, G: 158, B: 11, A: 255})
				nb.TextSize = 11
				notifyBadge = nb
			}
		}

		// 3. Title Label (high contrast, word-wrap)
		titleLbl := widget.NewLabel(it.Title)
		titleLbl.Wrapping = fyne.TextWrapWord
		if it.Status == StatusDone {
			titleLbl.TextStyle = fyne.TextStyle{Italic: true}
		} else {
			titleLbl.TextStyle = fyne.TextStyle{Bold: true}
		}

		// 4. Action Buttons based on Column
		actionsBox := container.NewHBox()

		switch it.Status {
		case StatusTodo:
			startBtn := widget.NewButtonWithIcon("Start", theme.MediaPlayIcon(), func() {
				_, _ = ts.SetStatus(it.ID, StatusInProgress)
				renderKanban()
			})
			startBtn.Importance = widget.HighImportance

			doneBtn := widget.NewButtonWithIcon("Done", theme.ConfirmIcon(), func() {
				_, _ = ts.SetStatus(it.ID, StatusDone)
				renderKanban()
			})

			editBtn := widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), func() {
				openEditDialog(it)
			})

			delBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
				ts.Delete(it.ID)
				renderKanban()
			})

			actionsBox.Add(startBtn)
			actionsBox.Add(doneBtn)
			actionsBox.Add(editBtn)
			actionsBox.Add(delBtn)

		case StatusInProgress:
			backBtn := widget.NewButtonWithIcon("Back", theme.NavigateBackIcon(), func() {
				_, _ = ts.SetStatus(it.ID, StatusTodo)
				renderKanban()
			})

			doneBtn := widget.NewButtonWithIcon("Done", theme.ConfirmIcon(), func() {
				_, _ = ts.SetStatus(it.ID, StatusDone)
				renderKanban()
			})
			doneBtn.Importance = widget.HighImportance

			editBtn := widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), func() {
				openEditDialog(it)
			})

			delBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
				ts.Delete(it.ID)
				renderKanban()
			})

			actionsBox.Add(backBtn)
			actionsBox.Add(doneBtn)
			actionsBox.Add(editBtn)
			actionsBox.Add(delBtn)

		case StatusDone:
			reopenBtn := widget.NewButtonWithIcon("Reopen", theme.ViewRefreshIcon(), func() {
				_, _ = ts.SetStatus(it.ID, StatusInProgress)
				renderKanban()
			})

			editBtn := widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), func() {
				openEditDialog(it)
			})

			delBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
				ts.Delete(it.ID)
				renderKanban()
			})

			actionsBox.Add(reopenBtn)
			actionsBox.Add(editBtn)
			actionsBox.Add(delBtn)
		}

		cardChildren := []fyne.CanvasObject{topRow}
		if notifyBadge != nil {
			cardChildren = append(cardChildren, notifyBadge)
		}
		cardChildren = append(cardChildren, titleLbl, container.NewHBox(actionsBox), widget.NewSeparator())

		return container.NewVBox(cardChildren...)
	}

	renderKanban = func() {
		searchMu.Lock()
		query := searchQuery
		pFilter := projectFilter
		searchMu.Unlock()

		todoItems := ts.ListByColumn(StatusTodo, pFilter, query)
		inProgItems := ts.ListByColumn(StatusInProgress, pFilter, query)
		doneItems := ts.ListByColumn(StatusDone, pFilter, query)

		todoCount, inProgCount, doneCount := ts.ColumnCounts(pFilter)
		total := todoCount + inProgCount + doneCount

		filterText := ""
		if pFilter != "" && pFilter != "All Projects" {
			filterText = fmt.Sprintf(" [%s]", pFilter)
		}
		summaryLbl.SetText(fmt.Sprintf("%s [%d To Do • %d In Progress • %d Done • %d Total]", filterText, todoCount, inProgCount, doneCount, total))

		todoHeaderLbl.SetText(fmt.Sprintf("📋 TO DO (%d)", len(todoItems)))
		inProgHeaderLbl.SetText(fmt.Sprintf("🚧 IN PROGRESS (%d)", len(inProgItems)))
		doneHeaderLbl.SetText(fmt.Sprintf("✅ DONE (%d)", len(doneItems)))

		// 1. Populate Todo Column
		todoCards.Objects = nil
		if len(todoItems) == 0 {
			empty := widget.NewLabelWithStyle("No tasks in To Do", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})
			todoCards.Add(container.NewPadded(empty))
		} else {
			for _, it := range todoItems {
				todoCards.Add(buildCard(it))
			}
		}
		todoCards.Refresh()

		// 2. Populate In Progress Column
		inProgCards.Objects = nil
		if len(inProgItems) == 0 {
			empty := widget.NewLabelWithStyle("No tasks in progress", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})
			inProgCards.Add(container.NewPadded(empty))
		} else {
			for _, it := range inProgItems {
				inProgCards.Add(buildCard(it))
			}
		}
		inProgCards.Refresh()

		// 3. Populate Done Column
		doneCards.Objects = nil
		if len(doneItems) == 0 {
			empty := widget.NewLabelWithStyle("No completed tasks", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})
			doneCards.Add(container.NewPadded(empty))
		} else {
			for _, it := range doneItems {
				doneCards.Add(buildCard(it))
			}
		}
		doneCards.Refresh()
	}

	// Top Toolbar: Big "➕ Add Task" button that opens Modal
	bigAddBtn := widget.NewButtonWithIcon("➕ Add Task (Modal)", theme.ContentAddIcon(), func() {
		openAddTaskModal(StatusTodo)
	})
	bigAddBtn.Importance = widget.HighImportance

	// Project filter dropdown
	allProjOptions := append([]string{"All Projects"}, ts.GetProjects()...)
	projFilterSelect = widget.NewSelect(allProjOptions, func(s string) {
		projectFilter = s
		renderKanban()
	})
	projFilterSelect.SetSelected("All Projects")

	// Search filter
	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("🔍 Filter tasks or projects...")
	searchEntry.OnChanged = func(s string) {
		searchMu.Lock()
		searchQuery = s
		if searchTimer != nil {
			searchTimer.Stop()
		}
		searchTimer = time.AfterFunc(150*time.Millisecond, func() {
			fyne.Do(renderKanban)
		})
		searchMu.Unlock()
	}

	clearDoneBtn := widget.NewButtonWithIcon("Clear Done", theme.DeleteIcon(), func() {
		_, _, done := ts.ColumnCounts(projectFilter)
		if done == 0 {
			dialog.ShowInformation("Kanban", "No completed tasks to clear.", w)
			return
		}
		dialog.ShowConfirm("Clear Done Tasks", fmt.Sprintf("Remove all %d completed tasks from Done?", done), func(ok bool) {
			if ok {
				ts.ClearCompleted()
				renderKanban()
			}
		}, w)
	})

	refreshBtn := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		allProjOptions := append([]string{"All Projects"}, ts.GetProjects()...)
		projFilterSelect.Options = allProjOptions
		projFilterSelect.Refresh()
		renderKanban()
	})

	topHeader := container.NewBorder(
		nil, nil,
		container.NewHBox(
			widget.NewLabelWithStyle("📋 Kanban Board", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			summaryLbl,
		),
		container.NewHBox(bigAddBtn, refreshBtn, clearDoneBtn),
	)

	filterRow := container.NewBorder(
		nil, nil,
		container.NewHBox(widget.NewLabel("Filter:"), projFilterSelect),
		nil,
		searchEntry,
	)

	topSection := container.NewVBox(
		topHeader,
		filterRow,
		widget.NewSeparator(),
	)

	// Column card frame helper with [+] button
	makeColumn := func(header *widget.Label, colStatus string, scroll *container.Scroll, headerColor color.Color) fyne.CanvasObject {
		colorBar := canvas.NewRectangle(headerColor)
		colorBar.SetMinSize(fyne.NewSize(0, 3))

		addColBtn := widget.NewButtonWithIcon("", theme.ContentAddIcon(), func() {
			openAddTaskModal(colStatus)
		})

		headerRow := container.NewBorder(nil, nil, header, addColBtn)

		colTop := container.NewVBox(
			headerRow,
			colorBar,
		)
		return container.NewBorder(colTop, nil, nil, nil, scroll)
	}

	col1 := makeColumn(todoHeaderLbl, StatusTodo, container.NewScroll(todoCards), color.RGBA{R: 56, G: 189, B: 248, A: 255})
	col2 := makeColumn(inProgHeaderLbl, StatusInProgress, container.NewScroll(inProgCards), color.RGBA{R: 245, G: 158, B: 11, A: 255})
	col3 := makeColumn(doneHeaderLbl, StatusDone, container.NewScroll(doneCards), color.RGBA{R: 16, G: 185, B: 129, A: 255})

	kanbanColumns := container.NewGridWithColumns(3, col1, col2, col3)
	kanbanScroll := container.NewScroll(kanbanColumns)
	mainContent := container.NewBorder(topSection, nil, nil, nil, kanbanScroll)

	// Background notification daemon (checks every 10 seconds for due tasks and fires Windows notifications)
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			dueItems := ts.CheckPendingNotifications()
			if len(dueItems) > 0 {
				fyne.Do(func() {
					for _, it := range dueItems {
						a.SendNotification(&fyne.Notification{
							Title:   fmt.Sprintf("⏰ Task Reminder: %s", it.Title),
							Content: fmt.Sprintf("[%s] Project: %s (#%s)", strings.ToUpper(it.Priority), it.Project, it.ID),
						})
					}
					renderKanban()
				})
			}
		}
	}()

	// Initial render
	renderKanban()

	return mainContent, func() {
		renderKanban()
	}
}
