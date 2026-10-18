package main

import (
	_ "embed"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

//go:embed yt-dlp.exe
var embeddedYtDlp []byte

// Format constants
type DownloadFormat string

const (
	FormatVideoBest  DownloadFormat = "video_best"
	FormatVideo1080p DownloadFormat = "video_1080p"
	FormatVideo720p  DownloadFormat = "video_720p"
	FormatVideo480p  DownloadFormat = "video_480p"
	FormatAudioMP3   DownloadFormat = "audio_mp3"
	FormatAudioM4A   DownloadFormat = "audio_m4a"
)

func (f DownloadFormat) Label() string {
	switch f {
	case FormatVideoBest:
		return "🎬 Best Quality (MP4)"
	case FormatVideo1080p:
		return "🎬 1080p Full HD (MP4)"
	case FormatVideo720p:
		return "🎬 720p HD (MP4)"
	case FormatVideo480p:
		return "🎬 480p SD (MP4)"
	case FormatAudioMP3:
		return "🎵 Audio Only (MP3)"
	case FormatAudioM4A:
		return "🎵 Audio Only (M4A)"
	default:
		mediaType, container, quality := ParseFormatComponents(f)
		return FormatDisplayLabel(mediaType, container, quality)
	}
}

func AllDownloadFormats() []DownloadFormat {
	return []DownloadFormat{
		FormatVideoBest,
		FormatVideo1080p,
		FormatVideo720p,
		FormatVideo480p,
		FormatAudioMP3,
		FormatAudioM4A,
	}
}

// VideoMetadata stores fetched video details before downloading.
type VideoMetadata struct {
	ID                   string   `json:"id"`
	Title                string   `json:"title"`
	Uploader             string   `json:"uploader"`
	Duration             int      `json:"duration"`
	DurationString       string   `json:"duration_string"`
	Thumbnail            string   `json:"thumbnail"`
	ViewCount            int64    `json:"view_count"`
	WebpageURL           string   `json:"webpage_url"`
	MaxResolution        string   `json:"max_resolution,omitempty"`
	AvailableResolutions []string `json:"available_resolutions,omitempty"`
	IsPlaylist           bool     `json:"is_playlist,omitempty"`
	PlaylistCount        int      `json:"playlist_count,omitempty"`
}

// DownloadItem represents a single video/audio download task.
type DownloadItem struct {
	ID          string         `json:"id"`
	URL         string         `json:"url"`
	Title       string         `json:"title"`
	Uploader    string         `json:"uploader,omitempty"`
	Duration    string         `json:"duration,omitempty"`
	Thumbnail   string         `json:"thumbnail,omitempty"`
	MediaType   string         `json:"media_type,omitempty"` // "video" or "audio"
	Container   string         `json:"container,omitempty"`  // "mp4", "mkv", "webm", "mp3", "m4a", etc.
	Quality     string         `json:"quality,omitempty"`    // "best", "1080p", "720p", "320k", etc.
	Format      DownloadFormat `json:"format"`
	FormatLabel string         `json:"format_label"`
	OutputDir   string         `json:"output_dir"`
	FilePath    string         `json:"file_path,omitempty"`
	FileSize    string         `json:"file_size,omitempty"`
	Status      string         `json:"status"` // "queued", "fetching", "downloading", "completed", "error", "cancelled"
	Progress    float64        `json:"progress"` // 0.0 - 100.0
	Speed       string         `json:"speed,omitempty"`
	ETA         string         `json:"eta,omitempty"`
	TotalSize   string         `json:"total_size,omitempty"`
	LogSnippet  string         `json:"log_snippet,omitempty"`
	Error       string         `json:"error,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`

	IsPlaylist    bool   `json:"is_playlist,omitempty"`
	PlaylistTitle string `json:"playlist_title,omitempty"`
	CurrentItem   int    `json:"current_item,omitempty"`
	TotalItems    int    `json:"total_items,omitempty"`

	cmd        *exec.Cmd          `json:"-"`
	cancelFunc context.CancelFunc `json:"-"`
}

// YtDlpConfig stores downloader preferences.
type YtDlpConfig struct {
	OutputDir        string         `json:"output_dir"`
	LastFormat       DownloadFormat `json:"last_format,omitempty"`
	LastMediaType    string         `json:"last_media_type,omitempty"`    // "video" or "audio"
	LastVideoFormat  string         `json:"last_video_format,omitempty"`  // "mp4", "mkv", "webm"
	LastVideoQuality string         `json:"last_video_quality,omitempty"` // "best", "1080p", "720p", etc.
	LastAudioFormat  string         `json:"last_audio_format,omitempty"`  // "mp3", "m4a", "flac", etc.
	LastAudioQuality string         `json:"last_audio_quality,omitempty"` // "320k", "256k", "192k", etc.
	DownloadPlaylist bool           `json:"download_playlist"`            // whether to download full playlist if URL contains one
}

// IsPlaylistURL checks if a URL points to or contains a playlist parameter.
func IsPlaylistURL(rawUrl string) bool {
	u := strings.ToLower(strings.TrimSpace(rawUrl))
	return strings.Contains(u, "list=") || strings.Contains(u, "/playlist")
}

// IsPurePlaylistURL checks if a URL is strictly a playlist without a specific video ID.
func IsPurePlaylistURL(rawUrl string) bool {
	u := strings.ToLower(strings.TrimSpace(rawUrl))
	return strings.Contains(u, "/playlist") || (strings.Contains(u, "list=") && !strings.Contains(u, "v="))
}

// ParseFormatComponents parses a DownloadFormat string into mediaType, container, and quality.
func ParseFormatComponents(f DownloadFormat) (mediaType, container, quality string) {
	str := string(f)
	switch f {
	case FormatVideoBest:
		return "video", "mp4", "best"
	case FormatVideo1080p:
		return "video", "mp4", "1080p"
	case FormatVideo720p:
		return "video", "mp4", "720p"
	case FormatVideo480p:
		return "video", "mp4", "480p"
	case FormatAudioMP3:
		return "audio", "mp3", "320k"
	case FormatAudioM4A:
		return "audio", "m4a", "best"
	}

	parts := strings.Split(str, "_")
	if len(parts) >= 3 {
		return parts[0], parts[1], parts[2]
	}
	if strings.HasPrefix(str, "audio") {
		return "audio", "mp3", "320k"
	}
	return "video", "mp4", "best"
}

// FormatDisplayLabel creates a user-friendly label for display.
func FormatDisplayLabel(mediaType, container, quality string) string {
	mediaType = strings.ToLower(mediaType)
	container = strings.ToUpper(container)
	if mediaType == "audio" {
		qStr := quality
		if qStr == "best" || qStr == "0" {
			qStr = "Best / VBR 0"
		} else if !strings.HasSuffix(qStr, "k") && !strings.HasSuffix(qStr, "kbps") {
			qStr = qStr + " kbps"
		}
		return fmt.Sprintf("🎵 %s (%s)", container, qStr)
	}

	qStr := quality
	if qStr == "best" {
		qStr = "Best"
	} else if strings.HasSuffix(qStr, "p") {
		qStr = strings.ToUpper(qStr)
	}
	return fmt.Sprintf("🎬 %s (%s)", container, qStr)
}

// BuildYtDlpFormatArgs builds the yt-dlp CLI arguments based on mediaType, container, and quality.
func BuildYtDlpFormatArgs(mediaType, container, quality, ffmpegPath string) []string {
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if mediaType == "" {
		mediaType = "video"
	}
	container = strings.ToLower(strings.TrimSpace(container))
	if container == "" {
		if mediaType == "audio" {
			container = "mp3"
		} else {
			container = "mp4"
		}
	}
	quality = strings.ToLower(strings.TrimSpace(quality))
	if quality == "" {
		quality = "best"
	}

	var args []string

	if mediaType == "audio" {
		args = append(args, "-x", "--audio-format", container)
		switch quality {
		case "best", "0":
			args = append(args, "--audio-quality", "0")
		case "320k", "320":
			args = append(args, "--audio-quality", "320k")
		case "256k", "256":
			args = append(args, "--audio-quality", "256k")
		case "192k", "192":
			args = append(args, "--audio-quality", "192k")
		case "128k", "128":
			args = append(args, "--audio-quality", "128k")
		case "96k", "96":
			args = append(args, "--audio-quality", "96k")
		default:
			args = append(args, "--audio-quality", "0")
		}
		return args
	}

	// Video mode
	height := 0
	cleanQ := strings.TrimSuffix(quality, "p")
	if h, err := strconv.Atoi(cleanQ); err == nil && h > 0 {
		height = h
	}

	if ffmpegPath != "" {
		if height > 0 {
			switch container {
			case "mp4":
				args = append(args, "-f", fmt.Sprintf("bestvideo[height<=%d][ext=mp4]+bestaudio[ext=m4a]/bestvideo[height<=%d]+bestaudio/best[height<=%d]/best", height, height, height), "--merge-output-format", "mp4")
			case "mkv":
				args = append(args, "-f", fmt.Sprintf("bestvideo[height<=%d]+bestaudio/best[height<=%d]/best", height, height), "--merge-output-format", "mkv")
			case "webm":
				args = append(args, "-f", fmt.Sprintf("bestvideo[height<=%d][ext=webm]+bestaudio[ext=webm]/bestvideo[height<=%d]+bestaudio/best[height<=%d]/best", height, height, height), "--merge-output-format", "webm")
			default:
				args = append(args, "-f", fmt.Sprintf("bestvideo[height<=%d]+bestaudio/best[height<=%d]/best", height, height), "--merge-output-format", container)
			}
		} else {
			// Best Available
			switch container {
			case "mp4":
				args = append(args, "-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/bestvideo+bestaudio/best", "--merge-output-format", "mp4")
			case "mkv":
				args = append(args, "-f", "bestvideo+bestaudio/best", "--merge-output-format", "mkv")
			case "webm":
				args = append(args, "-f", "bestvideo[ext=webm]+bestaudio[ext=webm]/bestvideo+bestaudio/best", "--merge-output-format", "webm")
			default:
				args = append(args, "-f", "bestvideo+bestaudio/best", "--merge-output-format", container)
			}
		}
	} else {
		// Single stream fallback without ffmpeg
		if height > 0 {
			if container == "mp4" {
				args = append(args, "-f", fmt.Sprintf("best[height<=%d][ext=mp4]/best[height<=%d]/best", height, height))
			} else {
				args = append(args, "-f", fmt.Sprintf("best[height<=%d]/best", height))
			}
		} else {
			if container == "mp4" {
				args = append(args, "-f", "best[ext=mp4]/best")
			} else {
				args = append(args, "-f", "best")
			}
		}
	}

	return args
}

// YtDlpEngine manages yt-dlp execution, downloading, and persistence.
type YtDlpEngine struct {
	mu           sync.RWMutex
	baseDir      string
	configFile   string
	historyFile  string
	config       YtDlpConfig
	history      []*DownloadItem
	activeItem   *DownloadItem
	progressSubs []func(*DownloadItem)
}

var (
	globalYtDlpEngine *YtDlpEngine
	globalYtDlpOnce   sync.Once
)

// GetYtDlpEngine returns the singleton instance of YtDlpEngine.
func GetYtDlpEngine() *YtDlpEngine {
	globalYtDlpOnce.Do(func() {
		baseDir := GetManager().BaseDir
		globalYtDlpEngine = NewYtDlpEngine(baseDir)
	})
	return globalYtDlpEngine
}

func NewYtDlpEngine(baseDir string) *YtDlpEngine {
	e := &YtDlpEngine{
		baseDir:     baseDir,
		configFile:  filepath.Join(baseDir, "ytdl_config.json"),
		historyFile: filepath.Join(baseDir, "downloads.json"),
	}
	e.loadConfig()
	e.loadHistory()
	return e
}

// GetExecutablePath ensures yt-dlp is available (local, extracted from binary, or in PATH).
func (e *YtDlpEngine) GetExecutablePath() (string, error) {
	// 1. Check local baseDir
	localPath := filepath.Join(e.baseDir, "yt-dlp.exe")
	if fi, err := os.Stat(localPath); err == nil && fi.Size() > 0 {
		return localPath, nil
	}

	// 2. Check dedicated AppData cache
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		localAppData = os.TempDir()
	}
	appDataDir := filepath.Join(localAppData, "ProcessManager", "bin")
	extractedPath := filepath.Join(appDataDir, "yt-dlp.exe")

	if fi, err := os.Stat(extractedPath); err == nil && fi.Size() == int64(len(embeddedYtDlp)) && len(embeddedYtDlp) > 0 {
		return extractedPath, nil
	}

	// 3. Extract from embedded bytes if available (Self-Contained 1-EXE support)
	if len(embeddedYtDlp) > 0 {
		_ = os.MkdirAll(appDataDir, 0755)
		err := os.WriteFile(extractedPath, embeddedYtDlp, 0755)
		if err == nil {
			return extractedPath, nil
		}
	}

	// 4. Fall back to system PATH
	if p, err := exec.LookPath("yt-dlp"); err == nil {
		return p, nil
	}

	return "", fmt.Errorf("yt-dlp.exe could not be found or extracted")
}

