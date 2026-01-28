package main

import (
	"bytes"
	"os"
	"testing"
)

func Test_tcr(t *testing.T) {
	review, err := parseJSON("testdata/projector_388e9be_20260127_152442.json")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/prompt.md")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, []byte(generateMarkdown(review))) {
		t.Fail()
	}
}

func TestParseGitHubURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    *GitHubPRInfo
		wantErr bool
	}{
		{
			name:    "valid URL",
			url:     "https://github.com/owner/repo/pull/123",
			want:    &GitHubPRInfo{Owner: "owner", Repo: "repo", Number: 123},
			wantErr: false,
		},
		{
			name:    "invalid URL - missing pull",
			url:     "https://github.com/owner/repo/issues/123",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid URL - not github",
			url:     "https://gitlab.com/owner/repo/pull/123",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid URL - malformed",
			url:     "not-a-url",
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseGitHubURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseGitHubURL() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if got.Owner != tt.want.Owner || got.Repo != tt.want.Repo || got.Number != tt.want.Number {
					t.Fatalf("parseGitHubURL() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestResolveToken(t *testing.T) {
	tests := []struct {
		name     string
		cliToken string
		envToken string
		want     string
	}{
		{
			name:     "CLI token takes precedence",
			cliToken: "cli-token",
			envToken: "env-token",
			want:     "cli-token",
		},
		{
			name:     "env token when no CLI token",
			cliToken: "",
			envToken: "env-token",
			want:     "env-token",
		},
		{
			name:     "empty when neither provided",
			cliToken: "",
			envToken: "",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set env var
			oldEnv := os.Getenv("GITHUB_TOKEN")
			defer os.Setenv("GITHUB_TOKEN", oldEnv)

			if tt.envToken != "" {
				os.Setenv("GITHUB_TOKEN", tt.envToken)
			} else {
				os.Unsetenv("GITHUB_TOKEN")
			}

			got := resolveToken(tt.cliToken)
			if got != tt.want {
				t.Fatalf("resolveToken() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractRepoFromURL(t *testing.T) {
	tests := []struct {
		name    string
		htmlURL string
		want    string
	}{
		{
			name:    "valid PR URL",
			htmlURL: "https://github.com/owner/repo/pull/123",
			want:    "owner/repo",
		},
		{
			name:    "valid PR URL with different owner/repo",
			htmlURL: "https://github.com/octocat/hello-world/pull/1",
			want:    "octocat/hello-world",
		},
		{
			name:    "malformed URL",
			htmlURL: "https://github.com/owner",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractRepoFromURL(tt.htmlURL)
			if got != tt.want {
				t.Fatalf("extractRepoFromURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWriteOutput(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		outputFile string
		wantStdout bool
	}{
		{
			name:       "write to stdout",
			content:    "test content",
			outputFile: "",
			wantStdout: true,
		},
		{
			name:       "write to file",
			content:    "test content",
			outputFile: "/tmp/test_tcr_output.md",
			wantStdout: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.outputFile != "" {
				// Clean up before and after
				os.Remove(tt.outputFile)
				defer os.Remove(tt.outputFile)

				writeOutput(tt.content, tt.outputFile)

				// Verify file was written
				content, err := os.ReadFile(tt.outputFile)
				if err != nil {
					t.Fatalf("failed to read output file: %v", err)
				}
				if string(content) != tt.content {
					t.Fatalf("file content = %v, want %v", string(content), tt.content)
				}
			}
		})
	}
}
