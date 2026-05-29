package cmd

import "strings"

// isUnifiedDiff reports whether the input looks like a git unified-diff stream
// (the format the TUI is built to render).
//
// `git diff` and `git show` produce unified diffs that always contain at least
// one `diff --git a/ b/` header line, regardless of which dialect of the diff
// command was invoked. Summary forms (`--stat`, `--shortstat`, `--name-only`,
// `--name-status`) and metadata-only commands (`git log` with no patch) emit
// other shapes that the renderer cannot consume.
//
// The check requires the header to appear at the start of a line to avoid
// false positives from prose that merely mentions "diff --git".
func isUnifiedDiff(input string) bool {
	for line := range strings.SplitSeq(input, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			return true
		}
	}
	return false
}
