package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
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

// GitHubPRInfo represents parsed GitHub PR URL
type GitHubPRInfo struct {
	Owner  string
	Repo   string
	Number int
}

// GitHubPR represents GitHub PR metadata
type GitHubPR struct {
	Title     string     `json:"title"`
	Number    int        `json:"number"`
	User      GitHubUser `json:"user"`
	CreatedAt time.Time  `json:"created_at"`
	HTMLURL   string     `json:"html_url"`
	Head      struct {
		CommitSha string `json:"sha"`
	} `json:"head"`
}

// GitHubUser represents a GitHub user
type GitHubUser struct {
	Login string `json:"login"`
}

// GitHubComment represents different types of GitHub comments
type GitHubComment struct {
	ID        int64      `json:"id"`
	Body      string     `json:"body"`
	User      GitHubUser `json:"user"`
	CreatedAt time.Time  `json:"created_at"`
	Path      string     `json:"path,omitempty"`           // For review comments
	Line      int        `json:"line,omitempty"`           // For review comments
	Position  int        `json:"position,omitempty"`       // For review comments
	InReplyTo int64      `json:"in_reply_to_id,omitempty"` // For threaded comments
	CommitSha string     `json:"commit_id"`
}

// GitHubReview represents a PR review
type GitHubReview struct {
	ID        int64      `json:"id"`
	User      GitHubUser `json:"user"`
	Body      string     `json:"body"`
	State     string     `json:"state"`
	CreatedAt time.Time  `json:"created_at"`
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

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  tcr convert json <file>        Convert JSON file to markdown")
	fmt.Println("  tcr convert github <url>       Convert GitHub PR to markdown")
	fmt.Println("")
	fmt.Println("Flags:")
	fmt.Println("  --token <token>    GitHub authentication token (or use GITHUB_TOKEN env var)")
	fmt.Println("  --output <file>    Write output to file instead of stdout")
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	if command == "convert" {
		if len(os.Args) < 4 {
			printUsage()
			os.Exit(1)
		}

		subcommand := os.Args[2]
		source := os.Args[3]

		// Parse flags
		var token, outputFile string
		for i := 4; i < len(os.Args); i++ {
			if os.Args[i] == "--token" && i+1 < len(os.Args) {
				token = os.Args[i+1]
				i++
			} else if os.Args[i] == "--output" && i+1 < len(os.Args) {
				outputFile = os.Args[i+1]
				i++
			}
		}

		switch subcommand {
		case "json":
			convertJSON(source, outputFile)
		case "github":
			convertGitHub(source, token, outputFile)
		default:
			fmt.Fprintf(os.Stderr, "Unknown subcommand: %s\n", subcommand)
			printUsage()
			os.Exit(1)
		}
	} else {
		// Legacy mode: direct JSON file input
		convertJSON(os.Args[1], "")
	}
}

