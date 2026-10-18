package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// Windows DLL procs for Admin elevation and native process metrics
var (
	psapiDLL               = syscall.NewLazyDLL("psapi.dll")
	procOpenProcess        = kernel32DLL.NewProc("OpenProcess")
	procCloseHandle        = kernel32DLL.NewProc("CloseHandle")
	procTerminateProcess   = kernel32DLL.NewProc("TerminateProcess")
	procGetProcessMemInfo  = psapiDLL.NewProc("GetProcessMemoryInfo")
	procQueryFullImageName = kernel32DLL.NewProc("QueryFullProcessImageNameW")

	procCreateJobObjectW         = kernel32DLL.NewProc("CreateJobObjectW")
	procSetInformationJobObject  = kernel32DLL.NewProc("SetInformationJobObject")
	procAssignProcessToJobObject = kernel32DLL.NewProc("AssignProcessToJobObject")
	procTerminateJobObject       = kernel32DLL.NewProc("TerminateJobObject")

	procCreateToolhelp32Snapshot = kernel32DLL.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW          = kernel32DLL.NewProc("Process32FirstW")
	procProcess32NextW           = kernel32DLL.NewProc("Process32NextW")

	shell32DLL     = syscall.NewLazyDLL("shell32.dll")
	procIsAdmin    = shell32DLL.NewProc("IsUserAnAdmin")
	procShellExecW = shell32DLL.NewProc("ShellExecuteW")
)

const (
	PROCESS_TERMINATE                 = 0x0001
	PROCESS_QUERY_INFORMATION         = 0x0400
	PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
	PROCESS_VM_READ                   = 0x0010

	TH32CS_SNAPPROCESS = 0x00000002

	JobObjectExtendedLimitInformation = 9
	JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE = 0x2000
)

