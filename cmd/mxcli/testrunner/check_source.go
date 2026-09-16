// SPDX-License-Identifier: Apache-2.0

// Rendering a .test.mdl file as something `mxcli check` and the LSP can check.
//
// A test file is not a sequence of top-level MDL statements. Each block is a
// MICROFLOW BODY — that is literally what the runner turns it into — so feeding
// one to the top-level grammar produces errors about the grammar rather than
// about the file. Measured on the block in mendixlabs/mxcli#1103: `DECLARE` is
// not a statement, the parser resyncs, `RETRIEVE` is swallowed as a non-reserved
// keyword, and the remaining `FROM …` starts an OQL query whose follow set is
// {GROUP_BY, SELECT, HAVING}. The reader is then told their RETRIEVE needs a
// SELECT — on a statement `mxcli syntax microflow.retrieve` prints as its own
// example.
//
// That is not a niche path. The VS Code extension binds the MDL language to
// `.mdl`, which `.test.mdl` matches, so every test file open in the editor was a
// wall of red squiggles; 9 of the 10 test files in this repository report errors
// this way, one of them 392.
//
// The rendering keeps every body on the line the author wrote it on, by padding
// with blank lines and putting the wrapper on the lines the doc comment and the
// '/' separator occupied. A diagnostic then needs no remapping — which is what
// makes this a translation of the source rather than a second parser for it.
package testrunner

import (
	"fmt"
	"path/filepath"
	"strings"
)

// CheckedSource is a test file rendered for checking.
type CheckedSource struct {
	// MDL is one microflow per test block, laid out on the source's own lines.
	MDL string
	// Problems are the things the MDL cannot carry: annotations that claim to
	// assert something and cannot.
	Problems []SourceProblem
}

// SourceProblem is one problem found in a test file's annotations.
type SourceProblem struct {
	Line    int
	Test    string
	Message string
}

// IsTestFile reports whether a path is one of the test file formats.
func IsTestFile(name string) bool { return isTestFile(name) }

// CheckSource renders a test file's blocks as microflows.
//
// It returns an error when the file cannot be parsed as a test file at all — two
// @test comments with no separator between them, say. That is a real problem
// with the file and is reported as itself, rather than as whatever the MDL
// grammar makes of the result.
func CheckSource(content, path string) (CheckedSource, error) {
	var tests []TestCase
	var err error
	if strings.EqualFold(filepath.Ext(path), ".md") {
		tests, err = parseMarkdownTests(content, path)
	} else {
		tests, err = parseMDLTests(content, path)
	}
	if err != nil {
		return CheckedSource{}, err
	}

	lines := strings.Split(content, "\n")
	// One slot per source line, blank unless something is placed on it. A
	// rendered line is only ever the body verbatim or a wrapper fragment, so
	// columns survive too. The spare slot is for a closing fragment on a file
	// whose last test runs to EOF with no '/' after it; appending past the end
	// shifts nothing.
	out := make([]string, len(lines)+1)

	var problems []SourceProblem
	for i, tc := range tests {
		for _, msg := range tc.AssertionErrors {
			problems = append(problems, SourceProblem{Line: tc.Line, Test: tc.Name, Message: msg})
		}
		body := strings.Split(tc.MDL, "\n")
		if tc.MDL == "" || tc.BodyLine <= 0 {
			continue
		}
		first := tc.BodyLine - 1 // 0-based
		if first >= len(out) {
			continue
		}
		for j := range body {
			if k := first + j; k < len(out) && k < len(lines) {
				out[k] = lines[k]
			}
		}
		// A void microflow needs no RETURN, so the wrapper is two fragments and
		// the body between them is exactly what the author typed.
		place(out, first-1, fmt.Sprintf("CREATE OR REPLACE MICROFLOW %s.%s () BEGIN", mxTestModule, checkFlowName(tc, i)))
		place(out, first+len(body), "END; /")
	}

	return CheckedSource{MDL: strings.Join(out, "\n"), Problems: problems}, nil
}

// checkFlowName names the wrapper after the test, because that name is what a
// semantic violation is reported against — linter locations carry a document,
// not a line. "at MxTest.Check_retrieve_with_a_limit" is the author's own words;
// "at MxTest.Check_1" is a number they never wrote and cannot search for.
func checkFlowName(tc TestCase, index int) string {
	var b strings.Builder
	b.WriteString("Check_")
	for _, r := range tc.Name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if tc.Name == "" {
		fmt.Fprintf(&b, "%d", index+1)
	}
	return b.String()
}

// place writes a wrapper fragment onto a line, appending when the line is
// already taken.
//
// Two tests separated by a single-line doc comment want the same line — one for
// its END, the next for its header — and both fragments are complete statements,
// so sharing the line costs nothing and keeps every later line where it was.
// A fragment with no slot left is dropped rather than shifting every line after
// it: the point of this rendering is that a diagnostic's line number is the
// author's, and a missing END is reported on the line it is missing from.
func place(out []string, idx int, fragment string) {
	if idx < 0 || idx >= len(out) {
		return
	}
	if strings.TrimSpace(out[idx]) == "" {
		out[idx] = fragment
		return
	}
	out[idx] += " " + fragment
}
