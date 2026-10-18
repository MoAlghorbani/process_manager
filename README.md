# Windows Process Manager & Supervisor (Go Engine)

A high-performance, standalone native desktop process supervisor and manager written in **Go** for Windows services, AI SDKs, and background workers.

---

## 🖥️ 100% Native Desktop GUI (Fyne Powered - Zero WebView)

The Go Process Manager runs as a **100% native hardware-accelerated desktop application** powered by **Fyne** (`fyne.io/fyne/v2`). It has **zero dependency on WebView2, Chromium, Edge, or external web browsers**.

### Method 1: Double-Click Launcher
Double-click [run.bat](file:///e:/process_manager/run.bat) in Windows Explorer:
```
e:\process_manager\run.bat
```
*Launches `pm.exe gui` directly into its native desktop application window.*

### Method 2: Command Line
```bash
.\pm.exe gui
```

### Desktop Window Features:
- 🚀 **100% Native Desktop Window**: Built on pure Go & Fyne with GPU acceleration (zero browser engine / zero WebView2 runtime required).
- 📁 **Interactive Group Filter Bar & Management**: Dynamic tabs (`🌐 ALL (6)`, `📁 AI SDKs (3)`, `📁 OCR Services (3)`) with live process counts and a dedicated **`[⚙️ Manage Groups]`** modal button. Rename groups, dissolve groups, and move services between groups on the fly.
- ⚡ **Group Batch Controls**: Dedicated `[▶ Start Group]`, `[⏹ Stop Group]`, and `[🔄 Restart Group]` buttons to control all group servers in one click.
- ✏️ **Native Process Editor**: Dedicated `[✏️ Edit]` button on every service row opening a pre-populated dialog to update name, group, command line, working directory, port, venv, and auto-restart policies with immediate live apply.
- ⚡ **Direct In-Memory Supervision**: Interacts directly with the Go `ProcessManager` in memory for instantaneous status and action dispatch.
- 🎨 **Sleek Modern Dark Theme**: Status color badges (Green for Running, Red for Stopped, Amber for Crashed/Restarting).
- 📊 **Real-Time Process Grid**: Live tracking of PID, Port, RAM memory, Restarts, and active Uptime.
- 📜 **Integrated Real-Time Log Viewer**: Split-panel viewer displaying live logs with auto-refresh and clear buttons.
- ➕ **Native Add Service Dialog**: Native modal dialog to configure service name, command, cwd, port, and auto-restart policy.
- 🔐 **Credentials Vault (Tab 3)**: Securely manage platform logins (Platform, Username, Password, URL, Notes) with 1-click `[Copy User]` and `[Copy Pass]`, show/hide toggles, and integrated random password generator (`🎲 Generate`). Persisted to `credentials.json`.
- 📋 **Kanban Task Board (Tab 4)**: 3-column Kanban workflow (`📋 TO DO`, `🚧 IN PROGRESS`, `✅ DONE`) with modal dialog creation (`➕ Add Task`), project selection (`📁 Project`), scheduled notification reminders (`⏰ Due/Reminder` with Windows desktop notifications), 1-click status advancement, debounced search, and priority tags (`🔥 HIGH`, `NORMAL`, `💤 LOW`). Persisted to `todos.json`.
- 📥 **YouTube & Media Downloader (Tab 5)**: 100% self-contained single-executable video and audio downloader powered by embedded `yt-dlp` and FFmpeg. Download 1080p/4K MP4, 720p, 480p, and MP3/M4A audio with real-time speed/ETA progress bars, clipboard URL auto-paste & preview, output folder selector, and recent download history with 1-click `[Open File]` and `[Show in Folder]`.
- 🎮 **Training Hub (Tab 6)**: Zero-overhead embedded WebView2 window for League skillshot dodge training and 3D FPS aim practice.
- 🛡️ **One-Click Admin Elevation**: Click `[🛡️ Elevate]` to restart with Windows Administrator privileges via UAC.

---

## 💻 CLI Commands (`pm.exe`)

You can also control everything directly from your terminal:

```bash
# View all services status table (includes GROUP column)
.\pm.exe list
# Or simply:
.\pm.exe

# YouTube & Media Downloader (Embedded yt-dlp)
.\pm.exe ytdl "https://www.youtube.com/watch?v=..."                   # Download best quality MP4
.\pm.exe ytdl "https://www.youtube.com/watch?v=..." --format 1080p     # Download 1080p Full HD
.\pm.exe ytdl "https://www.youtube.com/watch?v=..." --audio            # Download audio as MP3
.\pm.exe ytdl "https://www.youtube.com/watch?v=..." --dir "D:\Videos" # Custom destination folder

# Group Management
.\pm.exe group list                                  # List groups with member services
.\pm.exe group rename "AI SDKs" "Machine Learning"   # Rename group across all services & tasks
.\pm.exe group move "ocr client" "Web Apps"          # Move a service to a new or existing group
.\pm.exe group delete "TempGroup"                    # Dissolve group (moves services to default)
.\pm.exe start-group "AI SDKs"                       # Start all services in group
.\pm.exe stop-group "AI SDKs"                        # Stop all services in group
.\pm.exe restart-group "AI SDKs"                     # Restart all services in group

# Kanban Board & Tasks
.\pm.exe todo                          # View Kanban overview table (shows Project & Column)
.\pm.exe todo add "Deploy new service" --project "AI SDKs" # Add task with project
.\pm.exe todo add "Hotfix" --project "complain" --high --doing # Add high-priority in progress
.\pm.exe todo start <id>               # Move task to IN PROGRESS
.\pm.exe todo done <id>                # Move task to DONE (supports prefix ID)
.\pm.exe todo todo <id>                # Move task back to TO DO
.\pm.exe todo delete <id>              # Delete a task
.\pm.exe todo clear                    # Clear all DONE tasks

# Filter services table by specific group
.\pm.exe list --group "OCR Services"
.\pm.exe list --group "AI SDKs"

# View registered groups and member service summary
.\pm.exe groups

# Batch lifecycle controls for groups:
.\pm.exe start-group "AI SDKs"
.\pm.exe stop-group "AI SDKs"
.\pm.exe restart-group "AI SDKs"

# View stored credentials vault from terminal
.\pm.exe creds
.\pm.exe creds "github"

# View permanent clipboard history from terminal
.\pm.exe clips
.\pm.exe clips "search keyword"

# View service logs
.\pm.exe logs ocr -n 50

# YouTube / Media Downloader CLI:
.\pm.exe ytdl "https://www.youtube.com/watch?v=..."
.\pm.exe ytdl "https://www.youtube.com/watch?v=..." --format 1080p
.\pm.exe ytdl "https://www.youtube.com/watch?v=..." --audio --format mp3 --quality 320k
.\pm.exe ytdl "https://www.youtube.com/watch?v=..." --audio --format flac --dir "D:\Music"

# Optional: Run as local web browser server instead of desktop window
.\pm.exe web
```

---

## 📥 Media & YouTube Downloader (Embedded 1-EXE yt-dlp)

The native GUI includes a dedicated **Downloader** tab powered by an embedded `yt-dlp.exe` binary bundled inside `pm.exe`:

- **Single Portable Executable:** No external `yt-dlp` install required; auto-extracts to `%LOCALAPPDATA%\ProcessManager\bin\yt-dlp.exe` on-demand.
- **Granular Format & Quality Controls:**
  - **Video Formats:** MP4, MKV, WEBM
  - **Video Qualities:** Best Available, 4K (2160p), 2K (1440p), 1080p (Full HD), 720p (HD), 480p (SD), 360p, 240p
  - **Audio Formats:** MP3, M4A (AAC), FLAC (Lossless), WAV (Raw), OPUS, OGG
  - **Audio Bitrates:** Best Quality (VBR 0), 320 kbps, 256 kbps, 192 kbps, 128 kbps, 96 kbps
- **Dynamic Mode Switcher:** Instant toggle between `🎬 Video` and `🎵 Audio Only` with persistent preferences.
- **Video Preview Card:** Displays title, uploader, duration, view count, and automatically detects maximum resolution and available streams.
- **Real-Time Telemetry:** Live progress bar, speed (MB/s), ETA countdown, and file size.
- **Recent Downloads History:** Persistent download records with one-click **Open File**, **Show in Folder**, and **Delete**.
- **Anti-Bot Bypass:** Automatically applies `--extractor-args "youtube:player_client=android,ios,web"` to bypass `HTTP 429` rate limits.
- **FFmpeg Auto-Detection:** Automatically discovers FFmpeg on PATH or WinGet Links for 1080p/4K audio-video muxing and audio conversions.

---

## 🛠️ Building the Project from Source

### 1. Prerequisites

Before compiling `pm.exe`, ensure your development environment has the following tools installed:

* **Go (1.21 or newer)**:
  Download and install from [golang.org](https://go.dev/dl/). Verify with:
  ```powershell
  go version
  ```
* **C Compiler (CGO Enabled)**:
  Fyne (`fyne.io/fyne/v2`) uses hardware-accelerated OpenGL bindings on Windows and requires a C compiler (`gcc`).
  * Install **MSYS2** (UCRT64 or MinGW64) from [msys2.org](https://www.msys2.org/):
    ```bash
    pacman -S --needed base-devel mingw-w64-ucrt-x86_64-toolchain
    ```
  * Ensure `gcc.exe` is located at `C:\msys64\ucrt64\bin` or added to your system `PATH`.
* **Embedded Downloader Binary (`yt-dlp.exe`)**:
  * The project embeds `yt-dlp.exe` directly into the binary via Go's `//go:embed yt-dlp.exe` in `ytdl.go`.
  * Ensure `yt-dlp.exe` is present in the repository root directory (`e:\process_manager\yt-dlp.exe`).
* **Optional Runtime Tools**:
  * **FFmpeg**: Automatically detected by `pm.exe` for merging separate 1080p/4K audio+video streams and converting audio formats (e.g. `winget install Gyan.FFmpeg`).
  * **WebView2 Runtime**: Included by default in Windows 10/11; used by Tab 6 (Training Hub).

---

### 2. Build Methods

#### Option A: One-Click Build Script (Recommended)
Run [build.bat](file:///e:/process_manager/build.bat) from Command Prompt, PowerShell, or by double-clicking it in File Explorer:
```cmd
build.bat
```
*`build.bat` automatically configures the MSYS2 UCRT64 toolchain in the local session `PATH` and builds `pm.exe`.*

#### Option B: Manual Command Line Build
Open PowerShell in the project directory:
```powershell
# 1. Add GCC compiler to current session PATH
$env:PATH = "C:\msys64\ucrt64\bin;" + $env:PATH

# 2. Build the executable
go build -v -o pm.exe .
```

#### Option C: Pure Windowed GUI Build (Hide Console Window)
By default, `pm.exe` is built as a hybrid CLI + GUI binary so that commands like `pm.exe list` print output to your terminal. If you want a GUI-only binary that never opens a console window when launched from the desktop:
```powershell
$env:PATH = "C:\msys64\ucrt64\bin;" + $env:PATH
go build -ldflags "-H=windowsgui" -o pm.exe .
```
*(Note: When using `-H=windowsgui`, running CLI commands like `pm list` will execute without writing output to standard console streams).*

---

### 3. Troubleshooting Common Build Issues

| Issue / Error | Cause | Resolution |
| :--- | :--- | :--- |
| `exec: "gcc": executable file not found in %PATH%` | CGO is enabled by default for Fyne, but `gcc` is not in your current PATH. | Run `set "PATH=C:\msys64\ucrt64\bin;%PATH%"` or use `build.bat`. |
| `pattern yt-dlp.exe: cannot embed file: no such file or directory` | `yt-dlp.exe` is missing from the project root. | Place `yt-dlp.exe` in the root folder alongside `main.go`. |
| `open pm.exe: Access is denied.` | An existing instance of `pm.exe` is currently running and locked by Windows. | Terminate the running instance first: `taskkill /F /IM pm.exe`, then re-run `build.bat`. |
| `undefined: ...` (missing dependencies) | Go modules have not been downloaded. | Run `go mod download` and `go mod tidy` before building. |

