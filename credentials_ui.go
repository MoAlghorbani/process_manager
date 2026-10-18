package main

import (
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// buildCredentialsTab constructs the native Fyne GUI tab for Credential Manager & Permanent Clipboard History.
func buildCredentialsTab(w fyne.Window, a fyne.App) (fyne.CanvasObject, func()) {
	cs := GetCredentialStore()
	cw := GetClipboardWatcher()

	// -------------------------------------------------------------
	// SECTION 1: CREDENTIALS VAULT
	// -------------------------------------------------------------
	credSearchEntry := widget.NewEntry()
	credSearchEntry.SetPlaceHolder("🔍 Search by platform, username, url, or notes...")

	credCountLbl := widget.NewLabelWithStyle(fmt.Sprintf("%d Accounts", cs.TotalCount()), fyne.TextAlignTrailing, fyne.TextStyle{Bold: true})

	credListContainer := container.NewVBox()
	credScroll := container.NewScroll(credListContainer)

	// Keep track of which passwords are shown vs masked
	shownPasswords := make(map[string]bool)

	var renderCredentials func()

	openAddEditDialog := func(item *CredentialItem) {
		isEdit := item != nil

		platformEntry := widget.NewEntry()
		platformEntry.SetPlaceHolder("e.g. GitHub, AWS, Google Cloud, Postgres")

		usernameEntry := widget.NewEntry()
		usernameEntry.SetPlaceHolder("e.g. dev@company.com or admin")

		passwordEntry := widget.NewPasswordEntry()
		passwordEntry.SetPlaceHolder("Enter or generate secure password")

		showPassBtn := widget.NewButtonWithIcon("", theme.VisibilityIcon(), nil)
		passVisible := false
		showPassBtn.OnTapped = func() {
			passVisible = !passVisible
			txt := passwordEntry.Text
			if passVisible {
				passwordEntry.Password = false
				showPassBtn.SetIcon(theme.VisibilityOffIcon())
			} else {
				passwordEntry.Password = true
				showPassBtn.SetIcon(theme.VisibilityIcon())
			}
			passwordEntry.SetText(txt)
		}

		genPassBtn := widget.NewButtonWithIcon("🎲 Generate", theme.ViewRefreshIcon(), func() {
			generated := GeneratePassword(18, true)
			passwordEntry.Password = false
			passVisible = true
			showPassBtn.SetIcon(theme.VisibilityOffIcon())
			passwordEntry.SetText(generated)
		})

		passRow := container.NewBorder(nil, nil, nil, container.NewHBox(showPassBtn, genPassBtn), passwordEntry)

		urlEntry := widget.NewEntry()
		urlEntry.SetPlaceHolder("e.g. https://github.com/login")

		notesEntry := widget.NewMultiLineEntry()
		notesEntry.SetPlaceHolder("Optional notes, 2FA backup codes, port info, etc.")
		notesEntry.Wrapping = fyne.TextWrapWord

		if isEdit {
			platformEntry.SetText(item.Platform)
			usernameEntry.SetText(item.Username)
			passwordEntry.SetText(item.Password)
			urlEntry.SetText(item.URL)
			notesEntry.SetText(item.Notes)
		}

		form := widget.NewForm(
			widget.NewFormItem("Platform *", platformEntry),
			widget.NewFormItem("Username *", usernameEntry),
			widget.NewFormItem("Password *", passRow),
			widget.NewFormItem("URL / Host", urlEntry),
			widget.NewFormItem("Notes", notesEntry),
		)

		title := "➕ Add Account Credential"
		confirmText := "Save Credential"
		if isEdit {
			title = fmt.Sprintf("✏️ Edit '%s'", item.Platform)
			confirmText = "Update Credential"
		}

		modalContent := container.NewVScroll(form)
		modalContent.SetMinSize(fyne.NewSize(580, 360))

		d := dialog.NewCustomConfirm(title, confirmText, "Cancel", modalContent, func(confirm bool) {
			if !confirm {
				return
			}

			platform := strings.TrimSpace(platformEntry.Text)
			username := strings.TrimSpace(usernameEntry.Text)
			password := passwordEntry.Text
			url := strings.TrimSpace(urlEntry.Text)
			notes := strings.TrimSpace(notesEntry.Text)

			if platform == "" || username == "" {
				dialog.ShowInformation("Missing Fields", "Platform name and Username are required.", w)
				return
			}

			if isEdit {
				cs.Update(item.ID, platform, username, password, url, notes)
				a.SendNotification(&fyne.Notification{
					Title:   "Credential Updated",
					Content: fmt.Sprintf("Updated '%s' successfully.", platform),
				})
			} else {
				cs.Add(platform, username, password, url, notes)
				a.SendNotification(&fyne.Notification{
					Title:   "Credential Saved",
					Content: fmt.Sprintf("Saved '%s' (%s) to vault.", platform, username),
				})
			}

			renderCredentials()
		}, w)
		d.Resize(fyne.NewSize(620, 420))
		d.Show()
	}

	renderCredentials = func() {
		credListContainer.Objects = nil
		items := cs.List(credSearchEntry.Text)
		credCountLbl.SetText(fmt.Sprintf("%d Accounts", len(items)))

		if len(items) == 0 {
			emptyCard := widget.NewCard("", "", container.NewCenter(
				widget.NewLabelWithStyle("No credentials found.\nClick '+ Add Account' above to store your first platform credential.", fyne.TextAlignCenter, fyne.TextStyle{Italic: true}),
			))
			credListContainer.Add(emptyCard)
			credListContainer.Refresh()
			return
		}

		for _, item := range items {
			it := item // capture loop variable

			// Platform Title Badge
			platformTitle := canvas.NewText(it.Platform, color.RGBA{R: 56, G: 189, B: 248, A: 255}) // sky blue
			platformTitle.TextStyle = fyne.TextStyle{Bold: true}
			platformTitle.TextSize = 16

			updatedStr := it.UpdatedAt.Format("Jan 02, 2006")
			dateLbl := widget.NewLabelWithStyle(fmt.Sprintf("Saved: %s", updatedStr), fyne.TextAlignTrailing, fyne.TextStyle{Italic: true})

			cardTop := container.NewBorder(nil, nil, platformTitle, dateLbl)

			// Username Row
			userLbl := widget.NewLabelWithStyle(it.Username, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			copyUserBtn := widget.NewButtonWithIcon("Copy User", theme.ContentCopyIcon(), func() {
				w.Clipboard().SetContent(it.Username)
				a.SendNotification(&fyne.Notification{
					Title:   "Clipboard",
					Content: fmt.Sprintf("📋 Copied username '%s' to clipboard", it.Username),
				})
			})

			// Password Row
			isShown := shownPasswords[it.ID]
			var passDisplay *widget.Label
			if isShown {
				passDisplay = widget.NewLabelWithStyle(it.Password, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
			} else {
				masked := strings.Repeat("•", 12)
				passDisplay = widget.NewLabelWithStyle(masked, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
			}

			var togglePassBtn *widget.Button
			togglePassBtn = widget.NewButtonWithIcon("", theme.VisibilityIcon(), func() {
				shownPasswords[it.ID] = !shownPasswords[it.ID]
				renderCredentials()
			})
			if isShown {
				togglePassBtn.SetIcon(theme.VisibilityOffIcon())
			}

			copyPassBtn := widget.NewButtonWithIcon("Copy Pass", theme.ContentCopyIcon(), func() {
				w.Clipboard().SetContent(it.Password)
				a.SendNotification(&fyne.Notification{
					Title:   "Clipboard",
					Content: fmt.Sprintf("🔑 Copied password for '%s' to clipboard", it.Platform),
				})
			})

			// URL / Notes row
			infoBox := container.NewVBox()
			if it.URL != "" {
				urlLbl := widget.NewLabel(fmt.Sprintf("🌐 %s", it.URL))
				urlLbl.Truncation = fyne.TextTruncateEllipsis
				copyUrlBtn := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
					w.Clipboard().SetContent(it.URL)
				})
				infoBox.Add(container.NewHBox(urlLbl, copyUrlBtn))
			}
			if it.Notes != "" {
				notesPreview := it.Notes
				if len(notesPreview) > 80 {
					notesPreview = notesPreview[:77] + "..."
				}
				infoBox.Add(widget.NewLabel(fmt.Sprintf("📝 %s", notesPreview)))
			}

			// Action Buttons
			editBtn := widget.NewButtonWithIcon("Edit", theme.DocumentCreateIcon(), func() {
				openAddEditDialog(&it)
			})

			deleteBtn := widget.NewButtonWithIcon("Delete", theme.DeleteIcon(), func() {
				dialog.ShowConfirm("Delete Credential", fmt.Sprintf("Are you sure you want to delete '%s' (%s)?", it.Platform, it.Username), func(confirm bool) {
					if confirm {
						cs.Delete(it.ID)
						delete(shownPasswords, it.ID)
						renderCredentials()
					}
				}, w)
			})

			userPassGrid := container.NewGridWithColumns(2,
				container.NewHBox(widget.NewLabel("👤 User:"), userLbl, copyUserBtn),
				container.NewHBox(widget.NewLabel("🔑 Pass:"), passDisplay, togglePassBtn, copyPassBtn),
			)

			actionRow := container.NewHBox(layout.NewSpacer(), editBtn, deleteBtn)

			cardContent := container.NewVBox(
				cardTop,
				userPassGrid,
				infoBox,
				widget.NewSeparator(),
				actionRow,
			)

			card := widget.NewCard("", "", cardContent)
			credListContainer.Add(card)
		}

		credListContainer.Refresh()
	}

	credSearchEntry.OnChanged = func(string) {
		renderCredentials()
	}

	addAccountBtn := widget.NewButtonWithIcon("➕ Add Account", theme.ContentAddIcon(), func() {
		openAddEditDialog(nil)
	})
	addAccountBtn.Importance = widget.HighImportance

	credToolbar := container.NewBorder(nil, nil, nil, container.NewHBox(credCountLbl, addAccountBtn), credSearchEntry)
	credTabContainer := container.NewBorder(credToolbar, nil, nil, nil, credScroll)

	// -------------------------------------------------------------
	// SECTION 2: PERMANENT AUTO-CLIPBOARD SAVER
	// -------------------------------------------------------------
	clipSearchEntry := widget.NewEntry()
	clipSearchEntry.SetPlaceHolder("🔍 Search permanent clipboard history...")

	pinnedOnlyCheck := widget.NewCheck("⭐ Pinned Only", nil)

	clipCountLbl := widget.NewLabelWithStyle(fmt.Sprintf("%d Clips", cw.TotalCount()), fyne.TextAlignTrailing, fyne.TextStyle{Bold: true})

	var autoSaveBtn *widget.Button
	updateAutoSaveBtn := func() {
		if cw.IsAutoSaveEnabled() {
			autoSaveBtn.SetText("● Auto-Save: ACTIVE")
			autoSaveBtn.Importance = widget.HighImportance
		} else {
			autoSaveBtn.SetText("○ Auto-Save: PAUSED")
			autoSaveBtn.Importance = widget.MediumImportance
		}
	}

	autoSaveBtn = widget.NewButton("● Auto-Save: ACTIVE", func() {
		newState := cw.ToggleAutoSave()
		updateAutoSaveBtn()
		msg := "Clipboard auto-saver resumed"
		if !newState {
			msg = "Clipboard auto-saver paused"
		}
		a.SendNotification(&fyne.Notification{
			Title:   "Auto-Clipboard",
			Content: msg,
		})
	})
	updateAutoSaveBtn()

	clearClipsBtn := widget.NewButtonWithIcon("Clear Unpinned", theme.DeleteIcon(), func() {
		dialog.ShowConfirm("Clear Clipboard History", "Clear all unpinned clips? Pinned clips (⭐) will be kept permanently.", func(confirm bool) {
			if confirm {
				cleared := cw.ClearUnpinned()
				a.SendNotification(&fyne.Notification{
					Title:   "Clipboard History",
					Content: fmt.Sprintf("Cleared %d unpinned item(s).", cleared),
				})
			}
		}, w)
	})

	clipListContainer := container.NewVBox()
	clipScroll := container.NewScroll(clipListContainer)

	var renderClipboard func()

	openClipDetailModal := func(it ClipboardItem) {
		detailEntry := newReadOnlyTerminalEntry()
		detailEntry.SetText(it.Content)

		scroll := container.NewScroll(detailEntry)
		scroll.SetMinSize(fyne.NewSize(640, 360))

		copyBtn := widget.NewButtonWithIcon("Copy Content", theme.ContentCopyIcon(), func() {
			w.Clipboard().SetContent(it.Content)
			a.SendNotification(&fyne.Notification{
				Title:   "Clipboard",
				Content: "📋 Copied content back to Windows clipboard",
			})
		})
		copyBtn.Importance = widget.HighImportance

		content := container.NewBorder(
			widget.NewLabelWithStyle(fmt.Sprintf("Captured: %s | Length: %d characters", it.Timestamp.Format("2006-01-02 15:04:05"), it.Length), fyne.TextAlignLeading, fyne.TextStyle{Italic: true}),
			container.NewHBox(layout.NewSpacer(), copyBtn),
			nil, nil, scroll,
		)

		d := dialog.NewCustom("📋 Clipboard Snippet Detail", "Close", content, w)
		d.Resize(fyne.NewSize(700, 480))
		d.Show()
	}

	renderClipboard = func() {
		clipListContainer.Objects = nil
		items := cw.List(clipSearchEntry.Text, pinnedOnlyCheck.Checked)
		clipCountLbl.SetText(fmt.Sprintf("%d Clips", len(items)))

		if len(items) == 0 {
			emptyCard := widget.NewCard("", "", container.NewCenter(
				widget.NewLabelWithStyle("No clipboard snippets found.\nCopy text anywhere on Windows and it will be permanently saved here.", fyne.TextAlignCenter, fyne.TextStyle{Italic: true}),
			))
			clipListContainer.Add(emptyCard)
			clipListContainer.Refresh()
			return
		}

		for _, item := range items {
			it := item

			// Snippet preview (limit to 3 lines or 200 chars for list)
			lines := strings.Split(it.Content, "\n")
			previewText := it.Content
			if len(lines) > 3 {
				previewText = strings.Join(lines[:3], "\n") + "\n..."
			}
			if len(previewText) > 220 {
				previewText = previewText[:217] + "..."
			}

			snippetLbl := widget.NewLabelWithStyle(previewText, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
			snippetLbl.Wrapping = fyne.TextWrapWord

			// Meta Info
			timeStr := it.Timestamp.Format("Jan 02 15:04:05")
			metaStr := fmt.Sprintf("🕒 %s  |  📏 %d chars", timeStr, it.Length)
			if it.Pinned {
				metaStr = "⭐ PINNED  |  " + metaStr
			}
			metaLbl := widget.NewLabelWithStyle(metaStr, fyne.TextAlignLeading, fyne.TextStyle{Italic: true})

			// Buttons
			copyBtn := widget.NewButtonWithIcon("Copy", theme.ContentCopyIcon(), func() {
				w.Clipboard().SetContent(it.Content)
				a.SendNotification(&fyne.Notification{
					Title:   "Clipboard",
					Content: "📋 Restored snippet to Windows clipboard",
				})
			})

			pinText := "Pin"
			if it.Pinned {
				pinText = "Unpin"
			}
			pinBtn := widget.NewButtonWithIcon(pinText, theme.ConfirmIcon(), func() {
				cw.TogglePin(it.ID)
				renderClipboard()
			})

			viewBtn := widget.NewButtonWithIcon("View", theme.SearchIcon(), func() {
				openClipDetailModal(it)
			})

			delBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
				cw.Delete(it.ID)
				renderClipboard()
			})

			actionBox := container.NewHBox(copyBtn, pinBtn, viewBtn, delBtn)
			headerBox := container.NewBorder(nil, nil, metaLbl, actionBox)

			cardBox := container.NewVBox(
				headerBox,
				widget.NewSeparator(),
				snippetLbl,
			)

			card := widget.NewCard("", "", cardBox)
			clipListContainer.Add(card)
		}

		clipListContainer.Refresh()
	}

	clipSearchEntry.OnChanged = func(string) {
		renderClipboard()
	}
	pinnedOnlyCheck.OnChanged = func(bool) {
		renderClipboard()
	}

	// Connect watcher callback for live UI updates
	cw.SetOnNewClip(func(item ClipboardItem) {
		fyne.Do(func() {
			renderClipboard()
		})
	})

	clipRightBar := container.NewHBox(autoSaveBtn, pinnedOnlyCheck, clearClipsBtn, clipCountLbl)
	clipToolbar := container.NewBorder(nil, nil, nil, clipRightBar, clipSearchEntry)
	clipTabContainer := container.NewBorder(clipToolbar, nil, nil, nil, clipScroll)

	// -------------------------------------------------------------
	// COMBINE SUB-TABS INTO MASTER TAB
	// -------------------------------------------------------------
	tab1 := container.NewTabItemWithIcon("🔑 Credentials Vault", theme.ConfirmIcon(), credTabContainer)
	tab2 := container.NewTabItemWithIcon("📋 Auto Clipboard History", theme.ContentPasteIcon(), clipTabContainer)

	subTabs := container.NewAppTabs(tab1, tab2)
	subTabs.SetTabLocation(container.TabLocationTop)

	// Initial renders
	renderCredentials()
	renderClipboard()

	updateTelemetry := func() {
		renderCredentials()
		renderClipboard()
	}

	return subTabs, updateTelemetry
}
