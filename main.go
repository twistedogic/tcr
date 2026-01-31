package main

import (
	"cmp"
	"context"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/google/subcommands"
)

// FormattedComment holds comment data with metadata for sorting and formatting
type FormattedComment struct {
	IsOldSide bool
	File      string
	Line      int
	Type      string
	Content   string
	Index     int
}

func compareFormattedComment(a, b FormattedComment) int {
	switch {
	case len(a.File) > len(b.File):
		return -1
	case len(a.File) < len(b.File):
		return 1
	case a.Line == 0 && b.Line != 0:
		return -1
	case a.Line != 0 && b.Line == 0:
		return 1
	case a.Line != b.Line:
		return -cmp.Compare(a.Line, b.Line)
	}
	// Same line: preserve original order using index
	return cmp.Compare(a.Index, b.Index)
}

func (c FormattedComment) Location() string {
	switch {
	// File-level comment
	case c.Line == 0:
		return fmt.Sprintf("`%s`", c.File)
	case c.IsOldSide:
		return fmt.Sprintf("`%s:~%d`", c.File, c.Line)
	}
	return fmt.Sprintf("`%s:%d`", c.File, c.Line)
}

func (c FormattedComment) CommentType() string {
	return fmt.Sprintf("**[%s]**", strings.ToUpper(c.Type))
}

type FormattedReview struct {
	CommitSha string
	Comments  []FormattedComment
}

func (review *FormattedReview) String() string {
	var output strings.Builder

	// Header
	output.WriteString("I reviewed your code and have the following comments. Please address them.\n\n")

	// Commit line
	shortHash := review.CommitSha
	if len(shortHash) > 7 {
		shortHash = shortHash[:7]
	}
	output.WriteString("Reviewing commit: " + shortHash + "\n\n")

	// Comment type legend
	output.WriteString("Comment types: ISSUE (problems to fix), SUGGESTION (improvements), NOTE (observations), PRAISE (positive feedback)\n\n")

	slices.SortFunc(review.Comments, compareFormattedComment)

	// Generate numbered list
	for i, c := range review.Comments {
		line := fmt.Sprintf("%d. %s %s:\n%s\n\n", i+1, c.CommentType(), c.Location(), c.Content)
		output.WriteString(line)
	}

	return output.String()
}

func main() {
	subcommands.Register(&githubCmd{}, "")
	subcommands.Register(&tuicrCmd{}, "")
	subcommands.Register(&Server{}, "")

	flag.Parse()
	ctx := context.Background()
	os.Exit(int(subcommands.Execute(ctx)))
}
