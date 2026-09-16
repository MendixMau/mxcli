// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"errors"
	"strings"
	"testing"
)

// TestEndpointCleanupDropsEveryGeneratedFlow is the guard for the second half of
// mendixlabs/mxcli#1104.
//
// Generated names are positional — MxTest.Test_test_1, _2, … — and are reused by
// every test file. Cleaning up only the CURRENT suite's names therefore leaves a
// flow behind whenever a later run has fewer tests than an earlier one, and that
// leftover fails the build of every subsequent run of any file. What is in the
// project is the authority on what to drop; the suite is not.
func TestEndpointCleanupDropsEveryGeneratedFlow(t *testing.T) {
	st := projectState{afterStartup: "MyModule.Startup"}
	// This run has one test; Test_test_7 is an earlier run's leftover.
	present := []string{"MxTest.Test_test_1", "MxTest.Test_test_7"}

	cmds := endpointCleanupCommands(st, present, true)
	joined := strings.Join(cmds, "\n")

	for _, want := range []string{
		"DROP MICROFLOW MxTest.Test_test_1",
		"DROP MICROFLOW MxTest.Test_test_7",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("cleanup does not %s:\n%s", want, joined)
		}
	}
}

// TestCleanupNeverDropsWhatWasNotCreated is the other half of keying cleanup on
// the project rather than on the suite.
//
// An injection that failed part-way leaves some flows created and some not.
// Issuing DROP for every test in the suite makes the missing ones fail, so
// cleanup reported "the project has been left modified" for a project it had
// just cleaned — a false alarm that sends the reader looking for damage.
func TestCleanupNeverDropsWhatWasNotCreated(t *testing.T) {
	st := projectState{}
	cmds := endpointCleanupCommands(st, []string{"MxTest.Test_test_1"}, true)
	if strings.Contains(strings.Join(cmds, "\n"), "Test_test_2") {
		t.Errorf("cleanup drops a flow that was never created:\n%s", strings.Join(cmds, "\n"))
	}
}

// TestGeneratedTestFlowNames keeps the prefix filter honest: a user's own
// microflow in a pre-existing MxTest module is not mxcli's to delete.
func TestGeneratedTestFlowNames(t *testing.T) {
	got := generatedTestFlowNames([]string{"Test_test_1", "MyOwnFlow", "RegisterEndpoint", "Test_test_12"})
	want := []string{"MxTest.Test_test_1", "MxTest.Test_test_12"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("generatedTestFlowNames = %v, want %v", got, want)
	}
}

// TestReportCleanupNamesWhatWasLeft covers the reporter's second ask: a cleanup
// failure that does not say WHICH document survived leaves them to find it by
// hand, which is the step that cost them the debugging cycle.
func TestReportCleanupNamesWhatWasLeft(t *testing.T) {
	var b strings.Builder
	reportCleanup(&b, errors.New("DROP MICROFLOW MxTest.Test_test_2: exit status 1"), []string{"MxTest.Test_test_2"})
	out := b.String()

	if !strings.Contains(out, "MxTest.Test_test_2") {
		t.Errorf("the surviving document is not named:\n%s", out)
	}
	if !strings.Contains(out, "DROP MICROFLOW MxTest.Test_test_2") {
		t.Errorf("no runnable DROP was offered:\n%s", out)
	}
}
