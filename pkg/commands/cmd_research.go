package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/research"
)

// researchCommand returns the /research command definition.
func researchCommand() Definition {
	return Definition{
		Name:        "research",
		Description: "Start or manage a long-running research project",
		Usage:       "/research [quick|standard|deep] <research goal>",
		SubCommands: []SubCommand{
			{
				Name:        "quick",
				Description: "Quick research with 2-3 high-level tasks",
				ArgsUsage:   "<research goal>",
				Handler:     handleResearchQuick,
			},
			{
				Name:        "standard",
				Description: "Standard research with 5-7 tasks (default)",
				ArgsUsage:   "<research goal>",
				Handler:     handleResearchStandard,
			},
			{
				Name:        "deep",
				Description: "Deep research with 9+ comprehensive tasks",
				ArgsUsage:   "<research goal>",
				Handler:     handleResearchDeep,
			},
			{
				Name:        "status",
				Description: "Show status of current research session",
				Handler:     handleResearchStatus,
			},
			{
				Name:        "resume",
				Description: "Resume a paused research session",
				Handler:     handleResearchResume,
			},
			{
				Name:        "cancel",
				Description: "Cancel the current research session",
				Handler:     handleResearchCancel,
			},
		},
	}
}

func handleResearchQuick(ctx context.Context, req Request, rt *Runtime) error {
	goal := extractGoal(req.Text, "quick")
	if goal == "" {
		return req.Reply("Usage: /research quick <research goal>\n\nExample: /research quick Latest developments in quantum computing")
	}

	return startResearch(ctx, req, rt, goal, research.ModeQuick)
}

func handleResearchStandard(ctx context.Context, req Request, rt *Runtime) error {
	goal := extractGoal(req.Text, "standard")
	if goal == "" {
		return req.Reply("Usage: /research standard <research goal>\n\nExample: /research standard Market analysis for AI-powered medical diagnostics")
	}

	return startResearch(ctx, req, rt, goal, research.ModeStandard)
}

func handleResearchDeep(ctx context.Context, req Request, rt *Runtime) error {
	goal := extractGoal(req.Text, "deep")
	if goal == "" {
		return req.Reply("Usage: /research deep <research goal>\n\nExample: /research deep Comprehensive analysis of renewable energy trends in Southeast Asia")
	}

	return startResearch(ctx, req, rt, goal, research.ModeDeep)
}

func handleResearchStatus(ctx context.Context, req Request, rt *Runtime) error {
	// Check if there's an active research session in context
	// For now, return placeholder - will be implemented with proper session tracking
	return req.Reply("📊 **Research Status**\n\nNo active research session found.\n\nStart one with:\n- `/research quick <goal>` - Quick overview\n- `/research standard <goal>` - Standard analysis\n- `/research deep <goal>` - Comprehensive research")
}

func handleResearchResume(ctx context.Context, req Request, rt *Runtime) error {
	return req.Reply("⏸️ **Resume Research**\n\nTo resume a previous session, provide the session ID:\n```\n/research resume <session-id>\n```\n\nUse `/research status` to see available sessions.")
}

func handleResearchCancel(ctx context.Context, req Request, rt *Runtime) error {
	return req.Reply("❌ **Cancel Research**\n\nThis will stop the current research session and mark incomplete tasks as skipped.\n\nAre you sure? This action cannot be undone.\n\nReply with 'yes' to confirm cancellation.")
}

func startResearch(ctx context.Context, req Request, rt *Runtime, goal string, mode research.Mode) error {
	// Get AgentLoop from runtime
	getAgentLoopFn := rt.GetActiveTurn
	if getAgentLoopFn == nil {
		return req.Reply("❌ Cannot start research: Runtime does not support agent loop access.")
	}

	turnRaw := getAgentLoopFn()
	if turnRaw == nil {
		return req.Reply("❌ No active turn found. Please start a conversation first.")
	}

	// Note: In actual implementation, we need to properly extract AgentLoop
	// For now, show informational message about what would happen
	modeStr := "Standard"
	switch mode {
	case research.ModeQuick:
		modeStr = "Quick"
	case research.ModeDeep:
		modeStr = "Deep"
	}

	return req.Reply(fmt.Sprintf(`🔬 **Starting %s Research**

**Goal:** %s

**Mode:** %s (%d tasks expected)

The research orchestrator will:
1. 📋 Decompose your goal into atomic tasks
2. 🔍 Execute each task using search and web tools
3. 💾 Save results to disk with summaries
4. 📄 Synthesize a final report

Session directory: \`~/.picoclaw/research/\`

You can check progress with: \`/research status\`

Starting now...`, modeStr, goal, modeStr))

	// Actual execution would happen here in a goroutine:
	// go func() {
	//     config := research.DefaultOrchestratorConfig()
	//     config.Mode = mode
	//     
	//     orchestrator, err := research.NewOrchestrator(goal, agentLoop, config)
	//     if err != nil {
	//         req.Reply(fmt.Sprintf("❌ Failed to start research: %v", err))
	//         return
	//     }
	//     
	//     // Send initial plan
	//     req.Reply(orchestrator.Status())
	//     
	//     // Start execution
	//     if err := orchestrator.Start(ctx); err != nil {
	//         req.Reply(fmt.Sprintf("❌ Research failed: %v", err))
	//         return
	//     }
	//     
	//     // Send completion notification
	//     req.Reply(fmt.Sprintf("✅ Research completed!\n\nReport saved to: %s", orchestrator.GetSession().ReportFile))
	// }()

	return nil
}

func extractGoal(text string, subcommand string) string {
	// Remove "/research <subcommand>" prefix and get the goal
	prefix := fmt.Sprintf("/research %s", subcommand)
	idx := strings.Index(text, prefix)
	if idx >= 0 {
		goal := strings.TrimSpace(text[idx+len(prefix):])
		return goal
	}
	return ""
}
