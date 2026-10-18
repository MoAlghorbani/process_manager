package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

func formatTable(services []map[string]interface{}, isAdmin bool) {
	fmt.Println("\n" + strings.Repeat("=", 112))
	adminStr := "[ADMIN: ELEVATED]"
	if !isAdmin {
		adminStr = "[USER: STANDARD]"
	}
	fmt.Printf("  PROCESS MANAGER (GO ENGINE) - Active Services %s\n", adminStr)
	fmt.Println(strings.Repeat("=", 112))

	if len(services) == 0 {
		fmt.Println("  No services registered. Use 'pm start <name> --cmd ...' or 'pm web'")
		fmt.Println(strings.Repeat("=", 112) + "\n")
		return
	}

	header := fmt.Sprintf("%-15s %-16s %-12s %-7s %-10s %-8s %-11s %-9s %-9s %-9s",
		"NAME", "GROUP", "SHORTCUT", "PORT", "VENV", "PID", "STATUS", "RESTARTS", "RAM", "UPTIME")
	fmt.Println(header)
	fmt.Println(strings.Repeat("-", 126))

	for _, s := range services {
		name := fmt.Sprintf("%v", s["name"])
		if len(name) > 14 {
			name = name[:14]
		}

		groupStr := "default"
		if g, ok := s["group"].(string); ok && g != "" {
			groupStr = g
		}
		if len(groupStr) > 15 {
			groupStr = groupStr[:15]
		}

		shortcutStr := "-"
		if sc, ok := s["shortcut"].(string); ok && sc != "" {
			shortcutStr = sc
		}
		if len(shortcutStr) > 12 {
			shortcutStr = shortcutStr[:12]
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

		venvStr := "-"
		if v, ok := s["venv"].(string); ok && v != "" {
			parts := strings.Split(strings.ReplaceAll(v, "\\", "/"), "/")
			venvStr = parts[len(parts)-1]
			if len(venvStr) > 9 {
				venvStr = venvStr[:9]
			}
		}

		pidStr := "-"
		if pid, ok := s["pid"].(int); ok && pid > 0 {
			pidStr = strconv.Itoa(pid)
		}

		status := strings.ToUpper(fmt.Sprintf("%v", s["status"]))
		restarts := fmt.Sprintf("%v", s["restarts"])
		ramStr := "-"
		if mem, ok := s["memory_mb"].(float64); ok && mem > 0 {
			ramStr = fmt.Sprintf("%.1fMB", mem)
		}

		uptimeStr := "-"
		if up, ok := s["uptime_seconds"].(int64); ok && up > 0 {
			if up < 60 {
				uptimeStr = fmt.Sprintf("%ds", up)
			} else if up < 3600 {
				uptimeStr = fmt.Sprintf("%dm", up/60)
			} else {
				uptimeStr = fmt.Sprintf("%dh", up/3600)
			}
		}

		fmt.Printf("%-15s %-16s %-12s %-7s %-10s %-8s %-11s %-9s %-9s %-9s\n",
			name, groupStr, shortcutStr, portStr, venvStr, pidStr, status, restarts, ramStr, uptimeStr)
	}

	fmt.Println(strings.Repeat("=", 126) + "\n")
}

func main() {
	if len(os.Args) < 2 {
		pm := GetManager()
		summary := pm.GetSummary()
		services, _ := summary["services"].([]map[string]interface{})
		isAdmin, _ := summary["admin"].(bool)
		formatTable(services, isAdmin)
		fmt.Println("Tip: Run 'pm web' to launch the browser control center, or 'pm --help' for commands.")
		return
	}

	cmd := os.Args[1]
	pm := GetManager()

	switch cmd {
	case "list":
		listCmd := flag.NewFlagSet("list", flag.ExitOnError)
		groupFilter := listCmd.String("group", "", "Filter services by group")
		_ = listCmd.Parse(os.Args[2:])

		summary := pm.GetSummary()
		services, _ := summary["services"].([]map[string]interface{})
		isAdmin, _ := summary["admin"].(bool)

		if *groupFilter != "" {
			var filtered []map[string]interface{}
			for _, s := range services {
				g, _ := s["group"].(string)
				if MatchGroup(g, *groupFilter) {
					filtered = append(filtered, s)
				}
			}
			services = filtered
		}
		formatTable(services, isAdmin)

	case "groups":
		summary := pm.GetSummary()
		services, _ := summary["services"].([]map[string]interface{})
		groups := pm.GetGroups()

		fmt.Println("\n" + strings.Repeat("=", 82))
		fmt.Printf("  REGISTERED SERVICE GROUPS (%d groups total)\n", len(groups))
		fmt.Println(strings.Repeat("=", 82))
		fmt.Printf("%-20s %-8s %-9s %s\n", "GROUP", "TOTAL", "RUNNING", "SERVICES")
		fmt.Println(strings.Repeat("-", 82))

		for _, grp := range groups {
			total := 0
			running := 0
			var svcNames []string
			for _, s := range services {
				g, _ := s["group"].(string)
				if MatchGroup(g, grp) {
					total++
					name, _ := s["name"].(string)
					svcNames = append(svcNames, name)
					if s["status"] == "running" {
						running++
					}
				}
			}
			namesStr := strings.Join(svcNames, ", ")
			if len(namesStr) > 42 {
				namesStr = namesStr[:39] + "..."
			}
			fmt.Printf("%-20s %-8d %-9d %s\n", grp, total, running, namesStr)
		}
		fmt.Println(strings.Repeat("=", 82) + "\n")

	case "start-group":
		if len(os.Args) < 3 {
			fmt.Println("Usage: pm start-group <group>")
			os.Exit(1)
		}
		group := os.Args[2]
		started := pm.StartGroup(group)
		if len(started) > 0 {
			fmt.Printf("[OK] Started %d service(s) in group '%s': %s\n", len(started), group, strings.Join(started, ", "))
		} else {
			fmt.Printf("[INFO] No stopped services found in group '%s'.\n", group)
		}

	case "stop-group":
		if len(os.Args) < 3 {
			fmt.Println("Usage: pm stop-group <group>")
			os.Exit(1)
		}
		group := os.Args[2]
		stopped := pm.StopGroup(group)
		if len(stopped) > 0 {
			fmt.Printf("[OK] Stopped %d service(s) in group '%s': %s\n", len(stopped), group, strings.Join(stopped, ", "))
		} else {
			fmt.Printf("[INFO] No running services found in group '%s'.\n", group)
		}

	case "restart-group":
		if len(os.Args) < 3 {
			fmt.Println("Usage: pm restart-group <group>")
			os.Exit(1)
		}
		group := os.Args[2]
		restarted := pm.RestartGroup(group)
		if len(restarted) > 0 {
			fmt.Printf("[OK] Restarted %d service(s) in group '%s': %s\n", len(restarted), group, strings.Join(restarted, ", "))
		} else {
			fmt.Printf("[INFO] No services found in group '%s'.\n", group)
		}

	case "group":
		if len(os.Args) < 3 {
			fmt.Println("Usage: pm group <list|start|stop|restart> [group_name]")
			os.Exit(1)
		}
		subCmd := os.Args[2]
		switch subCmd {
		case "list":
			summary := pm.GetSummary()
			services, _ := summary["services"].([]map[string]interface{})
			groups := pm.GetGroups()

			fmt.Println("\n" + strings.Repeat("=", 82))
			fmt.Printf("  REGISTERED SERVICE GROUPS (%d groups total)\n", len(groups))
			fmt.Println(strings.Repeat("=", 82))
			fmt.Printf("%-20s %-8s %-9s %s\n", "GROUP", "TOTAL", "RUNNING", "SERVICES")
			fmt.Println(strings.Repeat("-", 82))

			for _, grp := range groups {
				total := 0
				running := 0
				var svcNames []string
				for _, s := range services {
					g, _ := s["group"].(string)
					if MatchGroup(g, grp) {
						total++
						name, _ := s["name"].(string)
						svcNames = append(svcNames, name)
						if s["status"] == "running" {
							running++
						}
					}
				}
				namesStr := strings.Join(svcNames, ", ")
				if len(namesStr) > 42 {
					namesStr = namesStr[:39] + "..."
				}
				fmt.Printf("%-20s %-8d %-9d %s\n", grp, total, running, namesStr)
			}
			fmt.Println(strings.Repeat("=", 82) + "\n")

		case "start":
			if len(os.Args) < 4 {
				fmt.Println("Usage: pm group start <group_name>")
				os.Exit(1)
			}
			group := os.Args[3]
			started := pm.StartGroup(group)
			fmt.Printf("[OK] Started %d service(s) in group '%s': %s\n", len(started), group, strings.Join(started, ", "))

		case "stop":
			if len(os.Args) < 4 {
				fmt.Println("Usage: pm group stop <group_name>")
				os.Exit(1)
			}
			group := os.Args[3]
			stopped := pm.StopGroup(group)
			fmt.Printf("[OK] Stopped %d service(s) in group '%s': %s\n", len(stopped), group, strings.Join(stopped, ", "))

		case "restart":
			if len(os.Args) < 4 {
				fmt.Println("Usage: pm group restart <group_name>")
				os.Exit(1)
			}
			group := os.Args[3]
			restarted := pm.RestartGroup(group)
			fmt.Printf("[OK] Restarted %d service(s) in group '%s': %s\n", len(restarted), group, strings.Join(restarted, ", "))

		case "rename":
			if len(os.Args) < 5 {
				fmt.Println("Usage: pm group rename <old_group> <new_group>")
				os.Exit(1)
			}
			oldG := os.Args[3]
			newG := os.Args[4]
			count := pm.RenameGroup(oldG, newG)
			GetTodoStore().RenameProject(oldG, newG)
			fmt.Printf("[OK] Renamed group '%s' to '%s' (%d services updated).\n", oldG, newG, count)

		case "move":
			if len(os.Args) < 5 {
				fmt.Println("Usage: pm group move <service_name> <target_group>")
				os.Exit(1)
			}
			sName := os.Args[3]
			targetG := os.Args[4]
			if pm.MoveServiceToGroup(sName, targetG) {
				fmt.Printf("[OK] Moved service '%s' to group '%s'.\n", sName, targetG)
			} else {
				fmt.Printf("[ERROR] Service '%s' not found.\n", sName)
			}

		case "delete", "ungroup":
			if len(os.Args) < 4 {
				fmt.Println("Usage: pm group delete <group_name>")
				os.Exit(1)
			}
			grp := os.Args[3]
			count := pm.DeleteGroup(grp, "default")
			fmt.Printf("[OK] Dissolved group '%s' (%d services moved to 'default').\n", grp, count)

		default:
			fmt.Printf("Unknown group action '%s'. Available: list, start, stop, restart, rename, move, delete\n", subCmd)
		}

	case "gui":
		guiCmd := flag.NewFlagSet("gui", flag.ExitOnError)
		port := guiCmd.Int("port", 5055, "Backend port")
		_ = guiCmd.Parse(os.Args[2:])

		kernel32 := syscall.NewLazyDLL("kernel32.dll")
		user32 := syscall.NewLazyDLL("user32.dll")
		procCreateMutexW := kernel32.NewProc("CreateMutexW")
		procFindWindowW := user32.NewProc("FindWindowW")
		procShowWindow := user32.NewProc("ShowWindow")
		procSetForegroundWindow := user32.NewProc("SetForegroundWindow")

		mutexName, _ := syscall.UTF16PtrFromString("Local\\ProcessManager_NativeGUI_SingleInstance")
		hMutex, _, callErr := procCreateMutexW.Call(0, 1, uintptr(unsafe.Pointer(mutexName)))
		if errno, ok := callErr.(syscall.Errno); ok && errno == 183 {
			const winTitle = "Windows Process Manager & Service Supervisor (Native Go)"
			titleUTF16, _ := syscall.UTF16PtrFromString(winTitle)
			var hwnd uintptr
			for i := 0; i < 4; i++ {
				hwnd, _, _ = procFindWindowW.Call(0, uintptr(unsafe.Pointer(titleUTF16)))
				if hwnd != 0 {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			if hwnd != 0 {
				const swRestore = 9
				procShowWindow.Call(hwnd, uintptr(swRestore))
				procSetForegroundWindow.Call(hwnd)
				fmt.Println("[Process Manager] An active GUI instance is already running. Switched to active window.")
				time.Sleep(1200 * time.Millisecond)
				os.Exit(0)
			} else {
				// If no window found, the mutex was held by a headless or zombie instance.
				// Terminate stale background instances so the user can open their GUI.
				fmt.Println("[Process Manager] Cleaning up stale background instance...")
				_ = exec.Command("taskkill", "/F", "/IM", "pm.exe", "/FI", "PID ne "+strconv.Itoa(os.Getpid())).Run()
				time.Sleep(400 * time.Millisecond)
			}
		}
		_ = hMutex

		if err := StartDesktopGUI(*port); err != nil {
			fmt.Printf("Desktop GUI error: %v\n", err)
			fmt.Println("Falling back to web browser...")
			_ = StartWebServer(*port, true)
		}

	case "web":
		webCmd := flag.NewFlagSet("web", flag.ExitOnError)
		port := webCmd.Int("port", 5055, "Web server port")
		noBrowser := webCmd.Bool("no-browser", false, "Do not auto open browser")
		_ = webCmd.Parse(os.Args[2:])

		if err := StartWebServer(*port, !*noBrowser); err != nil {
			fmt.Printf("Web server error: %v\n", err)
			os.Exit(1)
		}

	case "start":
		startCmd := flag.NewFlagSet("start", flag.ExitOnError)
		execCmd := startCmd.String("cmd", "", "Command to execute")
		cwd := startCmd.String("cwd", "", "Working directory")
		group := startCmd.String("group", "default", "Group name for service")
		port := startCmd.Int("port", 0, "Port for service")
		venv := startCmd.String("venv", "", "Virtual environment path")
		shortcut := startCmd.String("shortcut", "", "Keyboard shortcut (e.g. Ctrl+1)")
		noRestart := startCmd.Bool("no-restart", false, "Disable auto-restart")

		var name string
		var flagArgs []string
		argsList := os.Args[2:]
		if len(argsList) > 0 && !strings.HasPrefix(argsList[0], "-") {
			name = argsList[0]
			flagArgs = argsList[1:]
		} else {
			flagArgs = argsList
		}
		_ = startCmd.Parse(flagArgs)
		if name == "" {
			name = startCmd.Arg(0)
		}

		if name == "" {
			fmt.Println("Usage: pm start <name> [--cmd <command>] [--group <group>] [--cwd <cwd>] [--port <port>] [--venv <venv>] [--shortcut <shortcut>]")
			os.Exit(1)
		}

		if *execCmd != "" {
			var p *int
			if *port > 0 {
				p = port
			}
			cfg := ServiceConfig{
				Name:        name,
				Group:       *group,
				Command:     *execCmd,
				Cwd:         *cwd,
				Port:        p,
				Venv:        *venv,
				AutoRestart: !*noRestart,
				Shortcut:    *shortcut,
			}
			pm.AddService(cfg, true)
			fmt.Printf("[OK] Registered and started '%s' (Group: %s).\n", name, NormalizeGroup(*group))
		} else {
			if name == "all" {
				pm.StartAll()
				fmt.Println("[OK] Started all services.")
			} else if pm.StartService(name) {
				fmt.Printf("[OK] Started service '%s'.\n", name)
			} else {
				fmt.Printf("[ERROR] Service '%s' not found. Specify --cmd to register it.\n", name)
			}
		}

	case "edit":
		editCmd := flag.NewFlagSet("edit", flag.ExitOnError)
		newName := editCmd.String("new-name", "", "New name if renaming")
		group := editCmd.String("group", "", "New group name")
		execCmd := editCmd.String("cmd", "", "New command")
		cwd := editCmd.String("cwd", "", "New working directory")
		port := editCmd.Int("port", -1, "New port number")
		venv := editCmd.String("venv", "", "Virtual environment")
		shortcut := editCmd.String("shortcut", "", "New keyboard shortcut (e.g. Ctrl+1, or none)")
		noRestart := editCmd.Bool("no-restart", false, "Do not restart if running")

		var name string
		var flagArgs []string
		argsList := os.Args[2:]
		if len(argsList) > 0 && !strings.HasPrefix(argsList[0], "-") {
			name = argsList[0]
			flagArgs = argsList[1:]
		} else {
			flagArgs = argsList
		}
		_ = editCmd.Parse(flagArgs)
		if name == "" {
			name = editCmd.Arg(0)
		}

		if name == "" {
			fmt.Println("Usage: pm edit <name> [--new-name <name>] [--group <group>] [--cmd <command>] [--port <port>] [--venv <venv>] [--shortcut <shortcut>]")
			os.Exit(1)
		}

		pm.lock.RLock()
		existing, ok := pm.services[name]
		pm.lock.RUnlock()
		if !ok {
			fmt.Printf("[ERROR] Service '%s' not found.\n", name)
			os.Exit(1)
		}

		cfg := existing.Config
		if *newName != "" {
			cfg.Name = *newName
		}
		if *group != "" {
			cfg.Group = *group
		}
		if *execCmd != "" {
			cfg.Command = *execCmd
		}
		if *cwd != "" {
			cfg.Cwd = *cwd
		}
		if *port >= 0 {
			if *port == 0 {
				cfg.Port = nil
			} else {
				cfg.Port = port
			}
		}
		if *venv != "" {
			cfg.Venv = *venv
		}
		if *shortcut != "" {
			if strings.EqualFold(*shortcut, "none") || strings.EqualFold(*shortcut, "clear") {
				cfg.Shortcut = ""
			} else {
				cfg.Shortcut = *shortcut
			}
		}

		if pm.UpdateService(name, cfg, !*noRestart) {
			fmt.Printf("[OK] Successfully updated service '%s'.\n", cfg.Name)
		} else {
			fmt.Printf("[ERROR] Failed to update service '%s'.\n", name)
		}

	case "stop":
		if len(os.Args) < 3 {
			fmt.Println("Usage: pm stop <name|all>")
			os.Exit(1)
		}
		name := os.Args[2]
		if name == "all" {
			pm.StopAll()
			fmt.Println("[OK] Stopped all services.")
		} else if pm.StopService(name) {
			fmt.Printf("[OK] Stopped service '%s'.\n", name)
		} else {
			fmt.Printf("[ERROR] Service '%s' not found.\n", name)
		}

	case "restart":
		if len(os.Args) < 3 {
			fmt.Println("Usage: pm restart <name|all>")
			os.Exit(1)
		}
		name := os.Args[2]
		if name == "all" {
			pm.RestartAll()
			fmt.Println("[OK] Restarted all services.")
		} else if pm.RestartService(name) {
			fmt.Printf("[OK] Restarted service '%s'.\n", name)
		} else {
			fmt.Printf("[ERROR] Service '%s' not found.\n", name)
		}

	case "delete":
		if len(os.Args) < 3 {
			fmt.Println("Usage: pm delete <name>")
			os.Exit(1)
		}
		name := os.Args[2]
		if pm.DeleteService(name) {
			fmt.Printf("[OK] Deleted service '%s'.\n", name)
		} else {
			fmt.Printf("[ERROR] Service '%s' not found.\n", name)
		}

	case "save":
		pm.SaveState()
		fmt.Printf("[OK] Saved state to %s\n", pm.ServicesFile)

	case "resurrect", "restore":
		pm.Resurrect()
		fmt.Printf("[OK] Resurrected all services from %s\n", pm.ServicesFile)
		time.Sleep(1 * time.Second)
		summary := pm.GetSummary()
		services, _ := summary["services"].([]map[string]interface{})
		isAdmin, _ := summary["admin"].(bool)
		formatTable(services, isAdmin)

	case "logs":
		if len(os.Args) < 3 {
			fmt.Println("Usage: pm logs <name> [-n <lines>]")
			os.Exit(1)
		}
		name := os.Args[2]
		lines := 50
		if len(os.Args) >= 5 && os.Args[3] == "-n" {
			if l, err := strconv.Atoi(os.Args[4]); err == nil {
				lines = l
			}
		}
		logs := pm.GetLogs(name, lines)
		fmt.Printf("\n--- Logs for '%s' (Last %d lines) ---\n", name, lines)
		fmt.Println(logs)
		fmt.Println("--- End of logs ---\n")

	case "elevate":
		if IsAdmin() {
			fmt.Println("Already running with Administrator privileges.")
		} else {
			fmt.Println("Requesting Windows Administrator elevation...")
			if err := RequestAdminElevation([]string{"web"}); err != nil {
				fmt.Printf("Elevation failed: %v\n", err)
			}
		}

	case "creds":
		cs := GetCredentialStore()
		search := ""
		if len(os.Args) >= 3 {
			search = os.Args[2]
		}
		items := cs.List(search)
		fmt.Println("\n" + strings.Repeat("=", 80))
		fmt.Printf("  CREDENTIALS VAULT (%d accounts)\n", len(items))
		fmt.Println(strings.Repeat("=", 80))
		fmt.Printf("%-20s %-25s %-20s %s\n", "PLATFORM", "USERNAME", "PASSWORD", "URL")
		fmt.Println(strings.Repeat("-", 80))
		for _, it := range items {
			url := it.URL
			if len(url) > 20 {
				url = url[:17] + "..."
			}
			masked := strings.Repeat("•", 10)
			fmt.Printf("%-20s %-25s %-20s %s\n", it.Platform, it.Username, masked, url)
		}
		fmt.Println(strings.Repeat("=", 80) + "\n")

	case "clips":
		cw := GetClipboardWatcher()
		search := ""
		if len(os.Args) >= 3 {
			search = os.Args[2]
		}
		items := cw.List(search, false)
		limit := 15
		if len(items) < limit {
			limit = len(items)
		}
		fmt.Println("\n" + strings.Repeat("=", 80))
		fmt.Printf("  PERMANENT CLIPBOARD HISTORY (Showing latest %d of %d clips)\n", limit, len(items))
		fmt.Println(strings.Repeat("=", 80))
		for i := 0; i < limit; i++ {
			it := items[i]
			pin := " "
			if it.Pinned {
				pin = "⭐"
			}
			preview := strings.ReplaceAll(it.Content, "\n", " ")
			if len(preview) > 55 {
				preview = preview[:52] + "..."
			}
			fmt.Printf(" %s [%s] (%4d chars) : %s\n", pin, it.Timestamp.Format("15:04:05"), it.Length, preview)
		}
		fmt.Println(strings.Repeat("=", 80) + "\n")

	case "todo", "todos", "task", "tasks":
		ts := GetTodoStore()
		if len(os.Args) < 3 || os.Args[2] == "list" {
			filter := "all"
			search := ""
			for i := 2; i < len(os.Args); i++ {
				arg := os.Args[i]
				if arg == "list" {
					continue
				}
				if arg == "--active" || arg == "-a" {
					filter = "active"
				} else if arg == "--completed" || arg == "-c" {
					filter = "completed"
				} else if arg == "--todo" {
					filter = StatusTodo
				} else if arg == "--doing" || arg == "--in-progress" {
					filter = StatusInProgress
				} else if !strings.HasPrefix(arg, "-") {
					search = arg
				}
			}
			items := ts.List(filter, search)
			todoC, inProgC, doneC := ts.ColumnCounts()
			total := todoC + inProgC + doneC
			fmt.Println("\n" + strings.Repeat("=", 88))
			fmt.Printf("  KANBAN BOARD & TASKS (%d To Do • %d In Progress • %d Done • %d Total)\n", todoC, inProgC, doneC, total)
			fmt.Println(strings.Repeat("=", 88))
			if len(items) == 0 {
				fmt.Println("  No tasks found. Use 'pm todo add <task>' to create one.")
			} else {
				fmt.Printf("%-8s %-14s %-12s %-8s %-36s %s\n", "ID", "PROJECT", "COLUMN", "PRIORITY", "TASK", "CREATED")
				fmt.Println(strings.Repeat("-", 94))
				for _, it := range items {
					colTag := "[📋 TO DO]"
					switch it.Status {
					case StatusInProgress:
						colTag = "[🚧 DOING]"
					case StatusDone:
						colTag = "[✅ DONE ]"
					}
					pri := strings.ToUpper(it.Priority)
					if pri == "" {
						pri = "NORMAL"
					}
					title := it.Title
					if len(title) > 34 {
						title = title[:31] + "..."
					}
					proj := it.Project
					if len(proj) > 13 {
						proj = proj[:13]
					}
					fmt.Printf("#%-7s %-14s %-12s %-8s %-36s %s\n", it.ID, proj, colTag, pri, title, it.CreatedAt.Format("01/02 15:04"))
				}
			}
			fmt.Println(strings.Repeat("=", 94) + "\n")

		} else {
			sub := os.Args[2]
			switch sub {
			case "add":
				if len(os.Args) < 4 {
					fmt.Println("Usage: pm todo add <title> [--project <name>] [--high|--low] [--doing|--done]")
					os.Exit(1)
				}
				priority := "normal"
				status := StatusTodo
				project := "General"
				var titleParts []string
				args := os.Args[3:]
				for i := 0; i < len(args); i++ {
					a := args[i]
					if (a == "--project" || a == "-p") && i+1 < len(args) {
						project = args[i+1]
						i++
					} else if a == "--high" || a == "-h" {
						priority = "high"
					} else if a == "--low" || a == "-l" {
						priority = "low"
					} else if a == "--doing" || a == "--in-progress" {
						status = StatusInProgress
					} else if a == "--done" {
						status = StatusDone
					} else {
						titleParts = append(titleParts, a)
					}
				}
				title := strings.Join(titleParts, " ")
				item, err := ts.AddWithDetails(title, project, priority, status, nil)
				if err != nil {
					fmt.Printf("[ERROR] %v\n", err)
				} else {
					fmt.Printf("[OK] Added task #%s to [%s] (Project: %s): %s [%s]\n", item.ID, strings.ToUpper(item.Status), item.Project, item.Title, strings.ToUpper(item.Priority))
				}

			case "start", "doing":
				if len(os.Args) < 4 {
					fmt.Println("Usage: pm todo start <id>")
					os.Exit(1)
				}
				id := os.Args[3]
				item, err := ts.SetStatus(id, StatusInProgress)
				if err != nil {
					fmt.Printf("[ERROR] %v\n", err)
				} else {
					fmt.Printf("[OK] Task #%s is now IN PROGRESS: %s\n", item.ID, item.Title)
				}

			case "done", "finish":
				if len(os.Args) < 4 {
					fmt.Println("Usage: pm todo done <id>")
					os.Exit(1)
				}
				id := os.Args[3]
				item, err := ts.SetStatus(id, StatusDone)
				if err != nil {
					fmt.Printf("[ERROR] %v\n", err)
				} else {
					fmt.Printf("[OK] Task #%s is now DONE: %s\n", item.ID, item.Title)
				}

			case "backlog", "todo":
				if len(os.Args) < 4 {
					fmt.Println("Usage: pm todo todo <id>")
					os.Exit(1)
				}
				id := os.Args[3]
				item, err := ts.SetStatus(id, StatusTodo)
				if err != nil {
					fmt.Printf("[ERROR] %v\n", err)
				} else {
					fmt.Printf("[OK] Task #%s moved to TO DO: %s\n", item.ID, item.Title)
				}

			case "toggle", "check":
				if len(os.Args) < 4 {
					fmt.Println("Usage: pm todo toggle <id>")
					os.Exit(1)
				}
				id := os.Args[3]
				item, err := ts.Toggle(id)
				if err != nil {
					fmt.Printf("[ERROR] %v\n", err)
				} else {
					fmt.Printf("[OK] Task #%s is now %s: %s\n", item.ID, strings.ToUpper(item.Status), item.Title)
				}

			case "rm", "delete", "del":
				if len(os.Args) < 4 {
					fmt.Println("Usage: pm todo delete <id>")
					os.Exit(1)
				}
				id := os.Args[3]
				if ts.Delete(id) {
					fmt.Printf("[OK] Deleted task '%s'.\n", id)
				} else {
					fmt.Printf("[ERROR] Task '%s' not found.\n", id)
				}

			case "clear":
				count := ts.ClearCompleted()
				fmt.Printf("[OK] Cleared %d completed tasks.\n", count)

			default:
				// Treat "pm todo <title>" as adding a task directly
				title := strings.Join(os.Args[2:], " ")
				item, err := ts.Add(title, "normal")
				if err != nil {
					fmt.Printf("[ERROR] %v\n", err)
				} else {
					fmt.Printf("[OK] Added task #%s: %s\n", item.ID, item.Title)
				}
			}
		}

	case "ytdl", "download", "yt":
		if len(os.Args) < 3 || os.Args[2] == "--help" || os.Args[2] == "-h" {
			fmt.Println("Usage: pm ytdl <url> [--audio|--video] [--format <mp4|mkv|webm|mp3|m4a|flac|wav|opus>] [--quality <best|4k|1080p|720p|480p|360p|320k|192k|128k>] [--dir <output_dir>] [--playlist|--no-playlist]")
			os.Exit(0)
		}
		rawUrl := os.Args[2]
		mediaType := "video"
		containerFmt := "mp4"
		quality := "best"
		outputDir := ""
		isPlaylist := IsPlaylistURL(rawUrl)

		for i := 3; i < len(os.Args); i++ {
			arg := os.Args[i]
			if arg == "--audio" || arg == "-a" {
				mediaType = "audio"
				if containerFmt == "mp4" {
					containerFmt = "mp3"
				}
				if quality == "best" {
					quality = "320k"
				}
			} else if arg == "--video" || arg == "-v" {
				mediaType = "video"
				if containerFmt == "mp3" {
					containerFmt = "mp4"
				}
			} else if arg == "--playlist" || arg == "--yes-playlist" || arg == "-p" {
				isPlaylist = true
			} else if arg == "--no-playlist" {
				isPlaylist = false
			} else if (arg == "--format" || arg == "-f") && i+1 < len(os.Args) {
				fStr := strings.ToLower(os.Args[i+1])
				i++
				switch fStr {
				case "mp3", "m4a", "flac", "wav", "opus", "ogg":
					mediaType = "audio"
					containerFmt = fStr
				case "mp4", "mkv", "webm", "mov", "avi":
					mediaType = "video"
					containerFmt = fStr
				case "1080p", "1080":
					mediaType = "video"
					quality = "1080p"
				case "720p", "720":
					mediaType = "video"
					quality = "720p"
				case "480p", "480":
					mediaType = "video"
					quality = "480p"
				case "4k", "2160p", "2160":
					mediaType = "video"
					quality = "2160p"
				case "2k", "1440p", "1440":
					mediaType = "video"
					quality = "1440p"
				default:
					containerFmt = fStr
				}
			} else if (arg == "--quality" || arg == "-q") && i+1 < len(os.Args) {
				quality = strings.ToLower(os.Args[i+1])
				i++
			} else if (arg == "--dir" || arg == "-d") && i+1 < len(os.Args) {
				outputDir = os.Args[i+1]
				i++
			}
		}

		engine := GetYtDlpEngine()
		if outputDir == "" {
			outputDir = engine.GetConfig().OutputDir
		}

		displayLabel := FormatDisplayLabel(mediaType, containerFmt, quality)
		fmt.Printf("\n[Downloader] Target: %s\n", rawUrl)
		if isPlaylist {
			fmt.Printf("[Downloader] Mode: %s (Playlist) | Save To: %s\n\n", displayLabel, outputDir)
		} else {
			fmt.Printf("[Downloader] Mode: %s | Save To: %s\n\n", displayLabel, outputDir)
		}

		done := make(chan bool, 1)
		_, err := engine.StartDownloadWithOptions(rawUrl, "", mediaType, containerFmt, quality, outputDir, isPlaylist, func(item *DownloadItem) {
			if item.Status == "downloading" {
				if item.IsPlaylist && item.TotalItems > 0 {
					fmt.Printf("[Downloading] Item %d/%d | %5.1f%% | Speed: %-10s | ETA: %-8s | Size: %-12s\n", item.CurrentItem, item.TotalItems, item.Progress, item.Speed, item.ETA, item.TotalSize)
				} else {
					fmt.Printf("[Downloading] %5.1f%% | Speed: %-10s | ETA: %-8s | Size: %-12s\n", item.Progress, item.Speed, item.ETA, item.TotalSize)
				}
			} else if item.Status == "completed" {
				if item.IsPlaylist {
					fmt.Printf("\n\n[OK] Playlist Download Complete: %s\n", item.FilePath)
					if item.FileSize != "" {
						fmt.Printf("     Total size: %s\n", item.FileSize)
					}
				} else {
					fmt.Printf("\n\n[OK] Download Complete: %s\n", item.FilePath)
					if item.FileSize != "" {
						fmt.Printf("     File size: %s\n", item.FileSize)
					}
				}
				done <- true
			} else if item.Status == "error" || item.Status == "cancelled" {
				fmt.Printf("\n\n[ERROR] %s: %s\n", strings.ToUpper(item.Status), item.Error)
				done <- true
			}
		})
		if err != nil {
			fmt.Printf("[ERROR] %v\n", err)
			os.Exit(1)
		}
		<-done

	default:
		fmt.Printf("Unknown command '%s'. Available: list, groups, start-group, stop-group, restart-group, start, edit, stop, restart, delete, save, resurrect, logs, creds, clips, todo, ytdl, web, gui, elevate\n", cmd)
		os.Exit(1)
	}
}

