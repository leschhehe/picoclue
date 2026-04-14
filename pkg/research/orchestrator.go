package research

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/agent"
)

// Orchestrator coordinates long-running research projects.
// It implements the centralized planner pattern with stateless execution.
type Orchestrator struct {
	session    *Session
	manager    *Manager
	decomposer *Decomposer
	offloader  *Offloader
	config     OrchestratorConfig
	
	mu sync.Mutex
}

// OrchestratorConfig holds configuration for the orchestrator.
type OrchestratorConfig struct {
	Mode              Mode          // Decomposition mode
	MaxRetries        int           // Max retries per task
	TaskTimeout       time.Duration // Timeout per task
	EnableParallelism bool          // Allow parallel task execution
	MaxParallel       int           // Maximum concurrent tasks
	AutoSynthesize    bool          // Automatically synthesize final report
}

// DefaultOrchestratorConfig returns an OrchestratorConfig with sensible defaults.
func DefaultOrchestratorConfig() OrchestratorConfig {
	return OrchestratorConfig{
		Mode:              ModeStandard,
		MaxRetries:        3,
		TaskTimeout:       10 * time.Minute,
		EnableParallelism: false, // Start with sequential for safety
		MaxParallel:       2,
		AutoSynthesize:    true,
	}
}

// NewOrchestrator creates a new research orchestrator.
func NewOrchestrator(goal string, al *agent.AgentLoop, config OrchestratorConfig) (*Orchestrator, error) {
	// Create session
	mgrConfig := DefaultManagerConfig()
	session, err := NewSession(goal, mgrConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Create components
	manager := NewManager(session, mgrConfig)
	decomposer := NewDecomposer(al, DefaultDecomposerConfig())
	offloader := NewOffloader(session, DefaultOffloaderConfig())

	return &Orchestrator{
		session:    session,
		manager:    manager,
		decomposer: decomposer,
		offloader:  offloader,
		config:     config,
	}, nil
}

// LoadOrchestrator loads an existing session from disk.
func LoadOrchestrator(stateFile string, al *agent.AgentLoop, config OrchestratorConfig) (*Orchestrator, error) {
	session, err := LoadState(stateFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load session: %w", err)
	}

	mgrConfig := DefaultManagerConfig()
	manager := NewManager(session, mgrConfig)
	decomposer := NewDecomposer(al, DefaultDecomposerConfig())
	offloader := NewOffloader(session, DefaultOffloaderConfig())

	return &Orchestrator{
		session:    session,
		manager:    manager,
		decomposer: decomposer,
		offloader:  offloader,
		config:     config,
	}, nil
}

// Start begins the research process.
// It decomposes the goal, executes tasks, and optionally synthesizes a report.
func (o *Orchestrator) Start(ctx context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	// Step 1: Decompose goal into tasks
	plan, err := o.decomposer.Decompose(ctx, o.session.Goal, o.config.Mode)
	if err != nil {
		return fmt.Errorf("failed to decompose goal: %w", err)
	}

	// Add tasks to session
	for _, task := range plan.Tasks {
		o.session.AddTask(task.Description, task.Dependencies...)
	}
	
	// Save plan to file
	if err := o.savePlan(plan); err != nil {
		return fmt.Errorf("failed to save plan: %w", err)
	}

	o.session.Status = TaskStatusRunning
	if err := o.session.SaveState(); err != nil {
		return fmt.Errorf("failed to update session status: %w", err)
	}

	// Step 2: Execute tasks
	if o.config.EnableParallelism {
		err = o.executeParallel(ctx)
	} else {
		err = o.executeSequential(ctx)
	}
	
	if err != nil {
		o.session.Status = TaskStatusFailed
		o.session.SaveState()
		return fmt.Errorf("task execution failed: %w", err)
	}

	// Step 3: Synthesize report if enabled
	if o.config.AutoSynthesize {
		if err := o.synthesizeReport(ctx); err != nil {
			return fmt.Errorf("failed to synthesize report: %w", err)
		}
	}

	o.session.Status = TaskStatusCompleted
	now := time.Now()
	o.session.CompletedAt = &now
	return o.session.SaveState()
}

// executeSequential runs tasks one by one in dependency order.
func (o *Orchestrator) executeSequential(ctx context.Context) error {
	for o.session.HasPendingTasks() {
		task := o.session.GetNextPendingTask()
		if task == nil {
			// No ready tasks but still has pending - might be stuck dependencies
			break
		}

		if err := o.executeTask(ctx, task); err != nil {
			// Try retry if possible
			if task.CanRetry() {
				o.session.ResetForRetry(task.ID)
				// Retry once immediately
				if retryErr := o.executeTask(ctx, task); retryErr != nil {
					o.session.MarkTaskFailed(task.ID, fmt.Sprintf("Failed after retry: %v", retryErr))
				}
			} else {
				o.session.MarkTaskFailed(task.ID, err.Error())
			}
		}
	}

	// Check for failed tasks
	failedTasks := o.session.GetFailedTasks()
	if len(failedTasks) > 0 {
		return fmt.Errorf("%d tasks failed permanently", len(failedTasks))
	}

	return nil
}

// executeParallel runs tasks in parallel where dependencies allow.
func (o *Orchestrator) executeParallel(ctx context.Context) error {
	// TODO: Implement parallel execution with worker pool
	// For now, fall back to sequential
	return o.executeSequential(ctx)
}

// executeTask executes a single atomic task using SpawnSubTurn.
func (o *Orchestrator) executeTask(ctx context.Context, task *Task) error {
	// Mark as started
	if err := o.session.MarkTaskStarted(task.ID); err != nil {
		return err
	}

	// Build task prompt with context from dependencies
	prompt := o.buildTaskPrompt(task)

	// Execute via SpawnSubTurn
	cfg := agent.SubTurnConfig{
		Model:        "default",
		SystemPrompt: prompt,
		ActualSystemPrompt: fmt.Sprintf(`You are executing a focused research task. 
Task ID: %s
Description: %s

Instructions:
1. Use available tools (search, web scraping) to gather information
2. Be thorough but concise
3. Format your output clearly
4. Include sources/URLs where applicable
5. Stay focused on this specific task only`, task.ID, task.Description),
		Async:   false,
		Timeout: o.config.TaskTimeout,
	}

	result, err := agent.SpawnSubTurn(ctx, cfg)
	if err != nil {
		return fmt.Errorf("task execution failed: %w", err)
	}

	// Save result to disk
	artifact, err := o.offloader.SaveContent(task.ID, result.Content, ArtifactTypeSummary)
	if err != nil {
		return fmt.Errorf("failed to save result: %w", err)
	}

	// Mark task completed
	if err := o.session.MarkTaskCompleted(task.ID, artifact.Summary, artifact.Path); err != nil {
		return err
	}

	return nil
}

// buildTaskPrompt constructs the prompt for a specific task.
func (o *Orchestrator) buildTaskPrompt(task *Task) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Research Task: %s\n\n", task.ID))
	sb.WriteString(fmt.Sprintf("**Goal:** %s\n\n", o.session.Goal))
	sb.WriteString(fmt.Sprintf("**Your Task:** %s\n\n", task.Description))

	// Add context from completed dependencies
	if len(task.Dependencies) > 0 {
		sb.WriteString("## Context from Previous Tasks\n\n")
		completedTasks := o.session.GetCompletedTasks()
		for _, completed := range completedTasks {
			for _, depID := range task.Dependencies {
				if completed.ID == depID {
					sb.WriteString(fmt.Sprintf("### %s\n", completed.ID))
					sb.WriteString(fmt.Sprintf("%s\n\n", completed.Result))
				}
			}
		}
	}

	sb.WriteString("## Instructions\n")
	sb.WriteString("1. Use search tools to find relevant information\n")
	sb.WriteString("2. Use web tools to scrape detailed content if needed\n")
	sb.WriteString("3. Synthesize findings clearly and concisely\n")
	sb.WriteString("4. Include URLs/sources for all claims\n")
	sb.WriteString("5. Format output in markdown\n")

	return sb.String()
}