// FindFfmpeg locates ffmpeg on the user's system for high-res stream merging.
func FindFfmpeg() string {
	// 1. Check system PATH
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p
	}

	// 2. Check standard Windows WinGet Links directory
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData != "" {
		wingetLink := filepath.Join(localAppData, "Microsoft", "WinGet", "Links", "ffmpeg.exe")
		if _, err := os.Stat(wingetLink); err == nil {
			return wingetLink
		}
	}

	// 3. Check baseDir
	baseDir := GetManager().BaseDir
	localFF := filepath.Join(baseDir, "ffmpeg.exe")
	if _, err := os.Stat(localFF); err == nil {
		return localFF
	}

	return ""
}

func defaultDownloadsDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		p := filepath.Join(home, "Downloads", "ProcessManager")
		_ = os.MkdirAll(p, 0755)
		return p
	}
	p := filepath.Join(GetManager().BaseDir, "downloads")
	_ = os.MkdirAll(p, 0755)
	return p
}

func (e *YtDlpEngine) loadConfig() {
	e.config = YtDlpConfig{
		OutputDir:        defaultDownloadsDir(),
		LastFormat:       FormatVideoBest,
		LastMediaType:    "video",
		LastVideoFormat:  "mp4",
		LastVideoQuality: "1080p",
		LastAudioFormat:  "mp3",
		LastAudioQuality: "320k",
		DownloadPlaylist: true,
	}

	data, err := os.ReadFile(e.configFile)
	if err != nil {
		return
	}
	var cfg YtDlpConfig
	if err := json.Unmarshal(data, &cfg); err == nil {
		if cfg.OutputDir != "" {
			e.config.OutputDir = cfg.OutputDir
		}
		if cfg.LastFormat != "" {
			e.config.LastFormat = cfg.LastFormat
		}
		if cfg.LastMediaType != "" {
			e.config.LastMediaType = cfg.LastMediaType
		}
		if cfg.LastVideoFormat != "" {
			e.config.LastVideoFormat = cfg.LastVideoFormat
		}
		if cfg.LastVideoQuality != "" {
			e.config.LastVideoQuality = cfg.LastVideoQuality
		}
		if cfg.LastAudioFormat != "" {
			e.config.LastAudioFormat = cfg.LastAudioFormat
		}
		if cfg.LastAudioQuality != "" {
			e.config.LastAudioQuality = cfg.LastAudioQuality
		}
		e.config.DownloadPlaylist = cfg.DownloadPlaylist
	}
}

