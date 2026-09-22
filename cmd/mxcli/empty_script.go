// SPDX-License-Identifier: Apache-2.0

package main

import "strings"

// unparsableInput reports input that has content but produced no statements.
//
// `visitor.Build` returns zero statements AND zero errors for text the grammar
// cannot begin to parse, so it is indistinguishable from an empty file — which
// is why `check` said "Check passed!" and `exec` applied nothing, both at exit 0
// (ako/mxcli#618). A malformed statement that *starts* with a keyword is caught
// normally; this is only for input the parser never got into.
//
// It returns the first line that is neither blank nor a comment, so the message
// can name where reading went wrong rather than just asserting that it did.
func unparsableInput(src string, statements int) (string, bool) {
	if statements > 0 {
		return "", false
	}
	for _, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(raw)
		// A file of nothing but comments is a legitimately empty script, and so
		// is a blank one; neither should be refused.
		if line == "" || strings.HasPrefix(line, "--") || strings.HasPrefix(line, "//") {
			continue
		}
		return line, true
	}
	return "", false
}

// unparsableInputError is the shared message. Both gates say the same thing,
// because a reader who hits it at one of them will try the other.
func unparsableInputError(path, line string) string {
	return "Error: " + path + " produced no statements, but it is not empty.\n" +
		"  The parser could not begin reading it, so there is nothing to " +
		"check or apply.\n" +
		"  First line that did not parse: " + clipLine(line) + "\n" +
		"  A truncated file, a lost heredoc, or the wrong path looks exactly " +
		"like this."
}

func clipLine(s string) string {
	const max = 80
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}
