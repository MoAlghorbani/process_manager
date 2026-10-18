package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Kanban column constants
const (
	StatusTodo       = "todo"
	StatusInProgress = "in_progress"
	StatusDone       = "done"
)

// TodoItem represents a task in the process manager.
type TodoItem struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Project     string     `json:"project,omitempty"` // e.g. "AI SDKs", "OCR Services", "General"
	Status      string     `json:"status"`            // "todo", "in_progress", "done"
	Completed   bool       `json:"completed"`
	Priority    string     `json:"priority,omitempty"` // "high", "normal", "low"
	NotifyAt    *time.Time `json:"notify_at,omitempty"` // reminder / due timestamp
	Notified    bool       `json:"notified,omitempty"`  // whether desktop notification has fired
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// TodoStore provides thread-safe persistent CRUD operations for todos.
type TodoStore struct {
	filePath string
	items    map[string]TodoItem
	lock     sync.RWMutex
}

var (
	globalTodoStore     *TodoStore
	globalTodoStoreOnce sync.Once
)

// GetTodoStore returns the singleton instance of TodoStore.
func GetTodoStore() *TodoStore {
	globalTodoStoreOnce.Do(func() {
		baseDir := GetManager().BaseDir
		storePath := filepath.Join(baseDir, "todos.json")
		globalTodoStore = NewTodoStore(storePath)
	})
	return globalTodoStore
}

// NewTodoStore initializes and loads a TodoStore from disk.
func NewTodoStore(filePath string) *TodoStore {
	ts := &TodoStore{
		filePath: filePath,
		items:    make(map[string]TodoItem),
	}
	ts.Load()
	return ts
}

func generateTodoID() string {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano()%0xFFFFFF)
	}
	return hex.EncodeToString(b)
}

func normalizeStatus(s string, completed bool) string {
	clean := strings.ToLower(strings.TrimSpace(s))
	switch clean {
	case StatusTodo, "to_do", "backlog", "pending":
		return StatusTodo
	case StatusInProgress, "in-progress", "doing", "progress":
		return StatusInProgress
	case StatusDone, "completed", "finish", "finished":
		return StatusDone
	default:
		if completed {
			return StatusDone
		}
		return StatusTodo
	}
}

func normalizeProject(p string) string {
	clean := strings.TrimSpace(p)
	if clean == "" {
		return "General"
	}
	return clean
}

// Load reads tasks from todos.json and normalizes fields.
func (ts *TodoStore) Load() {
	ts.lock.Lock()
	defer ts.lock.Unlock()

	data, err := os.ReadFile(ts.filePath)
	if err != nil {
		return
	}

	var list []TodoItem
	if err := json.Unmarshal(data, &list); err != nil {
		return
	}

	ts.items = make(map[string]TodoItem)
	for _, item := range list {
		if item.ID != "" {
			item.Status = normalizeStatus(item.Status, item.Completed)
			item.Completed = (item.Status == StatusDone)
			item.Project = normalizeProject(item.Project)
			ts.items[item.ID] = item
		}
	}
}