func (e *YtDlpEngine) SetDownloadPlaylist(enabled bool) {
	e.mu.Lock()
	e.config.DownloadPlaylist = enabled
	e.mu.Unlock()
	e.saveConfig()
}

func (e *YtDlpEngine) saveConfig() {
	data, err := json.MarshalIndent(e.config, "", "  ")
	if err == nil {
		_ = os.WriteFile(e.configFile, data, 0644)
	}
}

func (e *YtDlpEngine) GetConfig() YtDlpConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.config
}

func (e *YtDlpEngine) SetOutputDir(dir string) {
	clean := strings.TrimSpace(dir)
	if clean == "" {
		return
	}
	e.mu.Lock()
	e.config.OutputDir = clean
	e.mu.Unlock()
	e.saveConfig()
}

func (e *YtDlpEngine) SetLastFormat(fmt DownloadFormat) {
	e.mu.Lock()
	e.config.LastFormat = fmt
	e.mu.Unlock()
	e.saveConfig()
}

func (e *YtDlpEngine) SaveOptionsConfig(mediaType, videoFormat, videoQuality, audioFormat, audioQuality string) {
	e.mu.Lock()
	if mediaType != "" {
		e.config.LastMediaType = mediaType
	}
	if videoFormat != "" {
		e.config.LastVideoFormat = videoFormat
	}
	if videoQuality != "" {
		e.config.LastVideoQuality = videoQuality
	}
	if audioFormat != "" {
		e.config.LastAudioFormat = audioFormat
	}
	if audioQuality != "" {
		e.config.LastAudioQuality = audioQuality
	}
	e.mu.Unlock()
	e.saveConfig()
}

