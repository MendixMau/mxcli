// SPDX-License-Identifier: Apache-2.0

package microflownorm

import (
	"errors"
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

// The reporter's graph, mendixlabs/mxcli#923:
//
//	split1 : true → split2      false → merge1
//	split2 : true → merge1      false → merge2
//
// `log` sits on merge1, so it runs on reach(merge1) = ¬c1 ∨ (c1 ∧ c2), which
// must fold to ¬c1 ∨ c2. Describing it as `c1 ∧ c2` is the actual bug: with the
// reporter's expressions, a program that always logs described as one that
// never does.
func TestRegionCondition_ReportersGraph(t *testing.T) {
	r := Region{
		"split1": {Kind: KindSplit, Condition: "$a > 1", True: "split2", False: "merge1"},
		"split2": {Kind: KindSplit, Condition: "$b > 2", True: "merge1", False: "merge2"},
		"merge2": {Kind: KindMerge, Next: ""},
	}
	got, err := RegionCondition(r, "split1", "merge1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "$b > 2 or not($a > 1)"; got.String() != want {
		t.Errorf("condition = %q, want %q", got.String(), want)
	}
}

// The negative control for the case above: describing that graph as the
// conjunction is the #923 defect, so the fold must NOT produce it.
func TestRegionCondition_IsNotTheConjunction(t *testing.T) {
	r := Region{
		"split1": {Kind: KindSplit, Condition: "$a", True: "split2", False: "merge1"},
		"split2": {Kind: KindSplit, Condition: "$b", True: "merge1", False: "merge2"},
		"merge2": {Kind: KindMerge, Next: ""},
	}
	got, _ := RegionCondition(r, "split1", "merge1")
	if got.String() == "$a and $b" {
		t.Fatal("folded to the conjunction — this is exactly the #923 misdescription")
	}
}

// A plain nested if: only the true branch reaches the target, so the guard is
// just the condition. Nothing to fold, and folding must not invent structure.
func TestRegionCondition_SingleSplit(t *testing.T) {
	r := Region{
		"s": {Kind: KindSplit, Condition: "$x = 1", True: "target", False: "other"},
	}
	got, err := RegionCondition(r, "s", "target")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "$x = 1"; got.String() != want {
		t.Errorf("condition = %q, want %q", got.String(), want)
	}
}

// Both branches reach the target, so the guard is vacuously true. Emitting
// `if true then` would be silly; the caller uses Lit(true) to skip the wrapper.
func TestRegionCondition_BothBranchesReachTarget(t *testing.T) {
	r := Region{
		"s": {Kind: KindSplit, Condition: "$x", True: "target", False: "target"},
	}
	got, err := RegionCondition(r, "s", "target")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.String() != "true" {
		t.Errorf("condition = %q, want true", got.String())
	}
}

// THE PRECONDITION. An activity inside the region means folding would move a
// side effect, so this must refuse rather than produce a guard. Without it,
// Mode 3 would silently hoist a `create object` out of a branch.
func TestRegionCondition_RefusesImpureRegion(t *testing.T) {
	r := Region{
		"s":      {Kind: KindSplit, Condition: "$x", True: "create", False: "target"},
		"create": {Kind: KindOther, Next: "target"},
	}
	_, err := RegionCondition(r, "s", "target")
	if !errors.Is(err, ErrImpureRegion) {
		t.Fatalf("error = %v, want ErrImpureRegion — folding a region with an activity moves a side effect", err)
	}
}

// A back edge is a loop, not a guard.
func TestRegionCondition_RefusesCycle(t *testing.T) {
	r := Region{
		"s": {Kind: KindSplit, Condition: "$x", True: "m", False: "target"},
		"m": {Kind: KindMerge, Next: "s"},
	}
	_, err := RegionCondition(r, "s", "target")
	if !errors.Is(err, ErrCyclicRegion) {
		t.Fatalf("error = %v, want ErrCyclicRegion", err)
	}
}

// A path that leaves the region without reaching the target contributes
// nothing — it must not be mistaken for a path that arrives.
func TestRegionCondition_PathLeavingRegionIsFalse(t *testing.T) {
	r := Region{
		"s": {Kind: KindSplit, Condition: "$x", True: "elsewhere", False: "target"},
	}
	got, err := RegionCondition(r, "s", "target")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "not($x)"; got.String() != want {
		t.Errorf("condition = %q, want %q", got.String(), want)
	}
}

// DESCRIBE must be a fixed point, so folding the same graph twice must give the
// same text — no map iteration leaking into the output.
func TestRegionCondition_IsDeterministic(t *testing.T) {
	build := func() Region {
		return Region{
			"split1": {Kind: KindSplit, Condition: "$a", True: "split2", False: "merge1"},
			"split2": {Kind: KindSplit, Condition: "$b", True: "merge1", False: "m2"},
			"m2":     {Kind: KindMerge, Next: ""},
		}
	}
	first, err := RegionCondition(build(), "split1", "merge1")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		got, err := RegionCondition(build(), "split1", "merge1")
		if err != nil {
			t.Fatal(err)
		}
		if got.String() != first.String() {
			t.Fatalf("run %d gave %q, first gave %q", i, got.String(), first.String())
		}
	}
}

// Three levels deep, to show the fold is not special-cased to the two-split
// shape it was derived from.
func TestRegionCondition_ThreeSplits(t *testing.T) {
	r := Region{
		"s1": {Kind: KindSplit, Condition: "$a", True: "s2", False: "target"},
		"s2": {Kind: KindSplit, Condition: "$b", True: "s3", False: "target"},
		"s3": {Kind: KindSplit, Condition: "$c", True: "target", False: "out"},
	}
	got, err := RegionCondition(r, "s1", "target")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// reach = ¬a ∨ (a ∧ ¬b) ∨ (a ∧ b ∧ c) ≡ ¬a ∨ ¬b ∨ c
	if want := "$c or not($a) or not($b)"; got.String() != want {
		t.Errorf("condition = %q, want %q", got.String(), want)
	}
}

var _ = model.ID("")