// Save writes tasks to todos.json sorted by status and priority.
func (ts *TodoStore) Save() error {
	ts.lock.RLock()
	list := make([]TodoItem, 0, len(ts.items))
	for _, item := range ts.items {
		list = append(list, item)
	}
	ts.lock.RUnlock()

	priorityVal := func(p string) int {
		switch strings.ToLower(p) {
		case "high":
			return 3
		case "normal", "":
			return 2
		case "low":
			return 1
		default:
			return 2
		}
	}

	statusOrder := func(s string) int {
		switch s {
		case StatusInProgress:
			return 1
		case StatusTodo:
			return 2
		case StatusDone:
			return 3
		default:
			return 2
		}
	}

	sort.Slice(list, func(i, j int) bool {
		oI := statusOrder(list[i].Status)
		oJ := statusOrder(list[j].Status)
		if oI != oJ {
			return oI < oJ
		}
		if list[i].Status != StatusDone {
			pI := priorityVal(list[i].Priority)
			pJ := priorityVal(list[j].Priority)
			if pI != pJ {
				return pI > pJ
			}
			return list[i].CreatedAt.After(list[j].CreatedAt)
		}
		if list[i].CompletedAt != nil && list[j].CompletedAt != nil {
			return list[i].CompletedAt.After(*list[j].CompletedAt)
		}
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ts.filePath, data, 0644)
}

// GetProjects returns a sorted unique list of project names from registered services and todos.
func (ts *TodoStore) GetProjects() []string {
	ts.lock.RLock()
	projMap := make(map[string]bool)
	projMap["General"] = true

	// Add projects from current tasks
	for _, it := range ts.items {
		if it.Project != "" {
			projMap[it.Project] = true
		}
	}
	ts.lock.RUnlock()

	// Add groups from process manager
	for _, g := range GetManager().GetGroups() {
		if g != "" {
			projMap[g] = true
		}
	}

	var projects []string
	for p := range projMap {
		projects = append(projects, p)
	}
	sort.Strings(projects)
	return projects
}

// CheckPendingNotifications returns tasks that are due for notification and marks them notified.
func (ts *TodoStore) CheckPendingNotifications() []TodoItem {
	ts.lock.Lock()
	now := time.Now()
	var due []TodoItem

	for id, it := range ts.items {
		if !it.Completed && !it.Notified && it.NotifyAt != nil && now.After(*it.NotifyAt) {
			it.Notified = true
			ts.items[id] = it
			due = append(due, it)
		}
	}
	ts.lock.Unlock()

	if len(due) > 0 {
		_ = ts.Save()
	}
	return due
}

// Add creates a task with default project and status.
func (ts *TodoStore) Add(title string, priority string) (*TodoItem, error) {
	return ts.AddWithDetails(title, "General", priority, StatusTodo, nil)
}

// AddWithStatus creates a task in a specific Kanban column.
func (ts *TodoStore) AddWithStatus(title, priority, status string) (*TodoItem, error) {
	return ts.AddWithDetails(title, "General", priority, status, nil)
}

// AddWithDetails creates a task with all fields (Project, Status, Priority, NotifyAt).
func (ts *TodoStore) AddWithDetails(title, project, priority, status string, notifyAt *time.Time) (*TodoItem, error) {
	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		return nil, fmt.Errorf("task title cannot be empty")
	}

	cleanPriority := strings.ToLower(strings.TrimSpace(priority))
	if cleanPriority != "high" && cleanPriority != "low" {
		cleanPriority = "normal"
	}

	colStatus := normalizeStatus(status, false)
	isDone := (colStatus == StatusDone)
	proj := normalizeProject(project)

	ts.lock.Lock()
	id := generateTodoID()
	for {
		if _, exists := ts.items[id]; !exists {
			break
		}
		id = generateTodoID()
	}

	item := TodoItem{
		ID:        id,
		Title:     cleanTitle,
		Project:   proj,
		Status:    colStatus,
		Completed: isDone,
		Priority:  cleanPriority,
		NotifyAt:  notifyAt,
		Notified:  false,
		CreatedAt: time.Now(),
	}
	if isDone {
		now := time.Now()
		item.CompletedAt = &now
	}
	ts.items[id] = item
	ts.lock.Unlock()

	_ = ts.Save()
	return &item, nil
}

// resolveID matches an exact ID or unique prefix.
func (ts *TodoStore) resolveID(targetID string) (string, error) {
	target := strings.ToLower(strings.TrimSpace(targetID))
	if target == "" {
		return "", fmt.Errorf("empty task ID")
	}

	if _, exists := ts.items[target]; exists {
		return target, nil
	}

	var matches []string
	for id := range ts.items {
		if strings.HasPrefix(strings.ToLower(id), target) {
			matches = append(matches, id)
		}
	}

	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("ambiguous task ID '%s' matches %d items", target, len(matches))
	}
	return "", fmt.Errorf("task '%s' not found", target)
}

// SetStatus updates the Kanban column status of a task.
func (ts *TodoStore) SetStatus(targetID, status string) (*TodoItem, error) {
	ts.lock.Lock()
	resolved, err := ts.resolveID(targetID)
	if err != nil {
		ts.lock.Unlock()
		return nil, err
	}

	item := ts.items[resolved]
	colStatus := normalizeStatus(status, false)
	item.Status = colStatus
	item.Completed = (colStatus == StatusDone)
	if item.Completed {
		if item.CompletedAt == nil {
			now := time.Now()
			item.CompletedAt = &now
		}
	} else {
		item.CompletedAt = nil
	}
	ts.items[resolved] = item
	ts.lock.Unlock()

	_ = ts.Save()
	return &item, nil
}

// MoveForward advances task: todo -> in_progress -> done.
func (ts *TodoStore) MoveForward(targetID string) (*TodoItem, error) {
	ts.lock.Lock()
	resolved, err := ts.resolveID(targetID)
	if err != nil {
		ts.lock.Unlock()
		return nil, err
	}

	item := ts.items[resolved]
	switch item.Status {
	case StatusTodo:
		item.Status = StatusInProgress
		item.Completed = false
		item.CompletedAt = nil
	case StatusInProgress:
		item.Status = StatusDone
		item.Completed = true
		now := time.Now()
		item.CompletedAt = &now
	case StatusDone:
		// already done
	}
	ts.items[resolved] = item
	ts.lock.Unlock()

	_ = ts.Save()
	return &item, nil
}

