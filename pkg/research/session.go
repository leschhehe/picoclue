package research

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// Manager handles research session lifecycle, task execution, and state persistence.
type Manager struct {
	session *Session
	config  ManagerConfig
}

// ManagerConfig holds configuration for the research manager.
type ManagerConfig struct {
	BaseDir       string        // Base directory for all research sessions
	MaxRetries    int           // Default max retries per task
	TaskTimeout   time.Duration // Default timeout per task
	EnableSummary bool          // Whether to generate summaries for artifacts
}

// DefaultManagerConfig returns a ManagerConfig with sensible defaults.
func DefaultManagerConfig() ManagerConfig {
	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		homeDir = "~"
	}
	return ManagerConfig{
		BaseDir:       filepath.Join(homeDir, ".picoclaw", "research"),
		MaxRetries:    3,
		TaskTimeout:   10 * time.Minute,
		EnableSummary: true,
	}
}

// NewManager creates a new research manager for the given session.
func NewManager(session *Session, config ManagerConfig) *Manager {
	return &Manager{
		session: session,
		config:  config,
	}
}

// NewSession creates a new research session with the given goal.
func NewSession(goal string, config ManagerConfig) (*Session, error) {
	sessionID := fmt.Sprintf("research_%s", uuid.New().String()[:8])
	timestamp := time.Now().Format("20060102_150405")
	
	dir := filepath.Join(config.BaseDir, fmt.Sprintf("%s_%s", timestamp, sessionID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create session directory: %w", err)
	}

	stepsDir := filepath.Join(dir, "steps")
	artifactsDir := filepath.Join(dir, "artifacts")
	rawDir := filepath.Join(artifactsDir, "raw")
	
	for _, d := range []string{stepsDir, artifactsDir, rawDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, fmt.Errorf("failed to create subdirectory %s: %w", d, err)
		}
	}

	session := &Session{
		ID:          sessionID,
		Goal:        goal,
		Status:      TaskStatusPending,
		Tasks:       make([]*Task, 0),
		Dir:         dir,
		StateFile:   filepath.Join(dir, "session_state.yaml"),
		PlanFile:    filepath.Join(dir, "plan.md"),
		ReportFile:  filepath.Join(dir, "final_report.md"),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Metadata:    make(map[string]string),
	}

	// Save initial state
	if err := session.SaveState(); err != nil {
		return nil, fmt.Errorf("failed to save initial session state: %w", err)
	}

	return session, nil
}

// SaveState persists the session state to disk atomically.
func (s *Session) SaveState() error {
	s.UpdatedAt = time.Now()
	
	// Write to temporary file first
	tmpFile := s.StateFile + ".tmp"
	data, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("failed to marshal session state: %w", err)
	}

	if err := os.WriteFile(tmpFile, data, 0o644); err != nil {
		return fmt.Errorf("failed to write temporary state file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpFile, s.StateFile); err != nil {
		return fmt.Errorf("failed to rename state file: %w", err)
	}

	return nil
}

// LoadState loads a session state from disk.
func LoadState(stateFile string) (*Session, error) {
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var session Session
	if err := yaml.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session state: %w", err)
	}

	return &session, nil
}

// AddTask adds a new task to the session.
func (s *Session) AddTask(description string, dependencies ...string) *Task {
	task := &Task{
		ID:           fmt.Sprintf("%02d_%s", len(s.Tasks)+1, uuid.New().String()[:6]),
		Description:  description,
		Status:       TaskStatusPending,
		Dependencies: dependencies,
		MaxRetries:   3,
		CreatedAt:    time.Now(),
		Metadata:     make(map[string]string),
	}
	s.Tasks = append(s.Tasks, task)
	s.UpdatedAt = time.Now()
	return task
}

// GetNextPendingTask returns the next task that is ready to execute.
// A task is ready if it's pending and all its dependencies are completed.
func (s *Session) GetNextPendingTask() *Task {
	for _, task := range s.Tasks {
		if task.Status != TaskStatusPending {
			continue
		}

		// Check dependencies
		allDepsCompleted := true
		for _, depID := range task.Dependencies {
			depFound := false
			for _, t := range s.Tasks {
				if t.ID == depID {
					depFound = true
					if t.Status != TaskStatusCompleted {
						allDepsCompleted = false
					}
					break
				}
			}
			if !depFound {
				// Dependency not found, skip this task
				allDepsCompleted = false
				break
			}
		}

		if allDepsCompleted {
			return task
		}
	}
	return nil
}

