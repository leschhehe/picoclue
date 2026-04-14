package research

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Offloader handles disk-based content storage and retrieval.
// It implements demand-paging: saves raw content to disk, keeps summaries in memory.
type Offloader struct {
	session     *Session
	config      OffloaderConfig
	artifacts   map[string]*Artifact // In-memory index
}

// OffloaderConfig holds configuration for the offloader.
type OffloaderConfig struct {
	// MaxSummaryLength limits the length of generated summaries.
	MaxSummaryLength int
	// PreserveURLs ensures URLs are extracted and preserved during summarization.
	PreserveURLs bool
	// CompressionEnabled enables gzip compression for large files.
	CompressionEnabled bool
}

// DefaultOffloaderConfig returns an OffloaderConfig with sensible defaults.
func DefaultOffloaderConfig() OffloaderConfig {
	return OffloaderConfig{
		MaxSummaryLength: 500,
		PreserveURLs:     true,
		CompressionEnabled: false,
	}
}

// NewOffloader creates a new offloader for the given session.
func NewOffloader(session *Session, config OffloaderConfig) *Offloader {
	return &Offloader{
		session:   session,
		config:    config,
		artifacts: make(map[string]*Artifact),
	}
}

// SaveContent saves content to disk and returns a summary for context.
// It follows the demand-paging pattern: raw content on disk, summary in context.
func (o *Offloader) SaveContent(taskID string, content string, artifactType ArtifactType) (*Artifact, error) {
	// Create artifact ID
	artifactID := fmt.Sprintf("%s_%s", taskID, time.Now().Format("20060102_150405"))
	
	// Determine file path
	var fileName string
	switch artifactType {
	case ArtifactTypeRaw:
		fileName = fmt.Sprintf("%s_raw.txt", artifactID)
	case ArtifactTypeSummary:
		fileName = fmt.Sprintf("%s_summary.md", artifactID)
	case ArtifactTypeReport:
		fileName = fmt.Sprintf("%s_report.md", artifactID)
	case ArtifactTypeMetadata:
		fileName = fmt.Sprintf("%s_meta.json", artifactID)
	default:
		fileName = fmt.Sprintf("%s.txt", artifactID)
	}

	// Secure path: ensure we're within session directory
	baseDir := filepath.Join(o.session.Dir, "artifacts")
	if artifactType == ArtifactTypeRaw {
		baseDir = filepath.Join(baseDir, "raw")
	}
	
	fullPath := filepath.Join(baseDir, fileName)
	
	// Validate path is within session directory (security check)
	cleanPath := filepath.Clean(fullPath)
	cleanBase := filepath.Clean(o.session.Dir)
	if !strings.HasPrefix(cleanPath, cleanBase) {
		return nil, fmt.Errorf("invalid path: attempted to write outside session directory")
	}

	// Write content to file
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("failed to write artifact file: %w", err)
	}

	// Generate summary (heuristic-based for now, can be enhanced with LLM)
	summary := o.generateSummary(content)
	
	// Extract URLs if enabled
	var urls []string
	if o.config.PreserveURLs {
		urls = o.extractURLs(content)
	}

	artifact := &Artifact{
		ID:        artifactID,
		TaskID:    taskID,
		Type:      artifactType,
		Path:      fullPath,
		Summary:   summary,
		URLs:      urls,
		CreatedAt: time.Now(),
		Size:      int64(len(content)),
	}

	// Store in index
	o.artifacts[artifactID] = artifact

	return artifact, nil
}

// LoadContent loads full content from disk by artifact ID or path.
// This implements the "demand" part of demand-paging.
func (o *Offloader) LoadContent(artifactID string) (string, error) {
	artifact, exists := o.artifacts[artifactID]
	if !exists {
		return "", fmt.Errorf("artifact %s not found in index", artifactID)
	}

	content, err := os.ReadFile(artifact.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read artifact file: %w", err)
	}

	return string(content), nil
}

