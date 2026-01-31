package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/google/subcommands"
)

// Comment represents a code review comment
type Comment struct {
	ID          string  `json:"id"`
	Content     string  `json:"content"`
	CommentType string  `json:"comment_type"`
	CreatedAt   string  `json:"created_at"`
	LineContext *string `json:"line_context"`
	Side        *string `json:"side"`
}

// FileInfo represents information about a file in the review
type FileInfo struct {
	Path         string               `json:"path"`
	Reviewed     bool                 `json:"reviewed"`
	Status       string               `json:"status"`
	FileComments []Comment            `json:"file_comments"`
	LineComments map[string][]Comment `json:"line_comments"`
}

// CodeReview represents the complete code review data structure
type CodeReview struct {
	ID           string              `json:"id"`
	Version      string              `json:"version"`
	RepoPath     string              `json:"repo_path"`
	BaseCommit   string              `json:"base_commit"`
	CreatedAt    string              `json:"created_at"`
	UpdatedAt    string              `json:"updated_at"`
	Files        map[string]FileInfo `json:"files"`
	SessionNotes *string             `json:"session_notes"`
}

func (c *CodeReview) comments() []FormattedComment {
	var comments []FormattedComment
	index := 0

	for fileName, fileInfo := range c.Files {
		// Collect file-level comments
		for _, comment := range fileInfo.FileComments {
			comments = append(comments, FormattedComment{
				File:    fileName,
				Line:    0, // 0 indicates file-level comment
				Type:    comment.CommentType,
				Content: comment.Content,
				Index:   index,
			})
			index++
		}

		// Collect line-level comments
		for lineStr, lineComments := range fileInfo.LineComments {
			line := 0
			fmt.Sscanf(lineStr, "%d", &line)

			for _, comment := range lineComments {
				comments = append(comments, FormattedComment{
					File:      fileName,
					Line:      line,
					Type:      comment.CommentType,
					Content:   comment.Content,
					IsOldSide: comment.Side != nil && *comment.Side == "old",
					Index:     index,
				})
				index++
			}
		}
	}

	return comments
}

func (c *CodeReview) Format() *FormattedReview {
	return &FormattedReview{
		CommitSha: c.BaseCommit,
		Comments:  c.comments(),
	}
}

// parseJSON reads and unmarshals a JSON file into a CodeReview struct
func parseTuicrJSON(filePath string) (*FormattedReview, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	var review CodeReview
	if err := json.Unmarshal(data, &review); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Validate required fields
	if review.ID == "" {
		return nil, fmt.Errorf("missing required field: id")
	}
	if review.Version == "" {
		return nil, fmt.Errorf("missing required field: version")
	}
	if review.Files == nil {
		return nil, fmt.Errorf("missing required field: files")
	}

	return review.Format(), nil
}

type tuicrCmd struct{}

func (*tuicrCmd) Name() string             { return "tuicr" }
func (*tuicrCmd) Synopsis() string         { return "convert tuicr review json to LLM review comment prompt" }
func (*tuicrCmd) Usage() string            { return "" }
func (*tuicrCmd) SetFlags(f *flag.FlagSet) {}
func (*tuicrCmd) Execute(_ context.Context, f *flag.FlagSet, _ ...any) subcommands.ExitStatus {
	file := f.Arg(0)
	if file == "" {
		fmt.Println("no file provided.")
		return subcommands.ExitUsageError
	}
	review, err := parseTuicrJSON(file)
	if err != nil {
		return subcommands.ExitFailure
	}
	fmt.Println(review.String())
	return subcommands.ExitSuccess
}