// HasPendingTasks returns true if there are any tasks still pending or running.
func (s *Session) HasPendingTasks() bool {
	for _, task := range s.Tasks {
		if task.Status == TaskStatusPending || task.Status == TaskStatusRunning || task.Status == TaskStatusRetrying {
			return true
		}
	}
	return false
}

// MarkTaskStarted marks a task as running.
func (s *Session) MarkTaskStarted(taskID string) error {
	task := s.getTask(taskID)
	if task == nil {
		return fmt.Errorf("task %s not found", taskID)
	}

	now := time.Now()
	task.Status = TaskStatusRunning
	task.StartedAt = &now
	s.UpdatedAt = now
	return s.SaveState()
}

// MarkTaskCompleted marks a task as completed with the given result.
func (s *Session) MarkTaskCompleted(taskID, result, resultFile string) error {
	task := s.getTask(taskID)
	if task == nil {
		return fmt.Errorf("task %s not found", taskID)
	}

	now := time.Now()
	task.Status = TaskStatusCompleted
	task.Result = result
	task.ResultFile = resultFile
	task.CompletedAt = &now
	if task.StartedAt != nil {
		task.Duration = now.Sub(*task.StartedAt)
	}
	s.UpdatedAt = now
	return s.SaveState()
}

// MarkTaskFailed marks a task as failed with the given error.
func (s *Session) MarkTaskFailed(taskID, errMsg string) error {
	task := s.getTask(taskID)
	if task == nil {
		return fmt.Errorf("task %s not found", taskID)
	}

	task.Status = TaskStatusFailed
	task.Error = errMsg
	task.RetryCount++
	s.UpdatedAt = time.Now()
	return s.SaveState()
}

// CanRetry returns true if the task can be retried.
func (t *Task) CanRetry() bool {
	return t.RetryCount < t.MaxRetries
}

// ResetForRetry resets the task status for retry.
func (s *Session) ResetForRetry(taskID string) error {
	task := s.getTask(taskID)
	if task == nil {
		return fmt.Errorf("task %s not found", taskID)
	}

	task.Status = TaskStatusPending
	task.Error = ""
	task.StartedAt = nil
	task.CompletedAt = nil
	task.Duration = 0
	s.UpdatedAt = time.Now()
	return s.SaveState()
}

func (s *Session) getTask(taskID string) *Task {
	for _, task := range s.Tasks {
		if task.ID == taskID {
			return task
		}
	}
	return nil
}

// GetCompletedTasks returns all completed tasks.
func (s *Session) GetCompletedTasks() []*Task {
	var completed []*Task
	for _, task := range s.Tasks {
		if task.Status == TaskStatusCompleted {
			completed = append(completed, task)
		}
	}
	return completed
}

// GetFailedTasks returns all failed tasks that cannot be retried.
func (s *Session) GetFailedTasks() []*Task {
	var failed []*Task
	for _, task := range s.Tasks {
		if task.Status == TaskStatusFailed && !task.CanRetry() {
			failed = append(failed, task)
		}
	}
	return failed
}

// Progress returns the completion percentage of the session.
func (s *Session) Progress() float64 {
	if len(s.Tasks) == 0 {
		return 0
	}
	completed := 0
	for _, task := range s.Tasks {
		if task.Status == TaskStatusCompleted {
			completed++
		}
	}
	return float64(completed) / float64(len(s.Tasks)) * 100
}

// Summary returns a human-readable summary of the session status.
func (s *Session) Summary() string {
	total := len(s.Tasks)
	completed := 0
	failed := 0
	running := 0
	pending := 0

	for _, task := range s.Tasks {
		switch task.Status {
		case TaskStatusCompleted:
			completed++
		case TaskStatusFailed:
			failed++
		case TaskStatusRunning, TaskStatusRetrying:
			running++
		case TaskStatusPending, TaskStatusSkipped:
			pending++
		}
	}

	return fmt.Sprintf("Session %s: %d/%d completed, %d running, %d failed, %d pending (%.1f%%)",
		s.ID, completed, total, running, failed, pending, s.Progress())
}
