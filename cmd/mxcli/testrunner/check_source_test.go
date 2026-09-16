// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// TestCheckSourceParsesATestFile is mendixlabs/mxcli#1103.
//
// A .test.mdl file is not a sequence of top-level MDL statements — each block is
// a MICROFLOW BODY, and that is what it becomes when the runner injects it. Fed
// to the top-level grammar, DECLARE is not a statement at all, the parser
// resyncs, RETRIEVE is swallowed as a non-reserved keyword, and `FROM …` starts
// an OQL query whose follow set is {GROUP_BY, SELECT, HAVING} — which is the
// error the reporter chased, on a statement mxcli's own syntax help prints.
func TestCheckSourceParsesATestFile(t *testing.T) {
	src := `/**
 * @test retrieve with a limit
 * @cleanup none
 */
DECLARE $result Boolean = false;
RETRIEVE $reqs FROM Probe.Request WHERE Status = Probe.ENUM_Status.Approved LIMIT 1;
$req = HEAD($reqs);
$result = $req != empty;
/
`
	got, err := CheckSource(src, "x.test.mdl")
	if err != nil {
		t.Fatalf("CheckSource: %v", err)
	}
	if _, errs := visitor.Build(got.MDL); len(errs) > 0 {
		t.Fatalf("a valid test file still does not parse:\n%v\n--- rendered ---\n%s", errs, got.MDL)
	}
}

// TestCheckSourceKeepsSourceLineNumbers is what makes the diagnostics usable: a
// rendered block must sit on the lines the author wrote it on, or every error
// points somewhere else and the reader is worse off than with no check.
func TestCheckSourceKeepsSourceLineNumbers(t *testing.T) {
	src := `/**
 * @test broken
 */
DECLARE $ok Boolean = true;
SET $ok = ;
/
`
	got, err := CheckSource(src, "x.test.mdl")
	if err != nil {
		t.Fatalf("CheckSource: %v", err)
	}
	_, errs := visitor.Build(got.MDL)
	if len(errs) == 0 {
		t.Fatal("the broken statement was not reported at all")
	}
	if !strings.Contains(errs[0].Error(), "line 5:") {
		t.Errorf("error %q is not on line 5, where the bad statement is:\n%s", errs[0], got.MDL)
	}
}

// TestCheckSourceReportsAnnotationProblems: an @expect that cannot be compiled is
// already an ERROR at run time. `mxcli check` is where the author would rather
// hear about it.
func TestCheckSourceReportsAnnotationProblems(t *testing.T) {
	src := `/**
 * @test bad expect
 * @expect count($ok)
 */
DECLARE $ok Boolean = true;
/
`
	got, err := CheckSource(src, "x.test.mdl")
	if err != nil {
		t.Fatalf("CheckSource: %v", err)
	}
	if len(got.Problems) == 0 {
		t.Fatalf("an unusable @expect was not reported: %+v", got)
	}
}

// TestCheckSourceRejectsAMalformedFile: a file the test parser refuses is not
// checkable, and saying so beats reporting the grammar's confusion about it.
func TestCheckSourceRejectsAMalformedFile(t *testing.T) {
	src := `/**
 * @test one
 */
/**
 * @test two
 */
DECLARE $ok Boolean = true;
/
`
	if _, err := CheckSource(src, "x.test.mdl"); err == nil {
		t.Error("a file with two @test comments and no separator was accepted")
	}
}

// TestIsTestFile pins what the translation applies to. A plain .mdl script must
// keep going through the top-level grammar unchanged.
func TestIsTestFile(t *testing.T) {
	cases := map[string]bool{
		"suite.test.mdl":      true,
		"suite.test.md":       true,
		"/a/b/SUITE.TEST.MDL": true,
		"script.mdl":          false,
		"notes.md":            false,
		"-":                   false,
	}
	for name, want := range cases {
		if got := IsTestFile(name); got != want {
			t.Errorf("IsTestFile(%q) = %v, want %v", name, got, want)
		}
	}
}