// MoveBackward moves task backward: done -> in_progress -> todo.
func (ts *TodoStore) MoveBackward(targetID string) (*TodoItem, error) {
	ts.lock.Lock()
	resolved, err := ts.resolveID(targetID)
	if err != nil {
		ts.lock.Unlock()
		return nil, err
	}

	item := ts.items[resolved]
	switch item.Status {
	case StatusDone:
		item.Status = StatusInProgress
		item.Completed = false
		item.CompletedAt = nil
	case StatusInProgress:
		item.Status = StatusTodo
		item.Completed = false
		item.CompletedAt = nil
	case StatusTodo:
		// already in todo
	}
	ts.items[resolved] = item
	ts.lock.Unlock()

	_ = ts.Save()
	return &item, nil
}

// Toggle flips completion status between done and todo.
func (ts *TodoStore) Toggle(targetID string) (*TodoItem, error) {
	ts.lock.Lock()
	resolved, err := ts.resolveID(targetID)
	if err != nil {
		ts.lock.Unlock()
		return nil, err
	}

	item := ts.items[resolved]
	if item.Status == StatusDone {
		item.Status = StatusTodo
		item.Completed = false
		item.CompletedAt = nil
	} else {
		item.Status = StatusDone
		item.Completed = true
		now := time.Now()
		item.CompletedAt = &now
	}
	ts.items[resolved] = item
	ts.lock.Unlock()

	_ = ts.Save()
	return &item, nil
}

// SetCompleted explicitly marks a task completed or todo.
func (ts *TodoStore) SetCompleted(targetID string, completed bool) (*TodoItem, error) {
	if completed {
		return ts.SetStatus(targetID, StatusDone)
	}
	return ts.SetStatus(targetID, StatusTodo)
}

// Update edits title, priority, and column.
func (ts *TodoStore) Update(targetID, title, priority, status string) (*TodoItem, error) {
	return ts.UpdateDetails(targetID, title, "", priority, status, nil, false)
}

// UpdateDetails updates all fields including Project and NotifyAt.
func (ts *TodoStore) UpdateDetails(targetID, title, project, priority, status string, notifyAt *time.Time, updateNotify bool) (*TodoItem, error) {
	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		return nil, fmt.Errorf("title cannot be empty")
	}

	ts.lock.Lock()
	resolved, err := ts.resolveID(targetID)
	if err != nil {
		ts.lock.Unlock()
		return nil, err
	}

	item := ts.items[resolved]
	item.Title = cleanTitle
	if project != "" {
		item.Project = normalizeProject(project)
	}
	if priority != "" {
		p := strings.ToLower(strings.TrimSpace(priority))
		if p == "high" || p == "normal" || p == "low" {
			item.Priority = p
		}
	}
	if status != "" {
		item.Status = normalizeStatus(status, false)
		item.Completed = (item.Status == StatusDone)
		if item.Completed {
			if item.CompletedAt == nil {
				now := time.Now()
				item.CompletedAt = &now
			}
		} else {
			item.CompletedAt = nil
		}
	}
	if updateNotify {
		item.NotifyAt = notifyAt
		item.Notified = false
	}
	ts.items[resolved] = item
	ts.lock.Unlock()

	_ = ts.Save()
	return &item, nil
}

// Delete removes a task by ID or prefix.
func (ts *TodoStore) Delete(targetID string) bool {
	ts.lock.Lock()
	resolved, err := ts.resolveID(targetID)
	if err != nil {
		ts.lock.Unlock()
		return false
	}
	delete(ts.items, resolved)
	ts.lock.Unlock()

	_ = ts.Save()
	return true
}

// ClearCompleted removes all completed tasks.
func (ts *TodoStore) ClearCompleted() int {
	ts.lock.Lock()
	count := 0
	for id, item := range ts.items {
		if item.Status == StatusDone || item.Completed {
			delete(ts.items, id)
			count++
		}
	}
	ts.lock.Unlock()

	if count > 0 {
		_ = ts.Save()
	}
	return count
}

// RenameProject updates all tasks belonging to oldProject to newProject.
func (ts *TodoStore) RenameProject(oldProject, newProject string) int {
	cleanOld := strings.ToLower(strings.TrimSpace(oldProject))
	cleanNew := normalizeProject(newProject)
	ts.lock.Lock()
	count := 0
	for id, it := range ts.items {
		if strings.ToLower(it.Project) == cleanOld {
			it.Project = cleanNew
			ts.items[id] = it
			count++
		}
	}
	ts.lock.Unlock()

	if count > 0 {
		_ = ts.Save()
	}
	return count
}

