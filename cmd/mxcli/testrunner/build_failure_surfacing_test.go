// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// failedBuild is the shape a serve /build response has when MxBuild rejected the
// model: the generic Message, with the real detail only in Problems.
func failedBuild(problems ...docker.BuildProblem) *docker.BuildResult {
	return &docker.BuildResult{
		Status:   "Failure",
		Message:  "The project cannot be deployed, because it contains errors.",
		Problems: docker.BuildProblems{Problems: problems},
	}
}

// TestBuildFailureCarriesTheProblems is the guard for mendixlabs/mxcli#1104's
// first half: a rebuild that fails must hand the caller the parsed problems, not
// only the one sentence MxBuild puts in Message.
//
// The sentence is identical for every failing build, so an error carrying only
// that is indistinguishable between "your test does not compile" and "an
// unrelated document in the project is broken" — which is exactly the report.
func TestBuildFailureCarriesTheProblems(t *testing.T) {
	build := failedBuild(problem("CE0097", "The selected 'accs' variable must be of type List.",
		"MxTest", "Microflow 'Test_test_2'", "List operation activity 'Head'"))

	err := buildFailure(build)

	var bf *docker.BuildFailedError
	if !errors.As(err, &bf) {
		t.Fatalf("error is %T, want *docker.BuildFailedError — the caller cannot attribute what it cannot inspect", err)
	}
	if len(bf.BuildErrors()) != 1 {
		t.Fatalf("BuildErrors() = %d, want 1", len(bf.BuildErrors()))
	}
	if msg := err.Error(); !strings.Contains(msg, "CE0097") || !strings.Contains(msg, "Test_test_2") {
		t.Errorf("Error() = %q, want the code and the document it was found in", msg)
	}
}

// TestResultsForBuildFailure covers the shared handling both runners now use: a
// build error belonging to a generated test microflow becomes that test's ERROR
// row, and one belonging to the project is reported with the documents named.
func TestResultsForBuildFailure(t *testing.T) {
	suite := suiteOf("test_1", "test_2")

	t.Run("an error in a test microflow becomes that test's row", func(t *testing.T) {
		err := buildFailure(failedBuild(problem("CE0097", "must be of type List.",
			"MxTest", "Microflow 'Test_test_2'", "List operation activity 'Head'")))

		result, outErr := resultsForBuildFailure(err, suite)
		if outErr != nil {
			t.Fatalf("outErr = %v, want nil — the run reports per-test rows", outErr)
		}
		if result == nil {
			t.Fatal("result = nil, want one row per test")
		}
		byID := map[string]TestResult{}
		for _, r := range result.Tests {
			byID[r.ID] = r
		}
		if byID["test_2"].Status != StatusError {
			t.Errorf("test_2 = %v, want ERROR", byID["test_2"].Status)
		}
		if byID["test_1"].Status != StatusSkip {
			t.Errorf("test_1 = %v, want SKIP — it was never run", byID["test_1"].Status)
		}
	})

	t.Run("an error in the project names the document", func(t *testing.T) {
		// The leftover case: a generated microflow from an EARLIER run is not in
		// this suite, so nothing here can be blamed for it — but the reader still
		// has to be told which document to go and remove.
		err := buildFailure(failedBuild(problem("CE0097", "must be of type List.",
			"MxTest", "Microflow 'Test_test_7'", "List operation activity 'Head'")))

		result, outErr := resultsForBuildFailure(err, suite)
		if result != nil {
			t.Errorf("result = %v, want nil — no test in this suite is at fault", result)
		}
		if outErr == nil {
			t.Fatal("outErr = nil, want the build failure")
		}
		if !strings.Contains(outErr.Error(), "Test_test_7") {
			t.Errorf("Error() = %q, want the leftover document named", outErr.Error())
		}
		// A Test_* microflow in MxTest that no current test owns can only be a
		// leftover from an earlier run. Saying so — and how to remove it — is the
		// difference between one command and a hunt through the model.
		if !strings.Contains(outErr.Error(), "DROP MICROFLOW MxTest.Test_test_7") {
			t.Errorf("Error() = %q, want a runnable DROP for the leftover", outErr.Error())
		}
	})

	t.Run("an error in the user's own model is not called a leftover", func(t *testing.T) {
		err := buildFailure(failedBuild(problem("CE0109", "Undefined variable 'x'.",
			"Sudoku", "Microflow 'SUB_Deal'", "End event")))

		_, outErr := resultsForBuildFailure(err, suite)
		if outErr == nil {
			t.Fatal("outErr = nil, want the build failure")
		}
		if strings.Contains(outErr.Error(), "DROP MICROFLOW") {
			t.Errorf("Error() = %q, must not offer to drop the user's own document", outErr.Error())
		}
	})

	t.Run("a non-build error is passed through untouched", func(t *testing.T) {
		want := fmt.Errorf("the runtime is not running")
		result, outErr := resultsForBuildFailure(want, suite)
		if result != nil || !errors.Is(outErr, want) {
			t.Errorf("resultsForBuildFailure(%v) = %v, %v; want nil, the same error", want, result, outErr)
		}
	})
}