func (e *YtDlpEngine) loadHistory() {
	data, err := os.ReadFile(e.historyFile)
	if err != nil {
		return
	}
	var list []*DownloadItem
	if err := json.Unmarshal(data, &list); err == nil {
		e.history = list
	}
}

func (e *YtDlpEngine) saveHistory() {
	e.mu.RLock()
	list := make([]*DownloadItem, len(e.history))
	copy(list, e.history)
	e.mu.RUnlock()

	data, err := json.MarshalIndent(list, "", "  ")
	if err == nil {
		_ = os.WriteFile(e.historyFile, data, 0644)
	}
}

func (e *YtDlpEngine) GetHistory() []*DownloadItem {
	e.mu.RLock()
	defer e.mu.RUnlock()
	res := make([]*DownloadItem, len(e.history))
	copy(res, e.history)
	return res
}

func (e *YtDlpEngine) GetActiveItem() *DownloadItem {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.activeItem
}

// FetchMetadata retrieves video or playlist details without downloading.
func (e *YtDlpEngine) FetchMetadata(ctx context.Context, rawUrl string, allowPlaylist bool) (*VideoMetadata, error) {
	url := strings.TrimSpace(rawUrl)
	if url == "" {
		return nil, fmt.Errorf("URL cannot be empty")
	}

	binPath, err := e.GetExecutablePath()
	if err != nil {
		return nil, err
	}

	isPlaylistReq := IsPurePlaylistURL(url) || (allowPlaylist && IsPlaylistURL(url))

	var args []string
	if isPlaylistReq {
		args = []string{
			"--dump-single-json",
			"--flat-playlist",
			"--yes-playlist",
			"--no-warnings",
			"--extractor-args", "youtube:player_client=android,ios,web",
			url,
		}
	} else {
		args = []string{
			"--dump-single-json",
			"--no-playlist",
			"--no-warnings",
			"--extractor-args", "youtube:player_client=android,ios,web",
			url,
		}
	}

	cmd := exec.CommandContext(ctx, binPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp info failed: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse video JSON: %v", err)
	}

	meta := &VideoMetadata{
		WebpageURL: url,
	}

	if t, ok := raw["_type"].(string); ok && t == "playlist" {
		meta.IsPlaylist = true
		if id, ok := raw["id"].(string); ok {
			meta.ID = id
		}
		if t, ok := raw["title"].(string); ok {
			meta.Title = t
		}
		if u, ok := raw["uploader"].(string); ok {
			meta.Uploader = u
		} else if c, ok := raw["channel"].(string); ok {
			meta.Uploader = c
		}
		if thumb, ok := raw["thumbnail"].(string); ok {
			meta.Thumbnail = thumb
		}
		if entries, ok := raw["entries"].([]interface{}); ok {
			meta.PlaylistCount = len(entries)
			if meta.Thumbnail == "" && len(entries) > 0 {
				if first, ok := entries[0].(map[string]interface{}); ok {
					if th, ok := first["thumbnail"].(string); ok {
						meta.Thumbnail = th
					}
				}
			}
		}
		if pc, ok := raw["playlist_count"].(float64); ok && pc > 0 {
			meta.PlaylistCount = int(pc)
		}
		meta.DurationString = fmt.Sprintf("%d videos", meta.PlaylistCount)
		meta.MaxResolution = "Playlist"
		return meta, nil
	}

	meta.IsPlaylist = false
	if id, ok := raw["id"].(string); ok {
		meta.ID = id
	}
	if t, ok := raw["title"].(string); ok {
		meta.Title = t
	}
	if u, ok := raw["uploader"].(string); ok {
		meta.Uploader = u
	} else if c, ok := raw["channel"].(string); ok {
		meta.Uploader = c
	}
	if dur, ok := raw["duration"].(float64); ok {
		meta.Duration = int(dur)
		meta.DurationString = formatSeconds(int(dur))
	} else if ds, ok := raw["duration_string"].(string); ok {
		meta.DurationString = ds
	}
	if thumb, ok := raw["thumbnail"].(string); ok {
		meta.Thumbnail = thumb
	}
	if vc, ok := raw["view_count"].(float64); ok {
		meta.ViewCount = int64(vc)
	}

	if formats, ok := raw["formats"].([]interface{}); ok {
		resMap := make(map[int]bool)
		for _, f := range formats {
			if fMap, ok := f.(map[string]interface{}); ok {
				if h, ok := fMap["height"].(float64); ok && h > 0 {
					resMap[int(h)] = true
				}
			}
		}
		var resList []int
		for h := range resMap {
			resList = append(resList, h)
		}
		sort.Sort(sort.Reverse(sort.IntSlice(resList)))
		for _, h := range resList {
			meta.AvailableResolutions = append(meta.AvailableResolutions, fmt.Sprintf("%dp", h))
		}
		if len(resList) > 0 {
			maxH := resList[0]
			if maxH >= 2160 {
				meta.MaxResolution = "4K (2160p)"
			} else if maxH >= 1440 {
				meta.MaxResolution = "2K (1440p)"
			} else if maxH >= 1080 {
				meta.MaxResolution = "1080p FHD"
			} else if maxH >= 720 {
				meta.MaxResolution = "720p HD"
			} else {
				meta.MaxResolution = fmt.Sprintf("%dp", maxH)
			}
		}
	}

	return meta, nil
}

