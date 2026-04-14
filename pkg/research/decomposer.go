package research

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// Decomposer breaks down research goals into atomic tasks using IO-CoT.
type Decomposer struct {
	agentLoop *agent.AgentLoop
	config    DecomposerConfig
}

// DecomposerConfig holds configuration for the decomposer.
type DecomposerConfig struct {
	// Mode is the default decomposition mode.
	DefaultMode Mode
	// MaxTasks limits the maximum number of tasks generated.
	MaxTasks int
	// EnableDependencyInference enables automatic dependency detection.
	EnableDependencyInference bool
}

// DefaultDecomposerConfig returns a DecomposerConfig with sensible defaults.
func DefaultDecomposerConfig() DecomposerConfig {
	return DecomposerConfig{
		DefaultMode:               ModeStandard,
		MaxTasks:                  15,
		EnableDependencyInference: true,
	}
}

// NewDecomposer creates a new decomposer using the given agent loop.
func NewDecomposer(al *agent.AgentLoop, config DecomposerConfig) *Decomposer {
	return &Decomposer{
		agentLoop: al,
		config:    config,
	}
}

// Decompose breaks down a research goal into atomic tasks.
// It uses Intent-Oriented Chain-of-Thought (IO-CoT) prompting.
func (d *Decomposer) Decompose(ctx context.Context, goal string, mode Mode) (*Plan, error) {
	prompt := d.buildDecompositionPrompt(goal, mode)

	// Use SpawnSubTurn to get decomposition from LLM
	cfg := agent.SubTurnConfig{
		Model:        "default", // Will use configured default model
		SystemPrompt: prompt,
		ActualSystemPrompt: `You are an expert research planner. Your task is to break down complex research goals into atomic, executable tasks.
Follow these principles:
1. Each task should be specific and actionable
2. Tasks should be independent where possible
3. Identify clear dependencies between tasks
4. Order tasks logically (foundational tasks first)
5. Include a final synthesis task

Output format: Return a YAML list of tasks with fields: id, description, dependencies (list of task IDs this task depends on, can be empty)`,
		Async: false,
	}

	result, err := agent.SpawnSubTurn(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to decompose goal: %w", err)
	}

	tasks, err := d.parseDecompositionResult(result.Content, mode)
	if err != nil {
		return nil, fmt.Errorf("failed to parse decomposition result: %w", err)
	}

	plan := &Plan{
		Goal:      goal,
		Tasks:     tasks,
		Mode:      mode,
		CreatedAt: time.Now(),
		Summary:   fmt.Sprintf("Decomposed into %d tasks using %v mode", len(tasks), mode),
	}

	return plan, nil
}

// buildDecompositionPrompt constructs the IO-CoT prompt for task decomposition.
func (d *Decomposer) buildDecompositionPrompt(goal string, mode Mode) string {
	var targetCount int
	switch mode {
	case ModeQuick:
		targetCount = 3
	case ModeStandard:
		targetCount = 6
	case ModeDeep:
		targetCount = 10
	default:
		targetCount = 6
	}

	if targetCount > d.config.MaxTasks {
		targetCount = d.config.MaxTasks
	}

	return fmt.Sprintf(`Analyze this research goal and decompose it into approximately %d atomic tasks.

Research Goal: %s

Use Intent-Oriented Chain-of-Thought (IO-CoT) reasoning:
1. WHAT: What specific information needs to be gathered?
2. HOW: How will each piece of information be obtained (search, scrape, analyze)?
3. WHY: Why is each task necessary for the overall goal?

For each task, consider:
- Is it atomic (single, focused objective)?
- Can it be completed independently or does it depend on other tasks?
- What tool would be most appropriate (search, web scraping, file analysis)?
- What output format would be most useful?

Return the tasks as a YAML list. Each task must have:
- id: A short identifier (e.g., "01_overview", "02_players")
- description: Clear, actionable description of what to do
- dependencies: List of task IDs that must complete before this task (empty list [] if none)

Example output format:
```yaml
- id: "01_overview"
  description: "Search for recent market overview reports on [topic]"
  dependencies: []
- id: "02_key_players"
  description: "Identify top 5-7 key companies/organizations in the space"
  dependencies: ["01_overview"]
- id: "03_analysis"
  description: "Analyze findings and create comparison table"
  dependencies: ["02_key_players"]
```

Generate the task list now:`, targetCount, goal)
}