type IO_COUNTERS struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type JOBOBJECT_BASIC_LIMIT_INFORMATION struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type JOBOBJECT_EXTENDED_LIMIT_INFORMATION struct {
	BasicLimitInformation JOBOBJECT_BASIC_LIMIT_INFORMATION
	IoInfo                IO_COUNTERS
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

type PROCESSENTRY32W struct {
	Size            uint32
	CntUsage        uint32
	ProcessID       uint32
	DefaultHeapID   uintptr
	ModuleID        uint32
	CntThreads      uint32
	ParentProcessID uint32
	PriClassBase    int32
	Flags           uint32
	ExeFile         [260]uint16
}

type PROCESS_MEMORY_COUNTERS struct {
	Cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

// IsAdmin checks whether the current process has Windows Administrator privileges.
func IsAdmin() bool {
	if err := procIsAdmin.Find(); err != nil {
		return false
	}
	ret, _, _ := procIsAdmin.Call()
	return ret != 0
}

// RequestAdminElevation relaunches the current executable as Administrator via UAC.
func RequestAdminElevation(args []string) error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	verbPtr, _ := syscall.UTF16PtrFromString("runas")
	exePtr, _ := syscall.UTF16PtrFromString(exePath)
	argsPtr, _ := syscall.UTF16PtrFromString(strings.Join(args, " "))
	dirPtr, _ := syscall.UTF16PtrFromString(filepath.Dir(exePath))

	ret, _, callErr := procShellExecW.Call(
		0,
		uintptr(unsafe.Pointer(verbPtr)),
		uintptr(unsafe.Pointer(exePtr)),
		uintptr(unsafe.Pointer(argsPtr)),
		uintptr(unsafe.Pointer(dirPtr)),
		1, // SW_SHOWNORMAL
	)

	if ret <= 32 {
		return fmt.Errorf("ShellExecute failed (code %d): %v", ret, callErr)
	}
	return nil
}

// getProcessName returns the executable name for a given process ID via native Win32.
func getProcessName(pid int) string {
	if pid <= 0 {
		return ""
	}
	hProcess, _, _ := procOpenProcess.Call(
		PROCESS_QUERY_LIMITED_INFORMATION,
		0,
		uintptr(pid),
	)
	if hProcess == 0 {
		return ""
	}
	defer procCloseHandle.Call(hProcess)

	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	ret, _, _ := procQueryFullImageName.Call(
		hProcess,
		0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if ret != 0 {
		fullPath := syscall.UTF16ToString(buf[:size])
		return strings.ToLower(filepath.Base(fullPath))
	}
	return ""
}

// isProtectedProcess determines whether a process must never be killed by FreePort.
func isProtectedProcess(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	if strings.Contains(name, "docker") || strings.Contains(name, "wsl") {
		return true
	}
	protected := []string{
		"system",
		"system idle process",
		"svchost.exe",
		"explorer.exe",
		"csrss.exe",
		"wininit.exe",
		"services.exe",
		"lsass.exe",
		"smss.exe",
		"pm.exe",
		"vmmem",
		"vmmemwsx",
	}
	for _, p := range protected {
		if name == p {
			return true
		}
	}
	return false
}

// TerminatePID forcefully terminates a process using direct Win32 TerminateProcess API.
func TerminatePID(pid int) bool {
	if pid <= 0 {
		return false
	}
	hProcess, _, _ := procOpenProcess.Call(PROCESS_TERMINATE, 0, uintptr(pid))
	if hProcess != 0 {
		procTerminateProcess.Call(hProcess, 1)
		procCloseHandle.Call(hProcess)
		return true
	}
	return false
}

// GetChildPIDs walks the Windows process tree and returns all immediate and descendant child PIDs.
func GetChildPIDs(parentPID int) []int {
	if parentPID <= 0 {
		return nil
	}

	hSnapshot, _, _ := procCreateToolhelp32Snapshot.Call(TH32CS_SNAPPROCESS, 0)
	if hSnapshot == uintptr(syscall.InvalidHandle) || hSnapshot == 0 {
		return nil
	}
	defer procCloseHandle.Call(hSnapshot)

	var entry PROCESSENTRY32W
	entry.Size = uint32(unsafe.Sizeof(entry))

	ret, _, _ := procProcess32FirstW.Call(hSnapshot, uintptr(unsafe.Pointer(&entry)))
	if ret == 0 {
		return nil
	}

	tree := make(map[int][]int)
	for {
		pid := int(entry.ProcessID)
		ppid := int(entry.ParentProcessID)
		tree[ppid] = append(tree[ppid], pid)

		ret, _, _ = procProcess32NextW.Call(hSnapshot, uintptr(unsafe.Pointer(&entry)))
		if ret == 0 {
			break
		}
	}

	var allDescendants []int
	var queue = []int{parentPID}
	visited := make(map[int]bool)

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		for _, child := range tree[curr] {
			if !visited[child] {
				visited[child] = true
				allDescendants = append(allDescendants, child)
				queue = append(queue, child)
			}
		}
	}

	return allDescendants
}

// KillProcessTree terminates a process and all its child workers using Win32 API and taskkill.
func KillProcessTree(pid int) error {
	if pid <= 0 {
		return nil
	}

	// 1. Terminate all child and descendant processes first so no orphans escape
	children := GetChildPIDs(pid)
	for _, childPID := range children {
		TerminatePID(childPID)
	}

	// 2. Terminate target PID directly via kernel
	TerminatePID(pid)

	// 3. Backup taskkill command to terminate any lingering processes
	cmd := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Run()

	return nil
}

// FreePort locates and terminates any non-protected process listening on or bound to the specified port.
func FreePort(port int) int {
	if port <= 0 {
		return 0
	}

	cmd := exec.Command("netstat", "-ano", "-p", "tcp")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return 0
	}

	lines := strings.Split(string(out), "\n")
	freedCount := 0
	killedPIDs := make(map[int]bool)
	myPID := os.Getpid()

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "TCP") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 5 {
			localAddr := fields[1]
			pidStr := fields[len(fields)-1]

			// Extract exact port number from localAddr (handles IPv4 and IPv6)
			parts := strings.Split(localAddr, ":")
			actualPortStr := parts[len(parts)-1]
			actualPort, err := strconv.Atoi(actualPortStr)
			if err != nil || actualPort != port {
				continue
			}

			if pid, err := strconv.Atoi(pidStr); err == nil && pid > 0 && pid != myPID && !killedPIDs[pid] {
				killedPIDs[pid] = true
				procName := getProcessName(pid)
				if isProtectedProcess(procName) {
					fmt.Printf("[FreePort] Port %d is occupied by protected process '%s' (PID %d). Skipping termination.\n", port, procName, pid)
					continue
				}
				TerminatePID(pid)
				_ = KillProcessTree(pid)
				freedCount++
			}
		}
	}
	return freedCount
}

