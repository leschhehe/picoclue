// Package research provides long-horizon research capabilities for Picoclaw.
// It implements atomic task-based research with limited context management,
// following patterns from Step-DeepResearch, QUASAR, and Tongyi DeepResearch.
//
// Key features:
//   - Task decomposition using IO-CoT (Intent-Oriented Chain-of-Thought)
//   - Disk-based offloading with demand-paging
//   - Persistent checkpointing for crash recovery
//   - Context compression and summarization
//
// Usage:
//
//	// Create a research session
//	session := research.NewSession("Analyze AI medical diagnostics market")
//	
//	// Decompose into atomic tasks
//	decomposer := research.NewDecomposer(llmClient)
//	plan, err := decomposer.Decompose(ctx, session.Goal, research.ModeStandard)
//	
//	// Execute tasks with state persistence
//	manager := research.NewManager(session)
//	for _, task := range plan.Tasks {
//	    result, err := manager.ExecuteTask(ctx, task)
//	    if err != nil {
//	        // Handle error, retry, or skip
//	    }
//	    manager.SaveResult(task.ID, result)
//	}
//	
//	// Synthesize final report
//	report, err := manager.SynthesizeReport(ctx)
package research

import (
	"time"
)

// Mode defines the depth of task decomposition.
type Mode int

const (
	// ModeQuick creates 2-3 high-level tasks for simple queries.
	ModeQuick Mode = iota
	// ModeStandard creates 5-7 tasks for typical research.
	ModeStandard
	// ModeDeep creates 9+ tasks for comprehensive research.
	ModeDeep
)

// TaskStatus represents the current state of a research task.
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusRunning    TaskStatus = "running"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusFailed     TaskStatus = "failed"
	TaskStatusSkipped    TaskStatus = "skipped"
	TaskStatusRetrying   TaskStatus = "retrying"
)

// Task represents an atomic research subtask.
type Task struct {
	ID          string            `json:"id" yaml:"id"`
	Description string            `json:"description" yaml:"description"`
	Status      TaskStatus        `json:"status" yaml:"status"`
	Dependencies []string         `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
	Result      string            `json:"result,omitempty" yaml:"result,omitempty"`
	ResultFile  string            `json:"result_file,omitempty" yaml:"result_file,omitempty"`
	Error       string            `json:"error,omitempty" yaml:"error,omitempty"`
	RetryCount  int               `json:"retry_count" yaml:"retry_count"`
	MaxRetries  int               `json:"max_retries" yaml:"max_retries"`
	CreatedAt   time.Time         `json:"created_at" yaml:"created_at"`
	StartedAt   *time.Time        `json:"started_at,omitempty" yaml:"started_at,omitempty"`
	CompletedAt *time.Time        `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
	Duration    time.Duration     `json:"duration,omitempty" yaml:"duration,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// Session represents a complete research session with its goal and tasks.
type Session struct {
	ID           string            `json:"id" yaml:"id"`
	Goal         string            `json:"goal" yaml:"goal"`
	Status       TaskStatus        `json:"status" yaml:"status"`
	Tasks        []*Task           `json:"tasks" yaml:"tasks"`
	Dir          string            `json:"dir" yaml:"dir"`
	StateFile    string            `json:"state_file" yaml:"state_file"`
	PlanFile     string            `json:"plan_file" yaml:"plan_file"`
	ReportFile   string            `json:"report_file" yaml:"report_file"`
	CreatedAt    time.Time         `json:"created_at" yaml:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at" yaml:"updated_at"`
	CompletedAt  *time.Time        `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	CheckpointID string            `json:"checkpoint_id,omitempty" yaml:"checkpoint_id,omitempty"`
}

// Plan represents a decomposed research plan.
type Plan struct {
	Goal        string    `json:"goal" yaml:"goal"`
	Tasks       []*Task   `json:"tasks" yaml:"tasks"`
	Mode        Mode      `json:"mode" yaml:"mode"`
	CreatedAt   time.Time `json:"created_at" yaml:"created_at"`
	Summary     string    `json:"summary,omitempty" yaml:"summary,omitempty"`
	EstimatedDuration time.Duration `json:"estimated_duration,omitempty" yaml:"estimated_duration,omitempty"`
}

// ArtifactType defines the type of saved artifact.
type ArtifactType string

const (
	ArtifactTypeRaw      ArtifactType = "raw"       // Raw HTML, JSON, etc.
	ArtifactTypeSummary  ArtifactType = "summary"   // Summarized content
	ArtifactTypeReport   ArtifactType = "report"    // Final or intermediate reports
	ArtifactTypeMetadata ArtifactType = "metadata"  // Metadata files
)

// Artifact represents a saved research artifact.
type Artifact struct {
	ID        string       `json:"id" yaml:"id"`
	TaskID    string       `json:"task_id" yaml:"task_id"`
	Type      ArtifactType `json:"type" yaml:"type"`
	Path      string       `json:"path" yaml:"path"`
	Summary   string       `json:"summary,omitempty" yaml:"summary,omitempty"`
	URLs      []string     `json:"urls,omitempty" yaml:"urls,omitempty"`
	CreatedAt time.Time    `json:"created_at" yaml:"created_at"`
	Size      int64        `json:"size" yaml:"size"`
}