func formatSeconds(sec int) string {
	if sec <= 0 {
		return "00:00"
	}
	h := sec / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

func formatViewCount(vc int64) string {
	if vc >= 1_000_000_000 {
		return fmt.Sprintf("%.1fB views", float64(vc)/1_000_000_000)
	}
	if vc >= 1_000_000 {
		return fmt.Sprintf("%.1fM views", float64(vc)/1_000_000)
	}
	if vc >= 1_000 {
		return fmt.Sprintf("%.1fK views", float64(vc)/1_000)
	}
	if vc > 0 {
		return fmt.Sprintf("%d views", vc)
	}
	return ""
}

// StartDownloadWithOptions initiates an asynchronous download with granular format and quality options.
func (e *YtDlpEngine) StartDownloadWithOptions(url, title, mediaType, container, quality, outputDir string, isPlaylist bool, onProgress func(*DownloadItem)) (*DownloadItem, error) {
	e.mu.Lock()
	if e.activeItem != nil && e.activeItem.Status == "downloading" {
		e.mu.Unlock()
		return nil, fmt.Errorf("a download is already in progress. Please wait or cancel it first")
	}

	binPath, err := e.GetExecutablePath()
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}

	if outputDir == "" {
		outputDir = e.config.OutputDir
	}
	_ = os.MkdirAll(outputDir, 0755)

	if mediaType == "" {
		mediaType = "video"
	}
	if container == "" {
		if mediaType == "audio" {
			container = "mp3"
		} else {
			container = "mp4"
		}
	}
	if quality == "" {
		quality = "best"
	}

	if title == "" {
		title = url
	}

	formatLabel := FormatDisplayLabel(mediaType, container, quality)
	id := generateTodoID()
	item := &DownloadItem{
		ID:          id,
		URL:         url,
		Title:       title,
		MediaType:   mediaType,
		Container:   container,
		Quality:     quality,
		Format:      DownloadFormat(fmt.Sprintf("%s_%s_%s", mediaType, container, quality)),
		FormatLabel: formatLabel,
		OutputDir:   outputDir,
		Status:      "downloading",
		Progress:    0.0,
		CreatedAt:   time.Now(),
		IsPlaylist:  isPlaylist,
	}

	e.activeItem = item
	if mediaType == "audio" {
		e.config.LastMediaType = "audio"
		e.config.LastAudioFormat = container
		e.config.LastAudioQuality = quality
	} else {
		e.config.LastMediaType = "video"
		e.config.LastVideoFormat = container
		e.config.LastVideoQuality = quality
	}
	e.mu.Unlock()
	e.saveConfig()

	ctx, cancel := context.WithCancel(context.Background())
	item.cancelFunc = cancel

	go e.runDownloadWorker(ctx, binPath, item, onProgress)

	return item, nil
}

