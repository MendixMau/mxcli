// SPDX-License-Identifier: Apache-2.0

// Package microflownorm folds the guards of a recombinable microflow graph into
// one condition, so a graph that nested `if`s cannot describe can still be
// described without duplicating an activity.
//
// This is Mode 3 of PROPOSAL_structured_microflow_description.md. The graph it
// exists for is mendixlabs/mxcli#923's:
//
//	split1 : true → split2      false → merge1
//	split2 : true → merge1      false → merge2
//	merge1 → log → merge2
//
// `log` runs when control reaches merge1, which is `¬c1 ∨ (c1 ∧ c2)` — and no
// nesting of if/then/else expresses that without emitting `log` twice. Folding
// the guards does, and Böhm-Jacopini says folding is available exactly when the
// overlap is a shared suffix (one entry).
//
// # Why folding is sound here
//
// Two preconditions, both from the proposal, and neither optional:
//
//  1. **Short-circuit.** The folded form may evaluate a condition the original
//     skipped. MEASURED on Mendix 11.14.0 (fixtures
//     mdl-examples/bug-tests/923-short-circuit-semantics{,.test}.mdl): both
//     `and` and `or` short-circuit, so `¬c1 ∨ c2` does not evaluate `c2` when
//     `c1` is false, exactly as the original graph did not. Had they been eager,
//     folding would introduce an error the original avoided and this package
//     could not exist in this form.
//
//  2. **Purity and ordering.** Re-evaluating a Mendix split condition is safe
//     (they are pure expressions), but only if nothing else in the folded region
//     runs. If a branch performs a `create object` before reaching the shared
//     suffix, folding moves a side effect. RegionCondition therefore REFUSES a
//     region containing anything but splits and merges, rather than folding and
//     hoping.
//
// The package is dependency-light on purpose (model only): it is a pure
// function of a graph description, so it can be tested without a project, a
// describer, or a runtime.
package microflownorm

import (
	"sort"
	"strings"
)

// Formula is a boolean combination of opaque condition atoms.
//
// An atom is a Mendix expression as the describer already renders it — this
// package never parses one. That keeps the folding independent of expression
// syntax: it reasons about the SHAPE of the guard, and treats the guard's text
// as a symbol it may only copy, negate, and compare for equality.
type Formula interface {
	isFormula()
	// String renders the formula as an MDL condition expression.
	String() string
}

// Lit is a constant. It appears from folding, not from any graph node.
type Lit bool

// Atom is one split's condition, verbatim as the describer renders it.
type Atom string

// Not negates.
type Not struct{ X Formula }

// And is an n-ary conjunction.
type And struct{ Xs []Formula }

// Or is an n-ary disjunction.
type Or struct{ Xs []Formula }

func (Lit) isFormula()  {}
func (Atom) isFormula() {}
func (Not) isFormula()  {}
func (And) isFormula()  {}
func (Or) isFormula()   {}

func (l Lit) String() string {
	if bool(l) {
		return "true"
	}
	return "false"
}

func (a Atom) String() string { return string(a) }

// String renders a negation as Mendix's `not(...)`, and collapses the double
// negation folding routinely produces (a false branch of a false branch).
func (n Not) String() string {
	if inner, ok := n.X.(Not); ok {
		return inner.X.String()
	}
	return "not(" + parenthesize(n.X) + ")"
}

func (a And) String() string { return join(a.Xs, " and ") }
func (o Or) String() string  { return join(o.Xs, " or ") }

func join(xs []Formula, sep string) string {
	if len(xs) == 0 {
		// An empty conjunction is true and an empty disjunction is false, but
		// Simplify never produces either — this is belt and braces.
		if sep == " and " {
			return "true"
		}
		return "false"
	}
	if len(xs) == 1 {
		return xs[0].String()
	}
	parts := make([]string, 0, len(xs))
	for _, x := range xs {
		parts = append(parts, parenthesize(x))
	}
	return strings.Join(parts, sep)
}

// parenthesize brackets a term only when it could otherwise re-associate.
// An atom is copied verbatim: it is the describer's own rendering and may
// already carry whatever brackets it needs.
func parenthesize(f Formula) string {
	switch f.(type) {
	case And, Or:
		return "(" + f.String() + ")"
	default:
		return f.String()
	}
}

// key is a canonical form used for equality and de-duplication. Two formulas
// with the same key are the same formula; the ordering it induces is arbitrary
// but stable, which is what keeps the rendered output deterministic.
func key(f Formula) string {
	switch v := f.(type) {
	case Lit:
		return "L" + v.String()
	case Atom:
		return "A" + string(v)
	case Not:
		return "N(" + key(v.X) + ")"
	case And:
		return "&(" + joinKeys(v.Xs) + ")"
	case Or:
		return "|(" + joinKeys(v.Xs) + ")"
	}
	return "?"
}

func joinKeys(xs []Formula) string {
	ks := make([]string, 0, len(xs))
	for _, x := range xs {
		ks = append(ks, key(x))
	}
	sort.Strings(ks)
	return strings.Join(ks, ",")
}

// negate returns the negation, pushing it through a literal or a double Not so
// the result stays readable rather than accumulating wrappers.
func negate(f Formula) Formula {
	switch v := f.(type) {
	case Lit:
		return Lit(!bool(v))
	case Not:
		return v.X
	}
	return Not{X: f}
}