// LoadContentByPath loads content directly from a file path.
// Use this when you have a stored path from a previous SaveContent call.
func (o *Offloader) LoadContentByPath(path string) (string, error) {
	// Validate path is within session directory
	cleanPath := filepath.Clean(path)
	cleanBase := filepath.Clean(o.session.Dir)
	if !strings.HasPrefix(cleanPath, cleanBase) {
		return "", fmt.Errorf("invalid path: attempted to read outside session directory")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	return string(content), nil
}

// GetSummary returns the summary for an artifact without loading full content.
func (o *Offloader) GetSummary(artifactID string) (string, error) {
	artifact, exists := o.artifacts[artifactID]
	if !exists {
		return "", fmt.Errorf("artifact %s not found in index", artifactID)
	}
	return artifact.Summary, nil
}

// GetArtifactPaths returns all artifact paths for a task.
// Useful for building context with pointers to files.
func (o *Offloader) GetArtifactPaths(taskID string) []string {
	var paths []string
	for _, artifact := range o.artifacts {
		if artifact.TaskID == taskID {
			paths = append(paths, artifact.Path)
		}
	}
	return paths
}

// GetAllArtifacts returns all artifacts for the session.
func (o *Offloader) GetAllArtifacts() []*Artifact {
	artifacts := make([]*Artifact, 0, len(o.artifacts))
	for _, artifact := range o.artifacts {
		artifacts = append(artifacts, artifact)
	}
	return artifacts
}

// GetContextForTask builds a context string with summaries and file pointers.
// This is the key method for managing limited context: include summaries, reference files.
func (o *Offloader) GetContextForTask(taskID string) string {
	var sb strings.Builder
	
	sb.WriteString(fmt.Sprintf("## Artifacts for Task %s\n\n", taskID))
	
	for _, artifact := range o.artifacts {
		if artifact.TaskID == taskID {
			sb.WriteString(fmt.Sprintf("### %s (%s)\n", artifact.ID, artifact.Type))
			sb.WriteString(fmt.Sprintf("**Summary:** %s\n", artifact.Summary))
			if len(artifact.URLs) > 0 {
				sb.WriteString("**Sources:**\n")
				for _, url := range artifact.URLs {
					sb.WriteString(fmt.Sprintf("- %s\n", url))
				}
			}
			sb.WriteString(fmt.Sprintf("**Full content:** `%s`\n\n", artifact.Path))
		}
	}
	
	return sb.String()
}

// BuildSynthesisContext builds context for final report synthesis.
// It includes summaries of all completed tasks with file references.
func (o *Offloader) BuildSynthesisContext(completedTasks []*Task) string {
	var sb strings.Builder
	
	sb.WriteString("# Research Synthesis Context\n\n")
	sb.WriteString("This document contains summaries and file references for all completed research tasks.\n")
	sb.WriteString("Use the file paths to load full content when needed for detailed analysis.\n\n")
	
	for _, task := range completedTasks {
		sb.WriteString(fmt.Sprintf("## Task: %s\n", task.ID))
		sb.WriteString(fmt.Sprintf("**Description:** %s\n\n", task.Description))
		
		// Add artifact summaries for this task
		for _, artifact := range o.artifacts {
			if artifact.TaskID == task.ID {
				sb.WriteString(fmt.Sprintf("### Artifact: %s\n", artifact.ID))
				sb.WriteString(fmt.Sprintf("Type: %s\n", artifact.Type))
				sb.WriteString(fmt.Sprintf("Summary: %s\n", artifact.Summary))
				if len(artifact.URLs) > 0 {
					sb.WriteString("Sources:\n")
					for _, url := range artifact.URLs {
						sb.WriteString(fmt.Sprintf("- %s\n", url))
					}
				}
				sb.WriteString(fmt.Sprintf("Full content: `%s`\n\n", artifact.Path))
			}
		}
		
		sb.WriteString("---\n\n")
	}
	
	return sb.String()
}

// generateSummary creates a heuristic-based summary of content.
// In production, this should use an LLM for better quality.
func (o *Offloader) generateSummary(content string) string {
	if len(content) <= o.config.MaxSummaryLength {
		return content
	}

	// Simple truncation with ellipsis
	// Look for natural break points (paragraphs, sentences)
	truncated := content[:o.config.MaxSummaryLength]
	
	// Try to break at paragraph
	if lastPara := strings.LastIndex(truncated, "\n\n"); lastPara > o.config.MaxSummaryLength/2 {
		truncated = truncated[:lastPara]
	} else if lastSentence := strings.LastIndex(truncated, ". "); lastSentence > o.config.MaxSummaryLength*2/3 {
		truncated = truncated[:lastSentence+1]
	}
	
	return truncated + "..."
}

// extractURLs finds and extracts URLs from content.
// This preserves citations during summarization.
func (o *Offloader) extractURLs(content string) []string {
	urls := make([]string, 0)
	
	// Simple URL extraction regex-like approach
	// Look for http:// or https:// patterns
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		words := strings.Fields(line)
		for _, word := range words {
			if strings.HasPrefix(word, "http://") || strings.HasPrefix(word, "https://") {
				// Clean trailing punctuation
				word = strings.TrimRight(word, ".,;:)\"'")
				urls = append(urls, word)
			}
		}
	}
	
	return urls
}

// HashContent generates a hash of content for deduplication.
func (o *Offloader) HashContent(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

// Cleanup removes all artifacts from disk.
// Use with caution - this is irreversible.
func (o *Offloader) Cleanup() error {
	artifactsDir := filepath.Join(o.session.Dir, "artifacts")
	return os.RemoveAll(artifactsDir)
}

// GetStorageUsage returns total storage used by artifacts.
func (o *Offloader) GetStorageUsage() (totalSize int64, fileCount int) {
	for _, artifact := range o.artifacts {
		if info, err := os.Stat(artifact.Path); err == nil {
			totalSize += info.Size()
			fileCount++
		}
	}
	return
}

// ExportResults creates a consolidated export of all results.
func (o *Offloader) ExportResults(outputPath string) error {
	var sb strings.Builder
	
	sb.WriteString("# Research Results Export\n\n")
	sb.WriteString(fmt.Sprintf("Generated: %s\n\n", time.Now().Format(time.RFC3339)))
	
	for _, artifact := range o.GetAllArtifacts() {
		sb.WriteString(fmt.Sprintf("## %s (%s)\n\n", artifact.ID, artifact.Type))
		sb.WriteString(fmt.Sprintf("Summary: %s\n\n", artifact.Summary))
		
		// Include full content
		content, err := o.LoadContent(artifact.ID)
		if err == nil {
			sb.WriteString("```\n")
			sb.WriteString(content)
			sb.WriteString("\n```\n\n")
		}
	}
	
	return os.WriteFile(outputPath, []byte(sb.String()), 0o644)
}