// Counts returns (total, active, completed).
func (ts *TodoStore) Counts() (total, active, completed int) {
	ts.lock.RLock()
	defer ts.lock.RUnlock()

	total = len(ts.items)
	for _, it := range ts.items {
		if it.Status == StatusDone || it.Completed {
			completed++
		} else {
			active++
		}
	}
	return
}

// isAllProjects returns true if the filter represents all projects.
func isAllProjects(filter string) bool {
	p := strings.ToLower(strings.TrimSpace(filter))
	return p == "" || p == "all" || p == "all projects" || p == "all_projects"
}

// ColumnCounts returns (todo, inProgress, done) filtered optionally by project.
func (ts *TodoStore) ColumnCounts(projectFilter ...string) (todo, inProg, done int) {
	ts.lock.RLock()
	defer ts.lock.RUnlock()

	pFilter := ""
	if len(projectFilter) > 0 {
		pFilter = strings.ToLower(strings.TrimSpace(projectFilter[0]))
	}

	for _, it := range ts.items {
		if !isAllProjects(pFilter) && strings.ToLower(it.Project) != pFilter {
			continue
		}
		switch it.Status {
		case StatusInProgress:
			inProg++
		case StatusDone:
			done++
		default:
			todo++
		}
	}
	return
}

// ListByColumn returns items for a column filtered by project and search query.
func (ts *TodoStore) ListByColumn(columnStatus, projectFilter, search string) []TodoItem {
	ts.lock.RLock()
	defer ts.lock.RUnlock()

	targetCol := normalizeStatus(columnStatus, false)
	search = strings.ToLower(strings.TrimSpace(search))
	pFilter := strings.ToLower(strings.TrimSpace(projectFilter))

	var res []TodoItem
	for _, it := range ts.items {
		if it.Status != targetCol {
			continue
		}
		if !isAllProjects(pFilter) && strings.ToLower(it.Project) != pFilter {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(it.Title), search) && !strings.Contains(strings.ToLower(it.Project), search) {
			continue
		}
		res = append(res, it)
	}

	priorityVal := func(p string) int {
		switch strings.ToLower(p) {
		case "high":
			return 3
		case "normal", "":
			return 2
		case "low":
			return 1
		default:
			return 2
		}
	}

	sort.Slice(res, func(i, j int) bool {
		if targetCol != StatusDone {
			pI := priorityVal(res[i].Priority)
			pJ := priorityVal(res[j].Priority)
			if pI != pJ {
				return pI > pJ
			}
			return res[i].CreatedAt.After(res[j].CreatedAt)
		}
		if res[i].CompletedAt != nil && res[j].CompletedAt != nil {
			return res[i].CompletedAt.After(*res[j].CompletedAt)
		}
		return res[i].CreatedAt.After(res[j].CreatedAt)
	})

	return res
}

// List returns items filtered by status/filter, project, and search.
func (ts *TodoStore) List(filter, search string) []TodoItem {
	ts.lock.RLock()
	defer ts.lock.RUnlock()

	filter = strings.ToLower(strings.TrimSpace(filter))
	search = strings.ToLower(strings.TrimSpace(search))

	var res []TodoItem
	for _, it := range ts.items {
		if filter == "active" && (it.Status == StatusDone || it.Completed) {
			continue
		}
		if filter == "completed" && it.Status != StatusDone && !it.Completed {
			continue
		}
		if filter == StatusTodo && it.Status != StatusTodo {
			continue
		}
		if filter == StatusInProgress && it.Status != StatusInProgress {
			continue
		}
		if filter == StatusDone && it.Status != StatusDone {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(it.Title), search) && !strings.Contains(strings.ToLower(it.Project), search) {
			continue
		}
		res = append(res, it)
	}

	priorityVal := func(p string) int {
		switch strings.ToLower(p) {
		case "high":
			return 3
		case "normal", "":
			return 2
		case "low":
			return 1
		default:
			return 2
		}
	}

	sort.Slice(res, func(i, j int) bool {
		if (res[i].Status == StatusDone) != (res[j].Status == StatusDone) {
			return res[i].Status != StatusDone
		}
		if res[i].Status != StatusDone {
			pI := priorityVal(res[i].Priority)
			pJ := priorityVal(res[j].Priority)
			if pI != pJ {
				return pI > pJ
			}
			return res[i].CreatedAt.After(res[j].CreatedAt)
		}
		if res[i].CompletedAt != nil && res[j].CompletedAt != nil {
			return res[i].CompletedAt.After(*res[j].CompletedAt)
		}
		return res[i].CreatedAt.After(res[j].CreatedAt)
	})

	return res
}