func convertJSON(inputPath, outputFile string) {
	review, err := parseJSON(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	markdown := generateMarkdown(review)
	writeOutput(markdown, outputFile)
}

func convertGitHub(url, token, outputFile string) {
	// Parse URL
	prInfo, err := parseGitHubURL(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Resolve authentication token
	authToken := resolveToken(token)

	// Fetch PR data
	pr, reviewComments, err := fetchGitHubPR(prInfo, authToken)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Generate markdown
	markdown := generateGitHubMarkdown(pr, reviewComments)
	writeOutput(markdown, outputFile)
}

func parseGitHubURL(url string) (*GitHubPRInfo, error) {
	pattern := `https://github\.com/([^/]+)/([^/]+)/pull/(\d+)`
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(url)

	if len(matches) != 4 {
		return nil, fmt.Errorf("invalid GitHub PR URL format. Expected: https://github.com/owner/repo/pull/123")
	}

	number, err := strconv.Atoi(matches[3])
	if err != nil {
		return nil, fmt.Errorf("invalid PR number: %s", matches[3])
	}

	return &GitHubPRInfo{
		Owner:  matches[1],
		Repo:   matches[2],
		Number: number,
	}, nil
}

func resolveToken(cliToken string) string {
	if cliToken != "" {
		return cliToken
	}
	return os.Getenv("GITHUB_TOKEN")
}

func fetchGitHubPR(prInfo *GitHubPRInfo, token string) (*GitHubPR, []GitHubComment, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	// Fetch PR metadata
	pr, err := fetchPRMetadata(client, prInfo, token)
	if err != nil {
		return nil, nil, err
	}

	// Fetch review comments (inline code comments)
	reviewComments, err := fetchReviewComments(client, prInfo, token)
	if err != nil {
		return nil, nil, err
	}

	return pr, reviewComments, nil
}

func fetchPRMetadata(client *http.Client, prInfo *GitHubPRInfo, token string) (*GitHubPR, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d", prInfo.Owner, prInfo.Repo, prInfo.Number)
	var pr GitHubPR
	err := makeGitHubRequest(client, url, token, &pr)
	return &pr, err
}

func fetchReviewComments(client *http.Client, prInfo *GitHubPRInfo, token string) ([]GitHubComment, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d/comments", prInfo.Owner, prInfo.Repo, prInfo.Number)
	var comments []GitHubComment
	err := makeGitHubRequest(client, url, token, &comments)
	return comments, err
}

func makeGitHubRequest(client *http.Client, url, token string, result any) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("network error: %w. Please check your internet connection", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return fmt.Errorf("PR not found. Check the URL or ensure you have access to this repository")
	}

	if resp.StatusCode == 403 {
		resetTime := resp.Header.Get("X-RateLimit-Reset")
		if resetTime != "" {
			timestamp, _ := strconv.ParseInt(resetTime, 10, 64)
			resetAt := time.Unix(timestamp, 0)
			return fmt.Errorf("GitHub API rate limit exceeded. Reset at: %s. Consider using a GitHub token (--token or GITHUB_TOKEN env var)", resetAt.Format(time.RFC3339))
		}
		return fmt.Errorf("access forbidden. This may be a private repository. Set GITHUB_TOKEN environment variable or use --token flag")
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error (status %d): %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("failed to parse GitHub response: %w", err)
	}

	return nil
}

func generateGitHubMarkdown(pr *GitHubPR, comments []GitHubComment) string {
	var output strings.Builder

	// Header with PR metadata
	output.WriteString(fmt.Sprintf("# Pull Request: %s\n", pr.Title))
	output.WriteString(fmt.Sprintf("Repository: %s\n", extractRepoFromURL(pr.HTMLURL)))
	output.WriteString(fmt.Sprintf("PR #%d | Author: @%s | Created: %s\n\n",
		pr.Number, pr.User.Login, pr.CreatedAt.Format(time.DateOnly)))

	hasContent := false

	reviewComments := make([]GitHubComment, 0, len(comments))

	for _, c := range comments {
		if pr.Head.CommitSha == c.CommitSha {
			reviewComments = append(reviewComments, c)
		}
	}

	// Review comments (inline code comments)
	if len(reviewComments) > 0 {
		hasContent = true
		output.WriteString("## Review Comments\n\n")

		// Group by file and line
		fileComments := groupReviewComments(reviewComments)

		for _, fc := range fileComments {
			if fc.Line > 0 {
				output.WriteString(fmt.Sprintf("### File: %s:%d\n", fc.Path, fc.Line))
			} else {
				output.WriteString(fmt.Sprintf("### File: %s\n", fc.Path))
			}

			for _, comment := range fc.Comments {
				output.WriteString(fmt.Sprintf("**@%s** (%s):\n%s\n\n",
					comment.User.Login,
					comment.CreatedAt.Format(time.DateTime),
					comment.Body))
			}
		}
	}

	if !hasContent {
		output.WriteString("This pull request has no comments.\n\n")
	}

	return output.String()
}

type fileCommentGroup struct {
	Path     string
	Line     int
	Comments []GitHubComment
}

func groupReviewComments(comments []GitHubComment) []fileCommentGroup {
	groups := make(map[string]*fileCommentGroup)

	for _, comment := range comments {
		key := fmt.Sprintf("%s:%d", comment.Path, comment.Line)
		if _, exists := groups[key]; !exists {
			groups[key] = &fileCommentGroup{
				Path:     comment.Path,
				Line:     comment.Line,
				Comments: []GitHubComment{},
			}
		}
		groups[key].Comments = append(groups[key].Comments, comment)
	}

	// Convert map to slice and sort
	result := make([]fileCommentGroup, 0, len(groups))
	for _, group := range groups {
		// Sort comments within group by creation time
		sort.Slice(group.Comments, func(i, j int) bool {
			return group.Comments[i].CreatedAt.Before(group.Comments[j].CreatedAt)
		})
		result = append(result, *group)
	}

	// Sort groups by file path then line number
	sort.Slice(result, func(i, j int) bool {
		if result[i].Path != result[j].Path {
			return result[i].Path < result[j].Path
		}
		return result[i].Line < result[j].Line
	})

	return result
}

func extractRepoFromURL(htmlURL string) string {
	// Extract owner/repo from https://github.com/owner/repo/pull/123
	parts := strings.Split(htmlURL, "/")
	if len(parts) >= 5 {
		return parts[3] + "/" + parts[4]
	}
	return ""
}

func writeOutput(content, outputFile string) {
	if outputFile != "" {
		err := os.WriteFile(outputFile, []byte(content), 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing to file %s: %v\n", outputFile, err)
			os.Exit(1)
		}
	} else {
		fmt.Print(content)
	}
}