// DetectVenvInDir checks if a directory contains a virtual environment (.venv, venv, env).
func DetectVenvInDir(dir string) string {
	if dir == "" {
		return ""
	}
	candidates := []string{".venv", "venv", "env"}
	for _, c := range candidates {
		checkPath := filepath.Join(dir, c, "Scripts", "python.exe")
		if _, err := os.Stat(checkPath); err == nil {
			return c
		}
	}
	return ""
}

// ServiceConfig defines the persisted configuration of a service.
type ServiceConfig struct {
	Name        string            `json:"name"`
	Group       string            `json:"group,omitempty"`
	Command     string            `json:"command"`
	Cwd         string            `json:"cwd"`
	Port        *int              `json:"port"`
	AutoRestart bool              `json:"auto_restart"`
	Venv        string            `json:"venv,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	SavedStatus string            `json:"saved_status,omitempty"`
	Shortcut    string            `json:"shortcut,omitempty"`
}

// NormalizeGroup trims and standardizes group names, defaulting empty strings to "default".
func NormalizeGroup(group string) string {
	trimmed := strings.TrimSpace(group)
	if trimmed == "" {
		return "default"
	}
	return trimmed
}

// MatchGroup checks if two group names match case-insensitively.
func MatchGroup(serviceGroup, targetGroup string) bool {
	return strings.EqualFold(NormalizeGroup(serviceGroup), NormalizeGroup(targetGroup))
}

// ManagedService represents a runtime managed background process.
type ManagedService struct {
	Config       ServiceConfig
	Cmd          *exec.Cmd
	JobHandle    syscall.Handle
	PID          int
	Status       string // "stopped", "running", "crashed", "restarting"
	StartTime    time.Time
	Restarts     int
	LastExitCode int
	ManualStop   bool
	LogFile      string
	lock         sync.Mutex
}

func (s *ManagedService) IsRunning() bool {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.Status == "running" || s.Status == "restarting"
}

func (s *ManagedService) ToMap() map[string]interface{} {
	s.lock.Lock()
	defer s.lock.Unlock()

	var uptimeSec int64 = 0
	if s.Status == "running" && !s.StartTime.IsZero() {
		uptimeSec = int64(time.Since(s.StartTime).Seconds())
	}

	group := NormalizeGroup(s.Config.Group)

	res := map[string]interface{}{
		"name":           s.Config.Name,
		"group":          group,
		"command":        s.Config.Command,
		"cwd":            s.Config.Cwd,
		"port":           s.Config.Port,
		"auto_restart":   s.Config.AutoRestart,
		"venv":           s.Config.Venv,
		"shortcut":       s.Config.Shortcut,
		"pid":            s.PID,
		"status":         s.Status,
		"restarts":       s.Restarts,
		"uptime_seconds": uptimeSec,
		"log_file":       s.LogFile,
		"last_exit_code": s.LastExitCode,
		"cpu_percent":    0.0,
		"memory_mb":      0.0,
	}

	// Try reading memory using tasklist if running
	if s.PID > 0 && s.Status == "running" {
		if mem := getProcessMemoryMB(s.PID); mem > 0 {
			res["memory_mb"] = mem
		}
	}
	return res
}

func getProcessMemoryMB(pid int) float64 {
	if pid <= 0 {
		return 0
	}
	hProcess, _, _ := procOpenProcess.Call(
		PROCESS_QUERY_INFORMATION|PROCESS_VM_READ,
		0,
		uintptr(pid),
	)
	if hProcess == 0 {
		return 0
	}
	defer procCloseHandle.Call(hProcess)

	var memCounters PROCESS_MEMORY_COUNTERS
	memCounters.Cb = uint32(unsafe.Sizeof(memCounters))

	ret, _, _ := procGetProcessMemInfo.Call(
		hProcess,
		uintptr(unsafe.Pointer(&memCounters)),
		uintptr(memCounters.Cb),
	)
	if ret == 0 {
		return 0
	}

	mb := float64(memCounters.WorkingSetSize) / (1024 * 1024)
	return float64(int(mb*10)) / 10.0
}

// ProcessManager coordinates services, state, and auto-restart loops.
type ProcessManager struct {
	BaseDir      string
	ServicesFile string
	LogsDir      string
	services     map[string]*ManagedService
	lock         sync.RWMutex
}

var (
	pmInstance *ProcessManager
	pmOnce     sync.Once
)

func GetManager() *ProcessManager {
	pmOnce.Do(func() {
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}
		pmInstance = &ProcessManager{
			BaseDir:      cwd,
			ServicesFile: filepath.Join(cwd, "services.json"),
			LogsDir:      filepath.Join(cwd, "logs"),
			services:     make(map[string]*ManagedService),
		}
		_ = os.MkdirAll(pmInstance.LogsDir, 0755)
		pmInstance.LoadState()
	})
	return pmInstance
}

func (pm *ProcessManager) AddService(cfg ServiceConfig, autoStart bool) *ManagedService {
	pm.lock.Lock()
	if existing, ok := pm.services[cfg.Name]; ok {
		existing.Stop()
	}

	if cfg.Cwd == "" {
		cfg.Cwd = pm.BaseDir
	}
	cfg.Cwd, _ = filepath.Abs(cfg.Cwd)
	cfg.Group = NormalizeGroup(cfg.Group)

	service := &ManagedService{
		Config: cfg,
		Status: "stopped",
	}
	pm.services[cfg.Name] = service
	pm.lock.Unlock()

	if autoStart {
		pm.StartService(cfg.Name)
	}
	pm.SaveState()
	return service
}

func (pm *ProcessManager) UpdateService(oldName string, cfg ServiceConfig, restartIfRunning bool) bool {
	pm.lock.Lock()
	service, exists := pm.services[oldName]
	if !exists {
		pm.lock.Unlock()
		return false
	}

	wasRunning := service.IsRunning()
	if wasRunning && restartIfRunning {
		service.Stop()
	}

	if oldName != cfg.Name {
		delete(pm.services, oldName)
		service.Config.Name = cfg.Name
	}

	if cfg.Cwd == "" {
		cfg.Cwd = pm.BaseDir
	}
	cfg.Cwd, _ = filepath.Abs(cfg.Cwd)
	cfg.Group = NormalizeGroup(cfg.Group)

	service.Config = cfg
	pm.services[cfg.Name] = service
	pm.lock.Unlock()

	if wasRunning && restartIfRunning {
		pm.StartService(cfg.Name)
	}
	pm.SaveState()
	return true
}

func (pm *ProcessManager) GetServiceConfig(name string) (ServiceConfig, bool) {
	pm.lock.RLock()
	defer pm.lock.RUnlock()
	s, exists := pm.services[name]
	if !exists {
		return ServiceConfig{}, false
	}
	return s.Config, true
}

func (pm *ProcessManager) StartService(name string) bool {
	pm.lock.RLock()
	s, exists := pm.services[name]
	pm.lock.RUnlock()
	if !exists {
		return false
	}

	var runWithWatcher func() bool
	runWithWatcher = func() bool {
		return s.Start(pm.LogsDir, func() {
			s.lock.Lock()
			manual := s.ManualStop
			exitCode := s.LastExitCode
			autoRestart := s.Config.AutoRestart
			s.lock.Unlock()

			if manual {
				s.lock.Lock()
				s.Status = "stopped"
				s.lock.Unlock()
				pm.SaveState()
				return
			}

			if exitCode == 0 && !autoRestart {
				s.lock.Lock()
				s.Status = "stopped"
				s.lock.Unlock()
				pm.SaveState()
				return
			}

			s.lock.Lock()
			s.Status = "crashed"
			s.lock.Unlock()
			pm.SaveState()

			if autoRestart {
				time.Sleep(1500 * time.Millisecond)
				s.lock.Lock()
				if s.ManualStop {
					s.Status = "stopped"
					s.lock.Unlock()
					pm.SaveState()
					return
				}
				s.Status = "restarting"
				s.Restarts++
				s.lock.Unlock()
				pm.SaveState()

				runWithWatcher()
			}
		})
	}

	success := runWithWatcher()
	if success {
		pm.SaveState()
	}
	return success
}

func (s *ManagedService) Start(logsDir string, onExit func()) bool {
	s.lock.Lock()
	if s.Status == "running" && s.Cmd != nil && s.Cmd.Process != nil {
		s.lock.Unlock()
		return true
	}

	s.ManualStop = false
	if s.Config.Port != nil && *s.Config.Port > 0 {
		FreePort(*s.Config.Port)
		time.Sleep(200 * time.Millisecond)
	}

	_ = os.MkdirAll(logsDir, 0755)
	s.LogFile = filepath.Join(logsDir, fmt.Sprintf("%s.log", s.Config.Name))

	logF, err := os.OpenFile(s.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		s.Status = "crashed"
		s.lock.Unlock()
		return false
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logF.WriteString(fmt.Sprintf("\n[%s] === STARTING SERVICE: %s ===\n", timestamp, s.Config.Name))
	logF.WriteString(fmt.Sprintf("Command: %s | CWD: %s\n", s.Config.Command, s.Config.Cwd))

	// Setup Environment & venv
	env := os.Environ()
	activeVenv := s.Config.Venv
	if activeVenv == "" {
		activeVenv = DetectVenvInDir(s.Config.Cwd)
	}

	if activeVenv != "" {
		venvDir := activeVenv
		if !filepath.IsAbs(venvDir) {
			venvDir = filepath.Join(s.Config.Cwd, venvDir)
		}
		scriptsDir := filepath.Join(venvDir, "Scripts")
		if _, err := os.Stat(scriptsDir); err == nil {
			env = append(env, fmt.Sprintf("VIRTUAL_ENV=%s", venvDir))
			pathVar := os.Getenv("PATH")
			env = append(env, fmt.Sprintf("PATH=%s;%s", scriptsDir, pathVar))
			logF.WriteString(fmt.Sprintf("Venv Activated: %s\n", venvDir))
		}
	}

	if s.Config.Port != nil {
		env = append(env, fmt.Sprintf("PORT=%d", *s.Config.Port))
	}
	for k, v := range s.Config.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	logF.WriteString("\n")
	_ = logF.Sync()

	// Create Windows Job Object to guarantee all spawned children/grandchildren are terminated on stop
	hJob, _, _ := procCreateJobObjectW.Call(0, 0)
	if hJob != 0 {
		info := JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
		info.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		procSetInformationJobObject.Call(
			hJob,
			JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)),
			uintptr(unsafe.Sizeof(info)),
		)
	}

	// Execute via cmd.exe so complex shell commands (go run, npm run, etc.) execute naturally
	cmd := exec.Command("cmd.exe", "/C", s.Config.Command)
	cmd.Dir = s.Config.Cwd
	cmd.Env = env
	cmd.Stdout = logF
	cmd.Stderr = logF
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
		HideWindow:    true,
	}

	if err := cmd.Start(); err != nil {
		if hJob != 0 {
			procCloseHandle.Call(hJob)
		}
		s.Status = "crashed"
		logF.WriteString(fmt.Sprintf("[%s] Launch Failed: %v\n", time.Now().Format("2006-01-02 15:04:05"), err))
		_ = logF.Close()
		s.lock.Unlock()
		return false
	}

	if hJob != 0 && cmd.Process != nil {
		hProc, _, _ := procOpenProcess.Call(0x0101, 0, uintptr(cmd.Process.Pid))
		if hProc != 0 {
			procAssignProcessToJobObject.Call(hJob, hProc)
			procCloseHandle.Call(hProc)
		}
		s.JobHandle = syscall.Handle(hJob)
	}

	s.Cmd = cmd
	s.PID = cmd.Process.Pid
	s.Status = "running"
	s.StartTime = time.Now()
	s.lock.Unlock()

	// Asynchronous exit watcher
	go func() {
		waitErr := cmd.Wait()
		_ = logF.Close()

		s.lock.Lock()
		s.PID = 0
		s.Cmd = nil
		if waitErr != nil {
			if exitErr, ok := waitErr.(*exec.ExitError); ok {
				s.LastExitCode = exitErr.ExitCode()
			} else {
				s.LastExitCode = 1
			}
		} else {
			s.LastExitCode = 0
		}
		s.lock.Unlock()

		if onExit != nil {
			onExit()
		}
	}()

	return true
}

func (s *ManagedService) Stop() bool {
	s.lock.Lock()
	s.ManualStop = true
	s.Status = "stopped"
	pid := s.PID
	port := s.Config.Port
	job := s.JobHandle
	s.PID = 0
	s.Cmd = nil
	s.JobHandle = 0
	s.lock.Unlock()

	// 1. Terminate entire Windows Job Object (kills all descendants at OS kernel level)
	if job != 0 {
		procTerminateJobObject.Call(uintptr(job), 1)
		procCloseHandle.Call(uintptr(job))
	}

	// 2. Kill the process tree (walks all children via Toolhelp snapshot)
	if pid > 0 {
		_ = KillProcessTree(pid)
	}

	// 3. Free the listening port if configured
	if port != nil && *port > 0 {
		FreePort(*port)
	}

	return true
}

func (s *ManagedService) Restart(logsDir string) bool {
	s.Stop()
	time.Sleep(300 * time.Millisecond)
	s.lock.Lock()
	s.Restarts++
	s.ManualStop = false
	s.lock.Unlock()
	return s.Start(logsDir, nil)
}

func (pm *ProcessManager) StopService(name string) bool {
	pm.lock.RLock()
	s, exists := pm.services[name]
	pm.lock.RUnlock()
	if !exists {
		return false
	}
	success := s.Stop()
	pm.SaveState()
	return success
}

func (pm *ProcessManager) RestartService(name string) bool {
	pm.lock.RLock()
	s, exists := pm.services[name]
	pm.lock.RUnlock()
	if !exists {
		return false
	}
	s.Stop()
	time.Sleep(300 * time.Millisecond)
	s.lock.Lock()
	s.Restarts++
	s.ManualStop = false
	s.lock.Unlock()
	return pm.StartService(name)
}

func (pm *ProcessManager) DeleteService(name string) bool {
	pm.lock.Lock()
	s, exists := pm.services[name]
	if !exists {
		pm.lock.Unlock()
		return false
	}
	s.Stop()
	delete(pm.services, name)
	pm.lock.Unlock()
	pm.SaveState()
	return true
}

func (pm *ProcessManager) StartAll() {
	pm.lock.RLock()
	defer pm.lock.RUnlock()
	for name := range pm.services {
		go pm.StartService(name)
	}
}

func (pm *ProcessManager) StopAll() {
	pm.lock.RLock()
	services := make([]*ManagedService, 0, len(pm.services))
	for _, s := range pm.services {
		services = append(services, s)
	}
	pm.lock.RUnlock()

	for _, s := range services {
		s.Stop()
	}
	pm.SaveState()
}

func (pm *ProcessManager) RestartAll() {
	pm.lock.RLock()
	services := make([]*ManagedService, 0, len(pm.services))
	for _, s := range pm.services {
		services = append(services, s)
	}
	pm.lock.RUnlock()

	for _, s := range services {
		s.Restart(pm.LogsDir)
	}
	pm.SaveState()
}

// getGroupsLocked returns unique group names. Caller must hold pm.lock.
func (pm *ProcessManager) getGroupsLocked() []string {
	groupMap := make(map[string]bool)
	for _, s := range pm.services {
		groupMap[NormalizeGroup(s.Config.Group)] = true
	}

	var groups []string
	for g := range groupMap {
		groups = append(groups, g)
	}
	sort.Strings(groups)
	return groups
}

// GetGroups returns a sorted list of unique group names across all registered services.
func (pm *ProcessManager) GetGroups() []string {
	pm.lock.RLock()
	defer pm.lock.RUnlock()
	return pm.getGroupsLocked()
}

// GetServicesByGroup returns all managed services belonging to a specified group.
func (pm *ProcessManager) GetServicesByGroup(group string) []*ManagedService {
	pm.lock.RLock()
	defer pm.lock.RUnlock()

	var list []*ManagedService
	for _, s := range pm.services {
		if MatchGroup(s.Config.Group, group) {
			list = append(list, s)
		}
	}
	return list
}

// StartGroup starts all services belonging to a specified group.
func (pm *ProcessManager) StartGroup(group string) []string {
	pm.lock.RLock()
	var toStart []string
	for name, s := range pm.services {
		if MatchGroup(s.Config.Group, group) {
			toStart = append(toStart, name)
		}
	}
	pm.lock.RUnlock()

	var started []string
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, name := range toStart {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			if pm.StartService(n) {
				mu.Lock()
				started = append(started, n)
				mu.Unlock()
			}
		}(name)
	}
	wg.Wait()
	sort.Strings(started)
	return started
}

// StopGroup stops all services belonging to a specified group.
func (pm *ProcessManager) StopGroup(group string) []string {
	pm.lock.RLock()
	var toStop []string
	for name, s := range pm.services {
		if MatchGroup(s.Config.Group, group) {
			toStop = append(toStop, name)
		}
	}
	pm.lock.RUnlock()

	var stopped []string
	for _, name := range toStop {
		if pm.StopService(name) {
			stopped = append(stopped, name)
		}
	}
	sort.Strings(stopped)
	return stopped
}

// RestartGroup restarts all services belonging to a specified group.
func (pm *ProcessManager) RestartGroup(group string) []string {
	pm.lock.RLock()
	var toRestart []string
	for name, s := range pm.services {
		if MatchGroup(s.Config.Group, group) {
			toRestart = append(toRestart, name)
		}
	}
	pm.lock.RUnlock()

	var restarted []string
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, name := range toRestart {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			if pm.RestartService(n) {
				mu.Lock()
				restarted = append(restarted, n)
				mu.Unlock()
			}
		}(name)
	}
	wg.Wait()
	sort.Strings(restarted)
	return restarted
}

// RenameGroup updates all services matching oldGroup to newGroup.
func (pm *ProcessManager) RenameGroup(oldGroup, newGroup string) int {
	normNew := NormalizeGroup(newGroup)
	pm.lock.Lock()
	count := 0
	for _, s := range pm.services {
		if MatchGroup(s.Config.Group, oldGroup) {
			s.Config.Group = normNew
			count++
		}
	}
	pm.lock.Unlock()

	if count > 0 {
		pm.SaveState()
	}
	return count
}

// MoveServiceToGroup changes the group of a single service.
func (pm *ProcessManager) MoveServiceToGroup(serviceName, newGroup string) bool {
	normNew := NormalizeGroup(newGroup)
	pm.lock.Lock()
	s, exists := pm.services[serviceName]
	if !exists {
		pm.lock.Unlock()
		return false
	}
	s.Config.Group = normNew
	pm.lock.Unlock()

	pm.SaveState()
	return true
}

// DeleteGroup resets all services in group to fallbackGroup.
func (pm *ProcessManager) DeleteGroup(group, fallbackGroup string) int {
	normFallback := NormalizeGroup(fallbackGroup)
	pm.lock.Lock()
	count := 0
	for _, s := range pm.services {
		if MatchGroup(s.Config.Group, group) {
			s.Config.Group = normFallback
			count++
		}
	}
	pm.lock.Unlock()

	if count > 0 {
		pm.SaveState()
	}
	return count
}

func (pm *ProcessManager) GetLogs(name string, linesCount int) string {
	logPath := filepath.Join(pm.LogsDir, fmt.Sprintf("%s.log", name))
	file, err := os.Open(logPath)
	if err != nil {
		return "(No logs recorded yet)"
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > linesCount {
			lines = lines[1:]
		}
	}
	return strings.Join(lines, "\n")
}

func (pm *ProcessManager) ClearLogs(name string) bool {
	logPath := filepath.Join(pm.LogsDir, fmt.Sprintf("%s.log", name))
	err := os.WriteFile(logPath, []byte(""), 0644)
	return err == nil
}

func (pm *ProcessManager) SaveState() {
	pm.lock.RLock()
	data := make(map[string]ServiceConfig)
	for name, s := range pm.services {
		cfg := s.Config
		if s.IsRunning() {
			cfg.SavedStatus = "running"
		} else {
			cfg.SavedStatus = "stopped"
		}
		data[name] = cfg
	}
	pm.lock.RUnlock()

	bytes, err := json.MarshalIndent(data, "", "  ")
	if err == nil {
		_ = os.WriteFile(pm.ServicesFile, bytes, 0644)
	}
}

func (pm *ProcessManager) LoadState() {
	pm.lock.Lock()
	defer pm.lock.Unlock()

	data, err := os.ReadFile(pm.ServicesFile)
	if err != nil {
		return
	}

	var configs map[string]ServiceConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		return
	}

	for name, cfg := range configs {
		if _, exists := pm.services[name]; !exists {
			cfg.Group = NormalizeGroup(cfg.Group)
			pm.services[name] = &ManagedService{
				Config: cfg,
				Status: "stopped",
			}
		}
	}
}

func (pm *ProcessManager) Resurrect() {
	pm.LoadState()
	pm.lock.RLock()
	var toStart []string
	for name, s := range pm.services {
		if s.Config.SavedStatus == "running" || s.Config.AutoRestart {
			toStart = append(toStart, name)
		}
	}
	pm.lock.RUnlock()

	for _, name := range toStart {
		pm.StartService(name)
	}
}

func (pm *ProcessManager) GetSummary() map[string]interface{} {
	pm.lock.RLock()
	defer pm.lock.RUnlock()

	var names []string
	for name := range pm.services {
		names = append(names, name)
	}
	sort.Strings(names)

	var items []map[string]interface{}
	runningCount := 0
	var totalMem float64 = 0

	for _, name := range names {
		s := pm.services[name]
		info := s.ToMap()
		items = append(items, info)
		if info["status"] == "running" {
			runningCount++
			if m, ok := info["memory_mb"].(float64); ok {
				totalMem += m
			}
		}
	}

	return map[string]interface{}{
		"admin":             IsAdmin(),
		"services":          items,
		"groups":            pm.getGroupsLocked(),
		"total_count":       len(items),
		"running_count":     runningCount,
		"total_memory_mb":   float64(int(totalMem*10)) / 10.0,
		"total_cpu_percent": 0.0,
	}
}
