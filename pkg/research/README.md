# Research System for Picoclaw

This package implements long-horizon research capabilities for Picoclaw, enabling atomic task-based research with limited context management.

## Architecture

The research system follows patterns from Step-DeepResearch, QUASAR, and Tongyi DeepResearch:

1. **Centralized Planner** - The `Orchestrator` decomposes goals and coordinates execution
2. **Stateless Execution** - Each task runs in isolation via `SpawnSubTurn`
3. **Persistent External State** - All results saved to disk with YAML checkpointing
4. **Demand-Paging** - Raw content on disk, summaries in context

## Components

### Types (`types.go`)
- `Session` - Complete research session with goal and tasks
- `Task` - Atomic research subtask with dependencies
- `Plan` - Decomposed research plan
- `Artifact` - Saved research artifact (raw, summary, report)

### Session Manager (`session.go`)
- Creates and manages research sessions
- Handles task state transitions
- Provides checkpointing and recovery
- Tracks progress and dependencies

### Decomposer (`decomposer.go`)
- Breaks down goals using IO-CoT (Intent-Oriented Chain-of-Thought)
- Three modes: Quick (3 tasks), Standard (6 tasks), Deep (10 tasks)
- Validates dependencies and detects cycles
- Estimates task durations

### Offloader (`offloader.go`)
- Saves content to disk with demand-paging
- Generates summaries for context management
- Preserves URLs/citations during summarization
- Builds synthesis context from artifacts

### Orchestrator (`orchestrator.go`)
- Coordinates the entire research process
- Executes tasks sequentially or in parallel
- Synthesizes final reports
- Supports pause/resume/cancel

## Usage

### As a Command

```bash
# Start research via chat command
/research quick <goal>      # Quick overview (2-3 tasks)
/research standard <goal>   # Standard analysis (5-7 tasks)
/research deep <goal>       # Comprehensive research (9+ tasks)
/research status            # Check progress
/research resume            # Resume paused session
/research cancel            # Cancel current session
```

### Programmatically

```go
import "github.com/sipeed/picoclaw/pkg/research"

// Create orchestrator
config := research.DefaultOrchestratorConfig()
config.Mode = research.ModeStandard

orchestrator, err := research.NewOrchestrator(
    "Analyze AI medical diagnostics market",
    agentLoop,
    config,
)

// Start research
if err := orchestrator.Start(ctx); err != nil {
    log.Fatal(err)
}

// Check progress
progress, status := orchestrator.GetProgress()
fmt.Printf("Progress: %.1f%% - %s\n", progress, status)

// Get final report location
session := orchestrator.GetSession()
fmt.Printf("Report saved to: %s\n", session.ReportFile)
```

## Session Directory Structure

```
~/.picoclaw/research/
└── 20250101_120000_research_abc123/
    ├── session_state.yaml    # Master checkpoint file
    ├── plan.md               # Research plan
    ├── steps/                # Individual task results
    │   ├── 01_overview_result.yaml
    │   └── ...
    ├── artifacts/
    │   └── raw/              # Raw HTML/downloads
    └── final_report.md       # Generated report
```

## Key Features

### Task Decomposition
Uses Intent-Oriented Chain-of-Thought (IO-CoT) prompting:
- **WHAT**: What information needs to be gathered?
- **HOW**: How will it be obtained (search, scrape, analyze)?
- **WHY**: Why is each task necessary?

### Context Management
- Saves raw tool outputs to disk
- Injects only high-density summaries into context
- Preserves URLs and citations during summarization
- Builds synthesis context with file pointers

### State Persistence
- Atomic YAML checkpointing after each task
- Crash recovery from last known good state
- Progress tracking and dependency management

### Error Handling
- Automatic retry with exponential backoff
- Dependency-aware task scheduling
- Graceful failure handling

## Configuration

```go
type OrchestratorConfig struct {
    Mode              Mode          // Quick, Standard, Deep
    MaxRetries        int           // Default: 3
    TaskTimeout       time.Duration // Default: 10 minutes
    EnableParallelism bool          // Default: false
    MaxParallel       int           // Default: 2
    AutoSynthesize    bool          // Default: true
}
```

## Integration Points

The research system integrates with:
- **Subagent System** (`pkg/agent/subturn.go`): Uses `SpawnSubTurn` for task execution
- **Tools** (`pkg/tools/`): Leverages search, web, and spawn tools
- **Commands** (`pkg/commands/`): Exposed via `/research` slash command

## Future Enhancements

1. **LLM-based Summarization** - Replace heuristic summaries with actual LLM calls
2. **Parallel Execution** - Implement worker pool for concurrent task execution
3. **Advanced Context Compression** - Add folding and Markovian state reconstruction
4. **Multi-session Management** - Track and manage multiple concurrent research sessions
5. **Enhanced Resume** - Full session resumption from checkpoint files
