package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/subcommands"
)

var prUrlPattern = regexp.MustCompile(`https://github\.com/([^/]+)/([^/]+)/pull/(\d+)`)

// GitHubPRInfo represents parsed GitHub PR URL
type GitHubPRInfo struct {
	Owner  string
	Repo   string
	Number int
}

func (i *GitHubPRInfo) Path() string {
	return fmt.Sprintf("/repos/%s/%s/pulls/%d", i.Owner, i.Repo, i.Number)
}

// GitHubPR represents GitHub PR metadata
type GitHubPR struct {
	Title     string    `json:"title"`
	Number    int       `json:"number"`
	CreatedAt time.Time `json:"created_at"`
	HTMLURL   string    `json:"html_url"`
	Head      struct {
		CommitSha string `json:"sha"`
	} `json:"head"`
}

// GitHubComment represents different types of GitHub comments
type GitHubComment struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	Path      string    `json:"path,omitempty"`     // For review comments
	Line      int       `json:"line,omitempty"`     // For review comments
	Position  int       `json:"position,omitempty"` // For review comments
	CommitSha string    `json:"commit_id"`
	Side      string    `json:"side"`
}

func (c GitHubComment) ToFormattedComment() FormattedComment {
	return FormattedComment{
		Type:      "suggestion",
		Index:     int(c.ID),
		Content:   c.Body,
		File:      c.Path,
		Line:      c.Line,
		IsOldSide: strings.ToLower(c.Side) == "left",
	}
}

func parseGitHubURL(url string) (*GitHubPRInfo, error) {
	matches := prUrlPattern.FindStringSubmatch(url)

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

type GitHubPRClient struct {
	baseURL string
	client  *http.Client
	token   string
	prInfo  *GitHubPRInfo
}

func NewGitHubPRClient(prInfo *GitHubPRInfo, token string) *GitHubPRClient {
	client := &http.Client{Timeout: 30 * time.Second}
	return &GitHubPRClient{
		baseURL: "https://api.github.com",
		client:  client,
		token:   resolveToken(token),
		prInfo:  prInfo,
	}
}

func (c *GitHubPRClient) request(ctx context.Context, url string, result any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}

	resp, err := c.client.Do(req)
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

func (c *GitHubPRClient) fetchPRMetadata(ctx context.Context) (*GitHubPR, error) {
	url := c.baseURL + c.prInfo.Path()
	var pr GitHubPR
	err := c.request(ctx, url, &pr)
	return &pr, err
}

func (c *GitHubPRClient) fetchReviewComments(ctx context.Context) ([]GitHubComment, error) {
	url := c.baseURL + c.prInfo.Path() + "/comments"
	var comments []GitHubComment
	err := c.request(ctx, url, &comments)
	return comments, err
}

func (c *GitHubPRClient) Review(ctx context.Context) (*FormattedReview, error) {
	// Fetch PR metadata
	pr, err := c.fetchPRMetadata(ctx)
	if err != nil {
		return nil, err
	}

	// Fetch review comments (inline code comments)
	comments, err := c.fetchReviewComments(ctx)
	if err != nil {
		return nil, err
	}

	reviewComments := make([]FormattedComment, 0, len(comments))
	for _, c := range comments {
		if pr.Head.CommitSha == c.CommitSha {
			reviewComments = append(reviewComments, c.ToFormattedComment())
		}
	}
	return &FormattedReview{
		CommitSha: pr.Head.CommitSha,
		Comments:  reviewComments,
	}, nil
}

type githubCmd struct{}

func (*githubCmd) Name() string             { return "github" }
func (*githubCmd) Synopsis() string         { return "convert github review to LLM review comment prompt" }
func (*githubCmd) Usage() string            { return "" }
func (*githubCmd) SetFlags(f *flag.FlagSet) {}
func (*githubCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	url := f.Arg(0)
	if url == "" {
		fmt.Println("no url provided.")
		return subcommands.ExitUsageError
	}
	info, err := parseGitHubURL(url)
	if err != nil {
		fmt.Println(err)
		return subcommands.ExitUsageError
	}
	review, err := NewGitHubPRClient(info, "").Review(ctx)
	if err != nil {
		return subcommands.ExitFailure
	}
	fmt.Println(review.String())
	return subcommands.ExitSuccess
}
