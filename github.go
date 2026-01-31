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

// parseGitHubURL attempts to parse a GitHub URL as either a PR URL or a repository URL.
// Returns (*GitHubPRInfo, nil, nil) for PR URLs, or (nil, *GitHubRepoInfo, nil) for repo URLs.
func parseGitHubURL(url string) (*GitHubPRInfo, error) {
	// Try PR URL pattern first
	matches := prUrlPattern.FindStringSubmatch(url)
	if len(matches) == 4 {
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
	return nil, fmt.Errorf("invalid GitHub URL format. Expected: https://github.com/owner/repo/pull/123 or https://github.com/owner/repo")
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
	owner   string
	repo    string
}

func NewGitHubPRClient(owner, repo, token string) *GitHubPRClient {
	client := &http.Client{Timeout: 30 * time.Second}
	return &GitHubPRClient{
		baseURL: "https://api.github.com",
		client:  client,
		token:   resolveToken(token),
		owner:   owner,
		repo:    repo,
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
		return fmt.Errorf("%s %s returns 404", req.Method, req.URL)
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

func (c *GitHubPRClient) fetchPRMetadata(ctx context.Context, prNumber int) (*GitHubPR, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d", c.baseURL, c.owner, c.repo, prNumber)
	var pr GitHubPR
	err := c.request(ctx, url, &pr)
	return &pr, err
}

func (c *GitHubPRClient) fetchReviewComments(ctx context.Context, prNumber int) ([]GitHubComment, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/comments", c.baseURL, c.owner, c.repo, prNumber)
	var comments []GitHubComment
	err := c.request(ctx, url, &comments)
	return comments, err
}

// fetchLatestOpenPR fetches the most recently created open PR for the repository
func (c *GitHubPRClient) fetchLatestOpenPR(ctx context.Context) (*GitHubPR, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls?state=open&sort=created&direction=desc&per_page=1", c.baseURL, c.owner, c.repo)
	var prs []*GitHubPR
	err := c.request(ctx, url, &prs)
	if err != nil {
		return nil, err
	}

	if len(prs) == 0 {
		return nil, fmt.Errorf("no open pull requests found for %s/%s", c.owner, c.repo)
	}
	return prs[0], nil

}

func (c *GitHubPRClient) Review(ctx context.Context, prNumber int) (*FormattedReview, error) {
	// Fetch PR metadata
	var pr *GitHubPR
	if prNumber <= 0 {
		latest, err := c.fetchLatestOpenPR(ctx)
		if err != nil {
			return nil, err
		}
		pr = latest
	} else {
		npr, err := c.fetchPRMetadata(ctx, prNumber)
		if err != nil {
			return nil, err
		}
		pr = npr
	}

	// Fetch review comments (inline code comments)
	comments, err := c.fetchReviewComments(ctx, pr.Number)
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

type githubCmd struct {
	owner    string
	repo     string
	prNumber int
}

func (*githubCmd) Name() string     { return "github" }
func (*githubCmd) Synopsis() string { return "convert github review to LLM review comment prompt" }
func (*githubCmd) Usage() string    { return "" }

func (g *githubCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&g.owner, "owner", "", "repo owner")
	f.StringVar(&g.owner, "o", "", "repo owner")
	f.StringVar(&g.repo, "repo", "", "repo name")
	f.StringVar(&g.repo, "r", "", "repo name")
	f.IntVar(&g.prNumber, "number", 0, "pr number")
	f.IntVar(&g.prNumber, "n", 0, "pr number")
}

func (g *githubCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...any) subcommands.ExitStatus {
	url := f.Arg(0)
	if url == "" && (g.owner == "" || g.repo == "") {
		fmt.Println("no url or repo info provided.")
		return subcommands.ExitUsageError
	}

	prInfo := &GitHubPRInfo{
		Owner:  g.owner,
		Repo:   g.repo,
		Number: g.prNumber,
	}

	if url != "" {
		info, err := parseGitHubURL(url)
		if err != nil {
			fmt.Println(err)
			return subcommands.ExitUsageError
		}
		prInfo = info
	}

	client := NewGitHubPRClient(prInfo.Owner, prInfo.Repo, "")
	review, err := client.Review(ctx, prInfo.Number)
	if err != nil {
		fmt.Println(err)
		return subcommands.ExitFailure
	}
	fmt.Println(review.String())
	return subcommands.ExitSuccess
}