// synthesizeReport generates the final research report.
func (o *Orchestrator) synthesizeReport(ctx context.Context) error {
	completedTasks := o.session.GetCompletedTasks()
	if len(completedTasks) == 0 {
		return fmt.Errorf("no completed tasks to synthesize")
	}

	// Build synthesis context
	synthesisContext := o.offloader.BuildSynthesisContext(completedTasks)

	prompt := fmt.Sprintf(`# Final Report Synthesis

Research Goal: %s

%s

Generate a comprehensive, well-structured research report in markdown format.
Include:
1. Executive Summary
2. Key Findings
3. Detailed Analysis
4. Sources and References
5. Conclusions

Format the report professionally with proper headings, lists, and citations.`, 
		o.session.Goal, synthesisContext)

	cfg := agent.SubTurnConfig{
		Model:              "default",
		SystemPrompt:       prompt,
		ActualSystemPrompt: "You are writing a professional research report based on gathered data.",
		Async:              false,
		Timeout:            15 * time.Minute,
	}

	result, err := agent.SpawnSubTurn(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to synthesize report: %w", err)
	}

	// Save final report
	reportPath := o.session.ReportFile
	if err := saveToFile(reportPath, result.Content); err != nil {
		return fmt.Errorf("failed to save report: %w", err)
	}

	o.session.ReportFile = reportPath
	return o.session.SaveState()
}

