package main

import (
	"os"
	"testing"
)

func Test_tcr(t *testing.T) {
	got, err := parseTuicrJSON("testdata/projector_388e9be_20260127_152442.json")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/prompt.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got.String() {
		t.Fatalf("want:\n%s\n\ngot:\n%s\n", want, got)
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
