// SPDX-License-Identifier: Apache-2.0

package microflownorm

import (
	"errors"
	"fmt"

	"github.com/mendixlabs/mxcli/model"
)

// Kind is what a node in the folded region is.
type Kind int

const (
	// KindSplit is a two-way decision. Only Split carries a Condition.
	KindSplit Kind = iota
	// KindMerge is an ExclusiveMerge: one successor, no condition.
	KindMerge
	// KindOther is anything else. Its presence inside a region is what makes
	// that region unfoldable — see ErrImpureRegion.
	KindOther
)

// Node describes one node of the region being folded. The caller builds these
// from the model; this package never touches sdk/microflows, which is what lets
// the whole fold be unit-tested from a literal.
type Node struct {
	Kind Kind
	// Condition is the split's guard, rendered exactly as DESCRIBE renders it.
	Condition string
	// True and False are the split's branch targets.
	True, False model.ID
	// Next is the single successor of a merge or other node. Empty means the
	// path ends here (a return or an end event).
	Next model.ID
}

// Region is the sub-graph between a split and the entry to its shared suffix.
type Region map[model.ID]Node

var (
	// ErrImpureRegion reports a region carrying something other than splits and
	// merges. Folding it would re-order or duplicate a side effect, so the
	// caller must fall back to a faithful rendering instead.
	ErrImpureRegion = errors.New("region contains an activity, so folding it would move a side effect")

	// ErrCyclicRegion reports a back edge. A loop is not a guard and cannot be
	// folded into one.
	ErrCyclicRegion = errors.New("region is cyclic")
)

// RegionCondition returns the condition under which control, starting at
// `from`, reaches `target`.
//
// The recurrence is the obvious one, which is why the interesting work is in
// the preconditions and in Simplify rather than here:
//
//	reach(target)      = true
//	reach(merge m)     = reach(next(m))
//	reach(split s)     = (c ∧ reach(true(s))) ∨ (¬c ∧ reach(false(s)))
//	reach(dead end)    = false
//
// It REFUSES rather than guesses in two cases, both of which would otherwise
// produce a plausible and wrong guard:
//
//   - a node that is neither a split nor a merge (ErrImpureRegion), because the
//     activity would either run under a different condition or not at all; and
//   - a cycle (ErrCyclicRegion), because the recurrence does not terminate and
//     a retry loop is not expressible as a guard.
//
// The returned formula is simplified.
func RegionCondition(r Region, from, target model.ID) (Formula, error) {
	f, err := reach(r, from, target, map[model.ID]bool{})
	if err != nil {
		return nil, err
	}
	return Simplify(f), nil
}

func reach(r Region, at, target model.ID, onPath map[model.ID]bool) (Formula, error) {
	if at == target {
		return Lit(true), nil
	}
	if at == "" {
		return Lit(false), nil
	}
	if onPath[at] {
		return nil, fmt.Errorf("%w: at %s", ErrCyclicRegion, at)
	}
	n, ok := r[at]
	if !ok {
		// Outside the region and not the target: this path leaves without
		// reaching the shared suffix.
		return Lit(false), nil
	}

	onPath[at] = true
	defer delete(onPath, at)

	switch n.Kind {
	case KindMerge:
		return reach(r, n.Next, target, onPath)

	case KindSplit:
		c := Atom(n.Condition)
		t, err := reach(r, n.True, target, onPath)
		if err != nil {
			return nil, err
		}
		fBranch, err := reach(r, n.False, target, onPath)
		if err != nil {
			return nil, err
		}
		return Or{Xs: []Formula{
			And{Xs: []Formula{c, t}},
			And{Xs: []Formula{negate(c), fBranch}},
		}}, nil

	default:
		return nil, fmt.Errorf("%w: at %s", ErrImpureRegion, at)
	}
}
