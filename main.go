package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
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

// FormattedComment holds comment data with metadata for sorting and formatting
type FormattedComment struct {
	File    string
	Line    int
	Type    string
	Content string
	Side    string
	Index   int
}

// parseJSON reads and unmarshals a JSON file into a CodeReview struct
func parseJSON(filePath string) (*CodeReview, error) {
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

	return &review, nil
}

// collectComments extracts all comments from the review into a flat slice
func collectComments(review *CodeReview) []FormattedComment {
	var comments []FormattedComment
	index := 0

	for fileName, fileInfo := range review.Files {
		// Collect file-level comments
		for _, comment := range fileInfo.FileComments {
			comments = append(comments, FormattedComment{
				File:    fileName,
				Line:    0, // 0 indicates file-level comment
				Type:    comment.CommentType,
				Content: comment.Content,
				Side:    "",
				Index:   index,
			})
			index++
		}

		// Collect line-level comments
		for lineStr, lineComments := range fileInfo.LineComments {
			line := 0
			fmt.Sscanf(lineStr, "%d", &line)

			for _, comment := range lineComments {
				side := ""
				if comment.Side != nil {
					side = *comment.Side
				}

				comments = append(comments, FormattedComment{
					File:    fileName,
					Line:    line,
					Type:    comment.CommentType,
					Content: comment.Content,
					Side:    side,
					Index:   index,
				})
				index++
			}
		}
	}

	return comments
}

// sortComments sorts comments by file-level first, then by line number, preserving order for same-line comments
func sortComments(comments []FormattedComment) {
	sort.Slice(comments, func(i, j int) bool {
		// File-level comments (line == 0) come before line-level comments
		if comments[i].Line == 0 && comments[j].Line != 0 {
			return true
		}
		if comments[i].Line != 0 && comments[j].Line == 0 {
			return false
		}

		// Both are line-level: sort by line number
		if comments[i].Line != comments[j].Line {
			return comments[i].Line < comments[j].Line
		}

		// Same line: preserve original order using index
		return comments[i].Index < comments[j].Index
	})
}

// formatCommentType converts comment type to uppercase bold brackets
func formatCommentType(commentType string) string {
	return fmt.Sprintf("**[%s]**", strings.ToUpper(commentType))
}

// formatLocation formats the file path and line number with backticks
func formatLocation(file string, line int, side string) string {
	if line == 0 {
		// File-level comment
		return fmt.Sprintf("`%s`", file)
	}

	// Line-level comment
	if side == "old" {
		return fmt.Sprintf("`%s:~%d`", file, line)
	}
	// "new" or empty/null side
	return fmt.Sprintf("`%s:%d`", file, line)
}

// generateMarkdown generates the complete Markdown output from a CodeReview
func generateMarkdown(review *CodeReview) string {
	var output strings.Builder

	// Header
	output.WriteString("I reviewed your code and have the following comments. Please address them.\n\n")

	// Commit line
	shortHash := review.BaseCommit
	if len(shortHash) > 7 {
		shortHash = shortHash[:7]
	}
	output.WriteString(fmt.Sprintf("Reviewing commit: %s\n\n", shortHash))

	// Comment type legend
	output.WriteString("Comment types: ISSUE (problems to fix), SUGGESTION (improvements), NOTE (observations), PRAISE (positive feedback)\n\n")

	// Collect and sort comments
	comments := collectComments(review)
	sortComments(comments)

	// Generate numbered list
	for i, comment := range comments {
		location := formatLocation(comment.File, comment.Line, comment.Side)
		typeLabel := formatCommentType(comment.Type)
		output.WriteString(fmt.Sprintf("%d. %s %s - %s\n", i+1, typeLabel, location, comment.Content))
	}

	// Trailing blank line
	output.WriteString("\n")

	return output.String()
}

func main() {
	// Check for command line argument
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <input.json>")
		os.Exit(1)
	}

	inputPath := os.Args[1]

	// Parse JSON
	review, err := parseJSON(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(generateMarkdown(review))
}