// StartDownload maintains backward compatibility for legacy callers.
func (e *YtDlpEngine) StartDownload(url string, format DownloadFormat, outputDir string, onProgress func(*DownloadItem)) (*DownloadItem, error) {
	mType, container, quality := ParseFormatComponents(format)
	isPlaylist := e.config.DownloadPlaylist && IsPlaylistURL(url)
	return e.StartDownloadWithOptions(url, "", mType, container, quality, outputDir, isPlaylist, onProgress)
}

func (e *YtDlpEngine) runDownloadWorker(ctx context.Context, binPath string, item *DownloadItem, onProgress func(*DownloadItem)) {
	ffmpegPath := FindFfmpeg()

	var args []string
	if item.IsPlaylist {
		args = append(args,
			"--yes-playlist",
			"--ignore-errors",
			"--newline",
			"--no-warnings",
			"--windows-filenames",
			"--extractor-args", "youtube:player_client=android,ios,web",
			"-o", filepath.Join(item.OutputDir, "%(playlist_title,playlist)s", "%(playlist_index)s - %(title)s.%(ext)s"),
		)
	} else {
		args = append(args,
			"--no-playlist",
			"--newline",
			"--no-warnings",
			"--windows-filenames",
			"--extractor-args", "youtube:player_client=android,ios,web",
			"-o", filepath.Join(item.OutputDir, "%(title)s.%(ext)s"),
		)
	}

	if ffmpegPath != "" {
		args = append(args, "--ffmpeg-location", ffmpegPath)
	}

	// Resolve mediaType, container, and quality
	if item.MediaType == "" || item.Container == "" || item.Quality == "" {
		mType, cont, qual := ParseFormatComponents(item.Format)
		item.MediaType = mType
		item.Container = cont
		item.Quality = qual
	}

	formatArgs := BuildYtDlpFormatArgs(item.MediaType, item.Container, item.Quality, ffmpegPath)
	args = append(args, formatArgs...)
	args = append(args, item.URL)

	cmd := exec.CommandContext(ctx, binPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
		HideWindow:    true,
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		e.finishItem(item, "error", fmt.Sprintf("StdoutPipe error: %v", err), onProgress)
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		e.finishItem(item, "error", fmt.Sprintf("StderrPipe error: %v", err), onProgress)
		return
	}

	if err := cmd.Start(); err != nil {
		e.finishItem(item, "error", fmt.Sprintf("Failed to launch yt-dlp: %v", err), onProgress)
		return
	}

	item.cmd = cmd

	// Drain stderr in background to capture warnings/errors and prevent pipe congestion
	go func() {
		errScanner := bufio.NewScanner(stderr)
		for errScanner.Scan() {
			txt := strings.TrimSpace(errScanner.Text())
			if txt != "" {
				item.LogSnippet = txt
			}
		}
	}()

	// Create Job Object to guarantee cleanup of yt-dlp and ffmpeg child processes
	jobHandle, _, _ := procCreateJobObjectW.Call(0, 0)
	if jobHandle != 0 {
		var info JOBOBJECT_EXTENDED_LIMIT_INFORMATION
		info.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		procSetInformationJobObject.Call(
			jobHandle,
			JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)),
			uintptr(unsafe.Sizeof(info)),
		)
		hProcess, _, _ := procOpenProcess.Call(PROCESS_TERMINATE|PROCESS_QUERY_INFORMATION, 0, uintptr(cmd.Process.Pid))
		if hProcess != 0 {
			procAssignProcessToJobObject.Call(jobHandle, hProcess)
			procCloseHandle.Call(hProcess)
		}
		defer procCloseHandle.Call(jobHandle)
	}

	// Progress Regex Patterns
	reProgress := regexp.MustCompile(`\[download\]\s+([\d\.]+)%\s+of\s+~?([^\s]+)\s+at\s+([^\s]+)\s+ETA\s+([^\s]+)`)
	reFinished := regexp.MustCompile(`\[download\]\s+100(?:\.0)?%`)
	reDest := regexp.MustCompile(`\[(?:download|Merger|ExtractAudio)\]\s+(?:Destination:\s+|Merging formats into\s+)"?([^"\r\n]+)"?`)
	reAlready := regexp.MustCompile(`\[download\]\s+(.*)\s+has already been downloaded`)
	rePlaylistItem := regexp.MustCompile(`\[download\]\s+Downloading\s+item\s+(\d+)\s+of\s+(\d+)`)
	rePlaylistCount := regexp.MustCompile(`\[download\]\s+Playlist\s+(?:.*?):\s+Downloading\s+(\d+)\s+items`)
	rePlaylistTitle := regexp.MustCompile(`\[download\]\s+Downloading\s+playlist:\s*(.+)`)

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		item.LogSnippet = line

		// Match Playlist title
		if m := rePlaylistTitle.FindStringSubmatch(line); len(m) > 1 {
			pTitle := strings.TrimSpace(m[1])
			if pTitle != "" {
				item.PlaylistTitle = pTitle
				if item.Title == item.URL || item.Title == "" {
					item.Title = pTitle
				}
			}
		}

		// Match Playlist items
		if m := rePlaylistItem.FindStringSubmatch(line); len(m) > 2 {
			if cur, err := strconv.Atoi(m[1]); err == nil {
				item.CurrentItem = cur
			}
			if tot, err := strconv.Atoi(m[2]); err == nil {
				item.TotalItems = tot
			}
		} else if m := rePlaylistCount.FindStringSubmatch(line); len(m) > 1 {
			if tot, err := strconv.Atoi(m[1]); err == nil && tot > 0 {
				item.TotalItems = tot
			}
		}

		// Match Destination filename
		if m := reDest.FindStringSubmatch(line); len(m) > 1 {
			p := strings.TrimSpace(m[1])
			if p != "" {
				if item.IsPlaylist {
					item.FilePath = filepath.Dir(p)
				} else {
					item.FilePath = p
					base := filepath.Base(p)
					item.Title = strings.TrimSuffix(base, filepath.Ext(base))
				}
			}
		} else if m := reAlready.FindStringSubmatch(line); len(m) > 1 {
			p := strings.TrimSpace(m[1])
			if p != "" {
				if item.IsPlaylist {
					item.FilePath = filepath.Dir(p)
				} else {
					item.FilePath = p
					base := filepath.Base(p)
					item.Title = strings.TrimSuffix(base, filepath.Ext(base))
					item.Progress = 100.0
				}
			}
		}

		// Match Progress line
		if m := reProgress.FindStringSubmatch(line); len(m) > 4 {
			if pct, err := strconv.ParseFloat(m[1], 64); err == nil {
				item.Progress = pct
			}
			item.TotalSize = m[2]
			item.Speed = m[3]
			item.ETA = m[4]
		} else if reFinished.MatchString(line) {
			if !item.IsPlaylist {
				item.Progress = 100.0
			}
		}

		if onProgress != nil {
			onProgress(item)
		}
	}

	waitErr := cmd.Wait()

	if ctx.Err() == context.Canceled {
		e.finishItem(item, "cancelled", "Download cancelled by user", onProgress)
		return
	}

	if waitErr != nil {
		errMsg := fmt.Sprintf("Download failed: %v", waitErr)
		if item.LogSnippet != "" {
			errMsg = item.LogSnippet
		}
		e.finishItem(item, "error", errMsg, onProgress)
		return
	}

	// Verify file or playlist folder on disk and retrieve size
	if item.FilePath != "" {
		if fi, err := os.Stat(item.FilePath); err == nil {
			if fi.IsDir() {
				var totalSize int64
				_ = filepath.Walk(item.FilePath, func(_ string, info os.FileInfo, err error) error {
					if err == nil && !info.IsDir() {
						totalSize += info.Size()
					}
					return nil
				})
				if totalSize > 0 {
					item.FileSize = formatFileSize(totalSize)
				}
			} else {
				item.FileSize = formatFileSize(fi.Size())
			}
		}
	}

	item.Progress = 100.0
	e.finishItem(item, "completed", "", onProgress)
}

