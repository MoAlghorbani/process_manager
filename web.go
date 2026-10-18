package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

//go:embed index.html
var htmlContent string

//go:embed game.html
var gameHtmlContent string

//go:embed aim.html
var aimHtmlContent string

//go:embed three.min.js
var threeJsContent string

var (
	webRoutesOnce sync.Once
	webRunning    bool
	webRunningMu  sync.Mutex
)

func StartWebServer(port int, autoOpen bool) error {
	pm := GetManager()

	webRoutesOnce.Do(func() {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" && r.URL.Path != "/index.html" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(htmlContent))
		})

		http.HandleFunc("/three.min.js", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			w.Write([]byte(threeJsContent))
		})

		http.HandleFunc("/game", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(gameHtmlContent))
		})

		http.HandleFunc("/aim", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(aimHtmlContent))
		})

	http.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		summary := pm.GetSummary()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(summary)
	})

	http.HandleFunc("/api/detect_venv", func(w http.ResponseWriter, r *http.Request) {
		cwd := r.URL.Query().Get("cwd")
		detected := DetectVenvInDir(cwd)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"detected":      detected != "",
			"detected_name": detected,
		})
	})

	http.HandleFunc("/api/logs", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		lines, _ := strconv.Atoi(r.URL.Query().Get("lines"))
		if lines <= 0 {
			lines = 150
		}
		logs := pm.GetLogs(name, lines)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"name": name,
			"logs": logs,
		})
	})

	http.HandleFunc("/api/service/start", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Name string `json:"name"` }
		_ = json.NewDecoder(r.Body).Decode(&req)
		success := pm.StartService(req.Name)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": success})
	})

	http.HandleFunc("/api/service/stop", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Name string `json:"name"` }
		_ = json.NewDecoder(r.Body).Decode(&req)
		success := pm.StopService(req.Name)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": success})
	})

	http.HandleFunc("/api/service/restart", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Name string `json:"name"` }
		_ = json.NewDecoder(r.Body).Decode(&req)
		success := pm.RestartService(req.Name)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": success})
	})

	http.HandleFunc("/api/service/delete", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Name string `json:"name"` }
		_ = json.NewDecoder(r.Body).Decode(&req)
		success := pm.DeleteService(req.Name)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": success})
	})

	http.HandleFunc("/api/service/add", func(w http.ResponseWriter, r *http.Request) {
		var req ServiceConfig
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		if req.Name == "" || req.Command == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Service Name and Command are required."})
			return
		}
		pm.AddService(req, true)
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})

	http.HandleFunc("/api/service/update", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			OldName          string `json:"old_name"`
			RestartIfRunning bool   `json:"restart_if_running"`
			ServiceConfig
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		if req.OldName == "" || req.Name == "" || req.Command == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "OldName, Name, and Command are required."})
			return
		}
		success := pm.UpdateService(req.OldName, req.ServiceConfig, req.RestartIfRunning)
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": success})
	})

	http.HandleFunc("/api/start_all", func(w http.ResponseWriter, r *http.Request) {
		pm.StartAll()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})

	http.HandleFunc("/api/stop_all", func(w http.ResponseWriter, r *http.Request) {
		pm.StopAll()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})

	http.HandleFunc("/api/restart_all", func(w http.ResponseWriter, r *http.Request) {
		pm.RestartAll()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})

	http.HandleFunc("/api/group/start", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Group string `json:"group"` }
		_ = json.NewDecoder(r.Body).Decode(&req)
		started := pm.StartGroup(req.Group)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"group":   req.Group,
			"started": started,
		})
	})

	http.HandleFunc("/api/group/stop", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Group string `json:"group"` }
		_ = json.NewDecoder(r.Body).Decode(&req)
		stopped := pm.StopGroup(req.Group)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"group":   req.Group,
			"stopped": stopped,
		})
	})

	http.HandleFunc("/api/group/restart", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Group string `json:"group"` }
		_ = json.NewDecoder(r.Body).Decode(&req)
		restarted := pm.RestartGroup(req.Group)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success":   true,
			"group":     req.Group,
			"restarted": restarted,
		})
	})

	http.HandleFunc("/api/groups", func(w http.ResponseWriter, r *http.Request) {
		groups := pm.GetGroups()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"groups": groups,
		})
	})

	http.HandleFunc("/api/group/rename", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			OldGroup string `json:"old_group"`
			NewGroup string `json:"new_group"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		count := pm.RenameGroup(req.OldGroup, req.NewGroup)
		GetTodoStore().RenameProject(req.OldGroup, req.NewGroup)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"updated": count,
		})
	})

	http.HandleFunc("/api/group/move", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Service string `json:"service"`
			Group   string `json:"group"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		ok := pm.MoveServiceToGroup(req.Service, req.Group)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{
			"success": ok,
		})
	})

	http.HandleFunc("/api/group/delete", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Group    string `json:"group"`
			Fallback string `json:"fallback"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Fallback == "" {
			req.Fallback = "default"
		}
		count := pm.DeleteGroup(req.Group, req.Fallback)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"updated": count,
		})
	})

	http.HandleFunc("/api/save", func(w http.ResponseWriter, r *http.Request) {
		pm.SaveState()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})

	http.HandleFunc("/api/resurrect", func(w http.ResponseWriter, r *http.Request) {
		pm.Resurrect()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})

	http.HandleFunc("/api/logs/clear", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Name string `json:"name"` }
		_ = json.NewDecoder(r.Body).Decode(&req)
		success := pm.ClearLogs(req.Name)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": success})
	})

	http.HandleFunc("/api/elevate", func(w http.ResponseWriter, r *http.Request) {
		go func() {
			time.Sleep(500 * time.Millisecond)
			_ = RequestAdminElevation([]string{"web", fmt.Sprintf("-port=%d", port)})
		}()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})

	http.HandleFunc("/api/todos", func(w http.ResponseWriter, r *http.Request) {
		ts := GetTodoStore()
		filter := r.URL.Query().Get("filter")
		search := r.URL.Query().Get("search")
		items := ts.List(filter, search)
		total, active, completed := ts.Counts()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"items":     items,
			"total":     total,
			"active":    active,
			"completed": completed,
		})
	})

	http.HandleFunc("/api/todo/add", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Title    string     `json:"title"`
			Project  string     `json:"project"`
			Priority string     `json:"priority"`
			Status   string     `json:"status"`
			NotifyAt *time.Time `json:"notify_at"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		item, err := GetTodoStore().AddWithDetails(req.Title, req.Project, req.Priority, req.Status, req.NotifyAt)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(item)
	})

	http.HandleFunc("/api/todo/status", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		item, err := GetTodoStore().SetStatus(req.ID, req.Status)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(item)
	})

	http.HandleFunc("/api/todo/toggle", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID string `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		item, err := GetTodoStore().Toggle(req.ID)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(item)
	})

	http.HandleFunc("/api/todo/delete", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID string `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		ok := GetTodoStore().Delete(req.ID)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": ok})
	})

	http.HandleFunc("/api/todo/clear_completed", func(w http.ResponseWriter, r *http.Request) {
		count := GetTodoStore().ClearCompleted()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int{"cleared": count})
	})
	})

	url := fmt.Sprintf("http://localhost:%d", port)

	webRunningMu.Lock()
	if webRunning {
		webRunningMu.Unlock()
		if autoOpen {
			go func() {
				time.Sleep(300 * time.Millisecond)
				_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
			}()
		}
		return nil
	}
	webRunning = true
	webRunningMu.Unlock()

	fmt.Println("\n========================================================")
	fmt.Printf("  Process Manager (Go Engine) Web UI running at:\n")
	fmt.Printf("     %s\n", url)
	fmt.Println("========================================================\n")

	if autoOpen {
		go func() {
			time.Sleep(800 * time.Millisecond)
			_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
		}()
	}

	return http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}