// parseDecompositionResult parses the LLM's YAML output into Task structs.
func (d *Decomposer) parseDecompositionResult(content string, mode Mode) ([]*Task, error) {
	// Extract YAML block if wrapped in markdown code fences
	yamlContent := content
	if idx := strings.Index(content, "```yaml"); idx >= 0 {
		start := idx + len("```yaml")
		end := strings.Index(content[start:], "```")
		if end >= 0 {
			yamlContent = strings.TrimSpace(content[start : start+end])
		}
	} else if idx := strings.Index(content, "```"); idx >= 0 {
		// Try without language specifier
		start := idx + len("```")
		end := strings.Index(content[start:], "```")
		if end >= 0 {
			yamlContent = strings.TrimSpace(content[start : start+end])
		}
	}

	// Simple YAML parsing (in production, use gopkg.in/yaml.v3)
	// For now, parse line by line
	tasks := make([]*Task, 0)
	
	lines := strings.Split(yamlContent, "\n")
	var currentTask *Task
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "- id:") || strings.HasPrefix(line, "- id: ") {
			// Save previous task
			if currentTask != nil {
				tasks = append(tasks, currentTask)
			}
			
			id := strings.TrimSpace(strings.TrimPrefix(line, "- id:"))
			id = strings.Trim(id, "\"'")
			currentTask = &Task{
				ID:           id,
				Status:       TaskStatusPending,
				Dependencies: make([]string, 0),
				MaxRetries:   3,
				CreatedAt:    time.Now(),
				Metadata:     make(map[string]string),
			}
		} else if strings.HasPrefix(line, "description:") && currentTask != nil {
			desc := strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			desc = strings.Trim(desc, "\"'")
			currentTask.Description = desc
		} else if strings.HasPrefix(line, "dependencies:") && currentTask != nil {
			depsStr := strings.TrimSpace(strings.TrimPrefix(line, "dependencies:"))
			if depsStr == "[]" || depsStr == "" {
				currentTask.Dependencies = make([]string, 0)
			} else {
				// Parse inline array like ["task1", "task2"]
				depsStr = strings.Trim(depsStr, "[]")
				if depsStr != "" {
					parts := strings.Split(depsStr, ",")
					for _, p := range parts {
						p = strings.TrimSpace(p)
						p = strings.Trim(p, "\"'")
						if p != "" {
							currentTask.Dependencies = append(currentTask.Dependencies, p)
						}
					}
				}
			}
		}
	}
	
	// Don't forget the last task
	if currentTask != nil && currentTask.Description != "" {
		tasks = append(tasks, currentTask)
	}

	// Validate and fix task IDs
	taskIDs := make(map[string]bool)
	for _, task := range tasks {
		taskIDs[task.ID] = true
	}

	// Validate dependencies exist
	for _, task := range tasks {
		for _, dep := range task.Dependencies {
			if !taskIDs[dep] {
				// Remove invalid dependency
				newDeps := make([]string, 0)
				for _, d := range task.Dependencies {
					if taskIDs[d] {
						newDeps = append(newDeps, d)
					}
				}
				task.Dependencies = newDeps
			}
		}
	}

	// Check for cycles (simple check)
	if err := d.detectCycles(tasks); err != nil {
		// Remove problematic dependencies to break cycles
		d.breakCycles(tasks)
	}

	// Ensure we have a reasonable number of tasks
	if len(tasks) == 0 {
		// Fallback: create a single task
		tasks = append(tasks, &Task{
			ID:          "01_research",
			Description: fmt.Sprintf("Research: %s", content[:min(200, len(content))]),
			Status:      TaskStatusPending,
			MaxRetries:  3,
			CreatedAt:   time.Now(),
			Metadata:    make(map[string]string),
		})
	}

	return tasks, nil
}

