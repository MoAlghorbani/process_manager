package main

import (
	"context"
	"fmt"
	"image/color"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var (
	uiVideoFormats = []string{
		"MP4 (Recommended)",
		"MKV (Universal)",
		"WEBM (WebM)",
	}
	uiVideoQualities = []string{
		"🌟 Best Available",
		"📺 4K - 2160p (Ultra HD)",
		"📺 2K - 1440p (Quad HD)",
		"📺 1080p (Full HD)",
		"📺 720p (HD)",
		"📺 480p (SD)",
		"📺 360p (Data Saver)",
		"📺 240p (Low)",
	}
	uiAudioFormats = []string{
		"MP3 (Universal)",
		"M4A (AAC)",
		"FLAC (Lossless)",
		"WAV (Raw)",
		"OPUS (Modern)",
		"OGG (Vorbis)",
	}
	uiAudioQualities = []string{
		"🌟 Best (VBR 0 / Maximum)",
		"🔊 320 kbps (High Quality)",
		"🔊 256 kbps",
		"🔊 192 kbps (Standard)",
		"🔊 128 kbps (Compact)",
		"🔊 96 kbps (Voice / Low)",
	}
)

func parseContainerFromSelect(selected string) string {
	s := strings.ToLower(selected)
	switch {
	case strings.HasPrefix(s, "mp4"):
		return "mp4"
	case strings.HasPrefix(s, "mkv"):
		return "mkv"
	case strings.HasPrefix(s, "webm"):
		return "webm"
	case strings.HasPrefix(s, "mp3"):
		return "mp3"
	case strings.HasPrefix(s, "m4a"):
		return "m4a"
	case strings.HasPrefix(s, "flac"):
		return "flac"
	case strings.HasPrefix(s, "wav"):
		return "wav"
	case strings.HasPrefix(s, "opus"):
		return "opus"
	case strings.HasPrefix(s, "ogg"):
		return "ogg"
	default:
		return "mp4"
	}
}

func parseQualityFromSelect(selected string) string {
	s := strings.ToLower(selected)
	switch {
	case strings.Contains(s, "best"):
		return "best"
	case strings.Contains(s, "2160p") || strings.Contains(s, "4k"):
		return "2160p"
	case strings.Contains(s, "1440p") || strings.Contains(s, "2k"):
		return "1440p"
	case strings.Contains(s, "1080p"):
		return "1080p"
	case strings.Contains(s, "720p"):
		return "720p"
	case strings.Contains(s, "480p"):
		return "480p"
	case strings.Contains(s, "360p"):
		return "360p"
	case strings.Contains(s, "240p"):
		return "240p"
	case strings.Contains(s, "320"):
		return "320k"
	case strings.Contains(s, "256"):
		return "256k"
	case strings.Contains(s, "192"):
		return "192k"
	case strings.Contains(s, "128"):
		return "128k"
	case strings.Contains(s, "96"):
		return "96k"
	default:
		return "best"
	}
}

func matchSelectOption(options []string, keyword string) string {
	lowerKW := strings.ToLower(keyword)
	for _, opt := range options {
		if strings.Contains(strings.ToLower(opt), lowerKW) {
			return opt
		}
	}
	if len(options) > 0 {
		return options[0]
	}
	return ""
}

// buildYtDlpTab constructs the native Fyne GUI tab for the YouTube & Media Downloader.
func buildYtDlpTab(w fyne.Window, a fyne.App) (fyne.CanvasObject, func()) {
	engine := GetYtDlpEngine()

	// --- Top Header ---
	titleLbl := widget.NewLabelWithStyle("📥 YouTube & Media Downloader", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	ffmpegPath := FindFfmpeg()
	var ffmpegBadge *canvas.Text
	if ffmpegPath != "" {
		ffmpegBadge = canvas.NewText("● FFmpeg Ready (1080p/4K Merging & Audio Conversion Enabled)", color.RGBA{R: 16, G: 185, B: 129, A: 255})
	} else {
		ffmpegBadge = canvas.NewText("● FFmpeg not found (Standard single-stream only)", color.RGBA{R: 245, G: 158, B: 11, A: 255})
	}
	ffmpegBadge.TextSize = 11
	ffmpegBadge.TextStyle = fyne.TextStyle{Italic: true}

	headerBox := container.NewHBox(
		titleLbl,
		layout.NewSpacer(),
		ffmpegBadge,
	)

	// --- URL Input ---
	cfg := engine.GetConfig()
	var fetchInfo func(string)

	urlEntry := widget.NewEntry()
	urlEntry.SetPlaceHolder("Paste video or playlist URL (e.g. https://www.youtube.com/playlist?list=...)")

	playlistCheck := widget.NewCheck("📦 Download Full Playlist (if URL contains a playlist)", nil)
	playlistCheck.SetChecked(cfg.DownloadPlaylist)
	playlistCheck.OnChanged = func(checked bool) {
		engine.SetDownloadPlaylist(checked)
		if strings.TrimSpace(urlEntry.Text) != "" && fetchInfo != nil {
			fetchInfo(urlEntry.Text)
		}
	}

	urlEntry.OnChanged = func(s string) {
		if IsPurePlaylistURL(s) && !playlistCheck.Checked {
			playlistCheck.SetChecked(true)
		}
	}

	// --- Format & Quality Preferences ---
	currentMediaType := cfg.LastMediaType
	if currentMediaType == "" {
		currentMediaType = "video"
	}
	currentVideoFormat := cfg.LastVideoFormat
	if currentVideoFormat == "" {
		currentVideoFormat = "mp4"
	}
	currentVideoQuality := cfg.LastVideoQuality
	if currentVideoQuality == "" {
		currentVideoQuality = "1080p"
	}
	currentAudioFormat := cfg.LastAudioFormat
	if currentAudioFormat == "" {
		currentAudioFormat = "mp3"
	}
	currentAudioQuality := cfg.LastAudioQuality
	if currentAudioQuality == "" {
		currentAudioQuality = "320k"
	}

	var formatSelect *widget.Select
	var qualitySelect *widget.Select

	formatSelect = widget.NewSelect([]string{}, func(s string) {
		if currentMediaType == "audio" {
			currentAudioFormat = parseContainerFromSelect(s)
		} else {
			currentVideoFormat = parseContainerFromSelect(s)
		}
		engine.SaveOptionsConfig(currentMediaType, currentVideoFormat, currentVideoQuality, currentAudioFormat, currentAudioQuality)
	})

	qualitySelect = widget.NewSelect([]string{}, func(s string) {
		if currentMediaType == "audio" {
			currentAudioQuality = parseQualityFromSelect(s)
		} else {
			currentVideoQuality = parseQualityFromSelect(s)
		}
		engine.SaveOptionsConfig(currentMediaType, currentVideoFormat, currentVideoQuality, currentAudioFormat, currentAudioQuality)
	})

	updateOptionsUI := func(mType string) {
		currentMediaType = mType
		if currentMediaType == "audio" {
			formatSelect.Options = uiAudioFormats
			qualitySelect.Options = uiAudioQualities
			formatSelect.SetSelected(matchSelectOption(uiAudioFormats, currentAudioFormat))
			qualitySelect.SetSelected(matchSelectOption(uiAudioQualities, currentAudioQuality))
		} else {
			formatSelect.Options = uiVideoFormats
			qualitySelect.Options = uiVideoQualities
			formatSelect.SetSelected(matchSelectOption(uiVideoFormats, currentVideoFormat))
			qualitySelect.SetSelected(matchSelectOption(uiVideoQualities, currentVideoQuality))
		}
		formatSelect.Refresh()
		qualitySelect.Refresh()
		engine.SaveOptionsConfig(currentMediaType, currentVideoFormat, currentVideoQuality, currentAudioFormat, currentAudioQuality)
	}

	mediaTypeRadio := widget.NewRadioGroup([]string{"🎬 Video (MP4 / MKV / WEBM)", "🎵 Audio Only (MP3 / M4A / FLAC / WAV)"}, func(sel string) {
		if strings.Contains(sel, "Audio") {
			updateOptionsUI("audio")
		} else {
			updateOptionsUI("video")
		}
	})
	mediaTypeRadio.Horizontal = true

	if currentMediaType == "audio" {
		mediaTypeRadio.SetSelected("🎵 Audio Only (MP3 / M4A / FLAC / WAV)")
	} else {
		mediaTypeRadio.SetSelected("🎬 Video (MP4 / MKV / WEBM)")
	}

	// Output directory
	dirEntry := widget.NewEntry()
	dirEntry.SetText(cfg.OutputDir)

	browseBtn := widget.NewButtonWithIcon("Browse...", theme.FolderOpenIcon(), func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			rawPath := uri.Path()
			clean := filepath.FromSlash(rawPath)
			if strings.HasPrefix(clean, "\\") && len(clean) > 2 && clean[2] == ':' {
				clean = clean[1:] // strip extra leading slash on Windows
			}
			dirEntry.SetText(clean)
			engine.SetOutputDir(clean)
		}, w)
	})

	openFolderBtn := widget.NewButtonWithIcon("Open Folder", theme.FolderIcon(), func() {
		targetDir := strings.TrimSpace(dirEntry.Text)
		if targetDir != "" {
			go func() {
				_ = exec.Command("explorer.exe", targetDir).Start()
			}()
		}
	})

	dirRow := container.NewBorder(nil, nil, nil, container.NewHBox(browseBtn, openFolderBtn), dirEntry)

	// --- Metadata Preview Card ---
	previewTitleLbl := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	previewTitleLbl.Wrapping = fyne.TextWrapWord
	previewMetaLbl := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
	previewMetaLbl.Wrapping = fyne.TextWrapWord

	previewCard := widget.NewCard("📺 Video Preview", "", container.NewVBox(previewTitleLbl, previewMetaLbl))
	previewCard.Hide()

	var (
		fetchCancel      context.CancelFunc
		fetchMu          sync.Mutex
		lastFetchedMeta  *VideoMetadata
	)

	fetchInfo = func(url string) {
		clean := strings.TrimSpace(url)
		if clean == "" {
			return
		}

		fetchMu.Lock()
		if fetchCancel != nil {
			fetchCancel()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		fetchCancel = cancel
		fetchMu.Unlock()

		previewTitleLbl.SetText("🔍 Fetching details...")
		previewMetaLbl.SetText("Contacting yt-dlp...")
		previewCard.Show()

		allowPlaylist := playlistCheck.Checked

		go func() {
			meta, err := engine.FetchMetadata(ctx, clean, allowPlaylist)
			fyne.Do(func() {
				if err != nil {
					if ctx.Err() == context.Canceled {
						return
					}
					previewTitleLbl.SetText("⚠️ Could not load preview")
					previewMetaLbl.SetText(err.Error())
					return
				}

				lastFetchedMeta = meta
				if meta.IsPlaylist {
					previewCard.SetTitle("📦 Playlist Preview")
					previewTitleLbl.SetText(fmt.Sprintf("📁 %s", meta.Title))
					metaText := fmt.Sprintf("👤 %s  •  📦 Playlist (%d videos)", meta.Uploader, meta.PlaylistCount)
					previewMetaLbl.SetText(metaText)
				} else {
					previewCard.SetTitle("📺 Video Preview")
					previewTitleLbl.SetText(meta.Title)
					viewText := formatViewCount(meta.ViewCount)
					metaText := fmt.Sprintf("👤 %s  •  ⏱️ Duration: %s", meta.Uploader, meta.DurationString)
					if meta.MaxResolution != "" {
						metaText += fmt.Sprintf("  •  📺 Max: %s", meta.MaxResolution)
					}
					if len(meta.AvailableResolutions) > 0 {
						avail := strings.Join(meta.AvailableResolutions, ", ")
						if len(avail) > 40 {
							avail = avail[:40] + "..."
						}
						metaText += fmt.Sprintf(" (%s)", avail)
					}
					if viewText != "" {
						metaText += fmt.Sprintf("  •  👁️ %s", viewText)
					}
					previewMetaLbl.SetText(metaText)
				}
				previewCard.Show()
			})
		}()
	}

	pasteBtn := widget.NewButtonWithIcon("Paste", theme.ContentPasteIcon(), func() {
		content := w.Clipboard().Content()
		clean := strings.TrimSpace(content)
		if clean != "" {
			urlEntry.SetText(clean)
			if strings.HasPrefix(clean, "http://") || strings.HasPrefix(clean, "https://") {
				fetchInfo(clean)
			}
		}
	})

	infoBtn := widget.NewButtonWithIcon("Preview", theme.SearchIcon(), func() {
		fetchInfo(urlEntry.Text)
	})

	urlRow := container.NewBorder(nil, nil, nil, container.NewHBox(pasteBtn, infoBtn), urlEntry)

	// --- Active Download Progress Card ---
	activeTitleLbl := widget.NewLabelWithStyle("No active downloads", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	activeTitleLbl.Wrapping = fyne.TextWrapWord
	activeMetricsLbl := widget.NewLabelWithStyle("Ready", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
	activeMetricsLbl.Wrapping = fyne.TextWrapWord
	activeProgressBar := widget.NewProgressBar()
	activeProgressBar.SetValue(0.0)

	var (
		cancelDownloadBtn *widget.Button
		openFileBtn       *widget.Button
		openInFolderBtn   *widget.Button
	)

	cancelDownloadBtn = widget.NewButtonWithIcon("Cancel", theme.CancelIcon(), func() {
		engine.CancelDownload("")
	})
	cancelDownloadBtn.Importance = widget.DangerImportance
	cancelDownloadBtn.Hide()

	openFileBtn = widget.NewButtonWithIcon("▶ Open File", theme.MediaPlayIcon(), nil)
	openFileBtn.Importance = widget.HighImportance
	openFileBtn.Hide()

	openInFolderBtn = widget.NewButtonWithIcon("📂 Show in Folder", theme.FolderIcon(), nil)
	openInFolderBtn.Hide()

	activeActionBox := container.NewHBox(cancelDownloadBtn, openFileBtn, openInFolderBtn)

	activeDownloadCard := widget.NewCard("⚡ Active Download", "", container.NewVBox(
		activeTitleLbl,
		activeProgressBar,
		container.NewBorder(nil, nil, activeMetricsLbl, activeActionBox),
	))

	// --- Download History ---
	historyListContainer := container.NewVBox()
	historyScroll := container.NewVScroll(historyListContainer)

	var renderHistory func()

	renderHistory = func() {
		historyListContainer.Objects = nil
		items := engine.GetHistory()

		if len(items) == 0 {
			emptyLbl := widget.NewLabelWithStyle("No downloaded items yet.", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})
			historyListContainer.Add(container.NewPadded(emptyLbl))
			historyListContainer.Refresh()
			return
		}

		for _, it := range items {
			itemCopy := it // capture loop var

			// Icon & Format Badge
			iconTag := "🎬"
			if itemCopy.IsPlaylist {
				iconTag = "📦"
			} else if itemCopy.MediaType == "audio" || strings.HasPrefix(string(itemCopy.Format), "audio") {
				iconTag = "🎵"
			}

			titleText := itemCopy.Title
			if titleText == "" {
				titleText = itemCopy.URL
			}
			titleLbl := widget.NewLabelWithStyle(fmt.Sprintf("%s %s", iconTag, titleText), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			titleLbl.Wrapping = fyne.TextWrapWord

			// Secondary telemetry row
			statusColor := color.RGBA{R: 16, G: 185, B: 129, A: 255} // emerald
			statusText := "● COMPLETED"
			if itemCopy.Status == "error" {
				statusColor = color.RGBA{R: 239, G: 68, B: 68, A: 255} // red
				statusText = "● FAILED"
			} else if itemCopy.Status == "cancelled" {
				statusColor = color.RGBA{R: 245, G: 158, B: 11, A: 255} // amber
				statusText = "● CANCELLED"
			}

			statusCanvas := canvas.NewText(statusText, statusColor)
			statusCanvas.TextSize = 11
			statusCanvas.TextStyle = fyne.TextStyle{Bold: true}

			labelStr := itemCopy.FormatLabel
			if labelStr == "" {
				labelStr = FormatDisplayLabel(itemCopy.MediaType, itemCopy.Container, itemCopy.Quality)
			}
			metaStr := labelStr
			if itemCopy.IsPlaylist {
				metaStr += "  •  Playlist"
			}
			if itemCopy.FileSize != "" {
				metaStr += fmt.Sprintf("  •  %s", itemCopy.FileSize)
			}
			if itemCopy.CompletedAt != nil {
				metaStr += fmt.Sprintf("  •  %s", itemCopy.CompletedAt.Format("01/02 15:04"))
			}
			metaLbl := widget.NewLabel(metaStr)
			metaLbl.TextStyle = fyne.TextStyle{Italic: true}

			// Actions
			actionRow := container.NewHBox()

			if itemCopy.FilePath != "" && itemCopy.Status == "completed" {
				if itemCopy.IsPlaylist {
					folderBtn := widget.NewButtonWithIcon("Open Folder", theme.FolderOpenIcon(), func() {
						fPath := itemCopy.FilePath
						go func() {
							_ = exec.Command("explorer.exe", fPath).Start()
						}()
					})
					actionRow.Add(folderBtn)
				} else {
					playBtn := widget.NewButtonWithIcon("Open", theme.MediaPlayIcon(), func() {
						fPath := itemCopy.FilePath
						go func() {
							_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", fPath).Start()
						}()
					})

					showBtn := widget.NewButtonWithIcon("Folder", theme.FolderIcon(), func() {
						fPath := itemCopy.FilePath
						go func() {
							_ = exec.Command("explorer.exe", "/select,"+fPath).Start()
						}()
					})

					actionRow.Add(playBtn)
					actionRow.Add(showBtn)
				}
			}

			delBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
				dialog.ShowConfirm("Delete Download Record", "Remove this item from history? (The downloaded file on disk will NOT be deleted)", func(ok bool) {
					if ok {
						engine.DeleteHistoryItem(itemCopy.ID, false)
						renderHistory()
					}
				}, w)
			})
			actionRow.Add(delBtn)

			rowTop := container.NewBorder(nil, nil, statusCanvas, actionRow, titleLbl)
			rowCard := container.NewVBox(rowTop, metaLbl, widget.NewSeparator())
			historyListContainer.Add(rowCard)
		}
		historyListContainer.Refresh()
	}

	clearHistoryBtn := widget.NewButtonWithIcon("Clear Completed", theme.DeleteIcon(), func() {
		dialog.ShowConfirm("Clear History", "Remove all completed download records from the list?", func(ok bool) {
			if ok {
				engine.ClearCompletedHistory()
				renderHistory()
			}
		}, w)
	})

	historyHeader := container.NewBorder(
		nil, nil,
		widget.NewLabelWithStyle("📋 Recent Downloads", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		clearHistoryBtn,
	)

	// --- Start Download Button & Logic ---
	var downloadBtn *widget.Button
	downloadBtn = widget.NewButtonWithIcon("⬇️  START DOWNLOAD", theme.DownloadIcon(), func() {
		rawUrl := strings.TrimSpace(urlEntry.Text)
		if rawUrl == "" {
			dialog.ShowInformation("URL Required", "Please enter or paste a valid video URL.", w)
			return
		}

		outputDir := strings.TrimSpace(dirEntry.Text)
		if outputDir == "" {
			outputDir = defaultDownloadsDir()
			dirEntry.SetText(outputDir)
		}
		engine.SetOutputDir(outputDir)

		downloadBtn.Disable()
		cancelDownloadBtn.Show()
		openFileBtn.Hide()
		openInFolderBtn.Hide()

		selectedContainer := currentVideoFormat
		selectedQuality := currentVideoQuality
		if currentMediaType == "audio" {
			selectedContainer = currentAudioFormat
			selectedQuality = currentAudioQuality
		}

		title := rawUrl
		isPlaylist := IsPurePlaylistURL(rawUrl) || (playlistCheck.Checked && IsPlaylistURL(rawUrl))
		if lastFetchedMeta != nil && lastFetchedMeta.WebpageURL == rawUrl && lastFetchedMeta.Title != "" {
			title = lastFetchedMeta.Title
			if lastFetchedMeta.IsPlaylist {
				isPlaylist = true
			}
		}

		if isPlaylist {
			activeTitleLbl.SetText(fmt.Sprintf("Downloading Playlist: %s", title))
		} else {
			activeTitleLbl.SetText(fmt.Sprintf("Downloading: %s", title))
		}
		activeProgressBar.SetValue(0.0)
		activeMetricsLbl.SetText("Starting download...")

		_, err := engine.StartDownloadWithOptions(rawUrl, title, currentMediaType, selectedContainer, selectedQuality, outputDir, isPlaylist, func(item *DownloadItem) {
			fyne.Do(func() {
				if item.Status == "downloading" {
					if item.IsPlaylist {
						if item.TotalItems > 0 && item.CurrentItem > 0 {
							overallVal := (float64(item.CurrentItem-1) + (item.Progress / 100.0)) / float64(item.TotalItems)
							activeProgressBar.SetValue(overallVal)
							metricStr := fmt.Sprintf("Item %d of %d (%.1f%%) • Overall: %.1f%%", item.CurrentItem, item.TotalItems, item.Progress, overallVal*100.0)
							if item.Speed != "" {
								metricStr += fmt.Sprintf("  •  Speed: %s", item.Speed)
							}
							if item.ETA != "" {
								metricStr += fmt.Sprintf("  •  ETA: %s", item.ETA)
							}
							activeMetricsLbl.SetText(metricStr)
							activeTitleLbl.SetText(fmt.Sprintf("📦 [%d/%d] %s", item.CurrentItem, item.TotalItems, item.Title))
						} else {
							activeProgressBar.SetValue(item.Progress / 100.0)
							metricStr := fmt.Sprintf("%.1f%%", item.Progress)
							if item.Speed != "" {
								metricStr += fmt.Sprintf("  •  Speed: %s", item.Speed)
							}
							activeMetricsLbl.SetText(metricStr)
							activeTitleLbl.SetText(fmt.Sprintf("📦 %s", item.Title))
						}
					} else {
						activeTitleLbl.SetText(item.Title)
						activeProgressBar.SetValue(item.Progress / 100.0)

						metricStr := fmt.Sprintf("%.1f%%", item.Progress)
						if item.Speed != "" {
							metricStr += fmt.Sprintf("  •  Speed: %s", item.Speed)
						}
						if item.ETA != "" {
							metricStr += fmt.Sprintf("  •  ETA: %s", item.ETA)
						}
						if item.TotalSize != "" {
							metricStr += fmt.Sprintf("  •  Size: %s", item.TotalSize)
						}
						activeMetricsLbl.SetText(metricStr)
					}

				} else if item.Status == "completed" {
					downloadBtn.Enable()
					cancelDownloadBtn.Hide()
					activeProgressBar.SetValue(1.0)
					if item.IsPlaylist {
						activeTitleLbl.SetText(fmt.Sprintf("✅ Playlist Complete: %s", item.Title))
						activeMetricsLbl.SetText(fmt.Sprintf("All items complete! Saved to: %s (%s)", item.FilePath, item.FileSize))
					} else {
						activeTitleLbl.SetText(fmt.Sprintf("✅ %s", item.Title))
						activeMetricsLbl.SetText(fmt.Sprintf("Completed! Saved to: %s", item.FilePath))
					}

					if item.FilePath != "" {
						fPath := item.FilePath
						if item.IsPlaylist {
							openFileBtn.SetText("📂 Open Playlist Folder")
							openFileBtn.OnTapped = func() {
								go func() {
									_ = exec.Command("explorer.exe", fPath).Start()
								}()
							}
							openFileBtn.Show()
							openInFolderBtn.Hide()
						} else {
							openFileBtn.SetText("▶ Open File")
							openFileBtn.OnTapped = func() {
								go func() {
									_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", fPath).Start()
								}()
							}
							openInFolderBtn.OnTapped = func() {
								go func() {
									_ = exec.Command("explorer.exe", "/select,"+fPath).Start()
								}()
							}
							openFileBtn.Show()
							openInFolderBtn.Show()
						}
					}

					a.SendNotification(&fyne.Notification{
						Title:   "Download Complete",
						Content: fmt.Sprintf("Downloaded %s (%s)", item.Title, item.FileSize),
					})

					renderHistory()

				} else if item.Status == "error" || item.Status == "cancelled" {
					downloadBtn.Enable()
					cancelDownloadBtn.Hide()
					if item.Status == "cancelled" {
						activeTitleLbl.SetText("⏹ Download Cancelled")
						activeMetricsLbl.SetText("Cancelled by user")
					} else {
						activeTitleLbl.SetText("❌ Download Failed")
						activeMetricsLbl.SetText(item.Error)
					}
					renderHistory()
				}
			})
		})

		if err != nil {
			downloadBtn.Enable()
			cancelDownloadBtn.Hide()
			dialog.ShowError(err, w)
		}
	})
	downloadBtn.Importance = widget.HighImportance

	// --- Form Card Layout ---
	optionsGrid := container.NewGridWithColumns(2,
		container.NewVBox(
			widget.NewLabelWithStyle("Format / Container:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			formatSelect,
		),
		container.NewVBox(
			widget.NewLabelWithStyle("Quality / Resolution:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			qualitySelect,
		),
	)

	formSection := widget.NewCard("⚙️ Download Settings", "", container.NewVBox(
		widget.NewLabelWithStyle("Media URL:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		urlRow,
		playlistCheck,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Media Type:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		mediaTypeRadio,
		widget.NewSeparator(),
		optionsGrid,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Destination Folder:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		dirRow,
		widget.NewSeparator(),
		downloadBtn,
	))

	topSection := container.NewVBox(
		headerBox,
		widget.NewSeparator(),
		formSection,
		previewCard,
		activeDownloadCard,
		widget.NewSeparator(),
		historyHeader,
	)

	mainLayout := container.NewBorder(topSection, nil, nil, nil, historyScroll)

	// Initial history render
	renderHistory()

	return mainLayout, func() {
		renderHistory()
	}
}
