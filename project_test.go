package main

import "testing"

func Test_parseOrigin(t *testing.T) {
	cases := map[string]struct {
		origin string
		owner  string
		repo   string
	}{
		"ssh url": {
			origin: "git@github.com:owner/repo.git",
			owner:  "owner",
			repo:   "repo",
		},
		"https url": {
			origin: "https://github.com/owner/repo.git",
			owner:  "owner",
			repo:   "repo",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			owner, repo, _ := parseOrigin(tc.origin)
			if owner != tc.owner || repo != tc.repo {
				t.Fatalf("got: %s/%s, want %s/%s", owner, repo, tc.owner, tc.repo)
			}
		})
	}
}