// detectCycles checks for circular dependencies in the task graph.
func (d *Decomposer) detectCycles(tasks []*Task) error {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var dfs func(taskID string) bool
	dfs = func(taskID string) bool {
		visited[taskID] = true
		recStack[taskID] = true

		// Find task
		var task *Task
		for _, t := range tasks {
			if t.ID == taskID {
				task = t
				break
			}
		}
		if task == nil {
			return false
		}

		for _, dep := range task.Dependencies {
			if !visited[dep] {
				if dfs(dep) {
					return true
				}
			} else if recStack[dep] {
				return true // Cycle detected
			}
		}

		recStack[taskID] = false
		return false
	}

	for _, task := range tasks {
		if !visited[task.ID] {
			if dfs(task.ID) {
				return fmt.Errorf("cycle detected in task dependencies")
			}
		}
	}

	return nil
}

// breakCycles removes dependencies to break cycles (greedy approach).
func (d *Decomposer) breakCycles(tasks []*Task) {
	for _, task := range tasks {
		// Remove all dependencies for simplicity in case of cycles
		// A more sophisticated approach would minimally break cycles
		task.Dependencies = make([]string, 0)
	}
}

// EstimateDuration provides a rough duration estimate for a task.
func (d *Decomposer) EstimateDuration(task *Task) time.Duration {
	desc := strings.ToLower(task.Description)
	
	// Heuristics based on task type
	switch {
	case strings.Contains(desc, "search"):
		return 2 * time.Minute
	case strings.Contains(desc, "scrape") || strings.Contains(desc, "website"):
		return 3 * time.Minute
	case strings.Contains(desc, "analyze") || strings.Contains(desc, "compare"):
		return 5 * time.Minute
	case strings.Contains(desc, "synthesize") || strings.Contains(desc, "report") || strings.Contains(desc, "summary"):
		return 10 * time.Minute
	default:
		return 3 * time.Minute
	}
}

// RefinePlan allows iterative refinement of the plan based on intermediate results.
func (d *Decomposer) RefinePlan(ctx context.Context, originalPlan *Plan, completedTasks []*Task, newGoal string) (*Plan, error) {
	// Build context from completed tasks
	var contextBuilder strings.Builder
	contextBuilder.WriteString("Completed tasks so far:\n\n")
	for _, task := range completedTasks {
		contextBuilder.WriteString(fmt.Sprintf("- %s: %s\n", task.ID, task.Result[:min(100, len(task.Result))]))
	}

	prompt := fmt.Sprintf(`Based on the research progress so far, refine the remaining plan.

Original Goal: %s
New/Refined Goal: %s

%s

Should we:
1. Continue with remaining tasks?
2. Add new tasks based on findings?
3. Skip some tasks that are no longer relevant?
4. Change the order of tasks?

Provide an updated task list.`, originalPlan.Goal, newGoal, contextBuilder.String())

	// Similar to Decompose but with context
	cfg := agent.SubTurnConfig{
		Model:              "default",
		SystemPrompt:       prompt,
		ActualSystemPrompt: "You are refining a research plan based on intermediate findings.",
		Async:              false,
	}

	result, err := agent.SpawnSubTurn(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to refine plan: %w", err)
	}

	// Parse and merge with existing plan
	refinedTasks, err := d.parseDecompositionResult(result.Content, originalPlan.Mode)
	if err != nil {
		return nil, fmt.Errorf("failed to parse refined plan: %w", err)
	}

	return &Plan{
		Goal:      newGoal,
		Tasks:     refinedTasks,
		Mode:      originalPlan.Mode,
		CreatedAt: time.Now(),
		Summary:   "Refined plan based on intermediate findings",
	}, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