// GetProgress returns current progress percentage and status.
func (o *Orchestrator) GetProgress() (float64, string) {
	return o.session.Progress(), o.session.Summary()
}

// GetSession returns the underlying session.
func (o *Orchestrator) GetSession() *Session {
	return o.session
}

// Pause saves the current state for later resumption.
func (o *Orchestrator) Pause() error {
	o.session.Status = TaskStatusPending
	return o.session.SaveState()
}

// Resume continues a paused session.
func (o *Orchestrator) Resume(ctx context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.session.Status != TaskStatusPending && o.session.Status != TaskStatusRunning {
		return fmt.Errorf("cannot resume session in status: %s", o.session.Status)
	}

	o.session.Status = TaskStatusRunning
	if err := o.session.SaveState(); err != nil {
		return err
	}

	// Continue execution
	if o.config.EnableParallelism {
		return o.executeParallel(ctx)
	}
	return o.executeSequential(ctx)
}

// Cancel stops the research and marks incomplete tasks as skipped.
func (o *Orchestrator) Cancel() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.session.Status = TaskStatusFailed
	for _, task := range o.session.Tasks {
		if task.Status == TaskStatusPending || task.Status == TaskStatusRunning {
			task.Status = TaskStatusSkipped
		}
	}
	return o.session.SaveState()
}

// Status returns a detailed status report.
func (o *Orchestrator) Status() string {
	var sb strings.Builder
	
	sb.WriteString(fmt.Sprintf("Research Session: %s\n", o.session.ID))
	sb.WriteString(fmt.Sprintf("Goal: %s\n\n", o.session.Goal))
	sb.WriteString(fmt.Sprintf("Status: %s\n", o.session.Status))
	sb.WriteString(fmt.Sprintf("Progress: %.1f%%\n\n", o.session.Progress()))
	
	sb.WriteString("Tasks:\n")
	for _, task := range o.session.Tasks {
		statusIcon := "⏳"
		switch task.Status {
		case TaskStatusCompleted:
			statusIcon = "✅"
		case TaskStatusFailed:
			statusIcon = "❌"
		case TaskStatusRunning:
			statusIcon = "🔄"
		case TaskStatusSkipped:
			statusIcon = "⏭️"
		}
		sb.WriteString(fmt.Sprintf("  %s [%s] %s\n", statusIcon, task.ID, task.Description))
		if task.Error != "" {
			sb.WriteString(fmt.Sprintf("      Error: %s\n", task.Error))
		}
	}
	
	return sb.String()
}

func (o *Orchestrator) savePlan(plan *Plan) error {
	var sb strings.Builder
	
	sb.WriteString("# Research Plan\n\n")
	sb.WriteString(fmt.Sprintf("Goal: %s\n", plan.Goal))
	sb.WriteString(fmt.Sprintf("Mode: %v\n", plan.Mode))
	sb.WriteString(fmt.Sprintf("Generated: %s\n\n", plan.CreatedAt.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("Summary: %s\n\n", plan.Summary))
	
	sb.WriteString("## Tasks\n\n")
	for i, task := range plan.Tasks {
		deps := "none"
		if len(task.Dependencies) > 0 {
			deps = strings.Join(task.Dependencies, ", ")
		}
		sb.WriteString(fmt.Sprintf("%d. **%s**: %s\n", i+1, task.ID, task.Description))
		sb.WriteString(fmt.Sprintf("   Dependencies: %s\n\n", deps))
	}
	
	return saveToFile(o.session.PlanFile, sb.String())
}

func saveToFile(path string, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