func formatFileSize(bytes int64) string {
	if bytes >= 1024*1024*1024 {
		return fmt.Sprintf("%.2f GB", float64(bytes)/(1024*1024*1024))
	}
	if bytes >= 1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	}
	if bytes >= 1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%d B", bytes)
}

func (e *YtDlpEngine) finishItem(item *DownloadItem, status, errStr string, onProgress func(*DownloadItem)) {
	e.mu.Lock()
	item.Status = status
	item.Error = errStr
	now := time.Now()
	item.CompletedAt = &now
	item.cmd = nil
	item.cancelFunc = nil

	// Prepend to history
	e.history = append([]*DownloadItem{item}, e.history...)
	if len(e.history) > 100 {
		e.history = e.history[:100]
	}

	if e.activeItem == item {
		e.activeItem = nil
	}
	e.mu.Unlock()

	e.saveHistory()

	if onProgress != nil {
		onProgress(item)
	}
}

// CancelDownload terminates active download job.
func (e *YtDlpEngine) CancelDownload(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.activeItem != nil && (id == "" || e.activeItem.ID == id) {
		if e.activeItem.cancelFunc != nil {
			e.activeItem.cancelFunc()
		}
		if e.activeItem.cmd != nil && e.activeItem.cmd.Process != nil {
			_ = e.activeItem.cmd.Process.Kill()
		}
		return true
	}
	return false
}

// DeleteHistoryItem removes an item from history and optionally removes the downloaded file.
func (e *YtDlpEngine) DeleteHistoryItem(id string, deleteFile bool) bool {
	e.mu.Lock()
	idx := -1
	var target *DownloadItem
	for i, it := range e.history {
		if it.ID == id {
			idx = i
			target = it
			break
		}
	}
	if idx >= 0 {
		e.history = append(e.history[:idx], e.history[idx+1:]...)
	}
	e.mu.Unlock()

	if idx >= 0 {
		e.saveHistory()
		if deleteFile && target != nil && target.FilePath != "" {
			_ = os.Remove(target.FilePath)
		}
		return true
	}
	return false
}

// ClearCompletedHistory clears all completed or cancelled items from history.
func (e *YtDlpEngine) ClearCompletedHistory() int {
	e.mu.Lock()
	var kept []*DownloadItem
	removed := 0
	for _, it := range e.history {
		if it.Status == "downloading" {
			kept = append(kept, it)
		} else {
			removed++
		}
	}
	e.history = kept
	e.mu.Unlock()

	if removed > 0 {
		e.saveHistory()
	}
	return removed
}
