// SPDX-License-Identifier: Apache-2.0

package microflownorm

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
)

// eval interprets a formula under an assignment of its atoms. Test-only: the
// package never evaluates a Mendix expression, it only rearranges guards.
func eval(f Formula, env map[string]bool) bool {
	switch v := f.(type) {
	case Lit:
		return bool(v)
	case Atom:
		return env[string(v)]
	case Not:
		return !eval(v.X, env)
	case And:
		for _, x := range v.Xs {
			if !eval(x, env) {
				return false
			}
		}
		return true
	case Or:
		for _, x := range v.Xs {
			if eval(x, env) {
				return true
			}
		}
		return false
	}
	panic("unknown formula")
}

func atomsOf(f Formula, into map[string]bool) {
	switch v := f.(type) {
	case Atom:
		into[string(v)] = true
	case Not:
		atomsOf(v.X, into)
	case And:
		for _, x := range v.Xs {
			atomsOf(x, into)
		}
	case Or:
		for _, x := range v.Xs {
			atomsOf(x, into)
		}
	}
}

// assertEquivalent enumerates every assignment of the atoms and fails on the
// first one where the two formulas disagree.
func assertEquivalent(t *testing.T, a, b Formula) {
	t.Helper()
	set := map[string]bool{}
	atomsOf(a, set)
	atomsOf(b, set)
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)
	if len(names) > 16 {
		t.Fatalf("too many atoms to enumerate: %d", len(names))
	}
	for mask := 0; mask < 1<<len(names); mask++ {
		env := map[string]bool{}
		for i, n := range names {
			env[n] = mask&(1<<i) != 0
		}
		if eval(a, env) != eval(b, env) {
			t.Fatalf("simplification changed meaning at %v:\n  before %s\n  after  %s", env, a.String(), b.String())
		}
	}
}

// THE control for the whole simplifier. A rewrite rule that is merely
// plausible produces a pretty, wrong guard — which is #923 again, from the
// other direction. Rather than trusting each rule's algebra, generate formulas
// and check the truth tables agree.
func TestSimplify_PreservesMeaningOnRandomFormulas(t *testing.T) {
	rng := rand.New(rand.NewSource(20260913))
	atoms := []Formula{Atom("a"), Atom("b"), Atom("c"), Atom("d")}

	var gen func(depth int) Formula
	gen = func(depth int) Formula {
		if depth == 0 || rng.Intn(4) == 0 {
			if rng.Intn(12) == 0 {
				return Lit(rng.Intn(2) == 0)
			}
			return atoms[rng.Intn(len(atoms))]
		}
		switch rng.Intn(3) {
		case 0:
			return Not{X: gen(depth - 1)}
		case 1:
			n := 2 + rng.Intn(2)
			xs := make([]Formula, n)
			for i := range xs {
				xs[i] = gen(depth - 1)
			}
			return And{Xs: xs}
		default:
			n := 2 + rng.Intn(2)
			xs := make([]Formula, n)
			for i := range xs {
				xs[i] = gen(depth - 1)
			}
			return Or{Xs: xs}
		}
	}

	for i := 0; i < 3000; i++ {
		f := gen(4)
		assertEquivalent(t, f, Simplify(f))
	}
}

// DESCRIBE has to be a fixed point, so simplifying an already-simplified guard
// must change nothing. Without this, re-describing an unchanged microflow could
// produce a diff.
func TestSimplify_IsIdempotent(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	atoms := []Formula{Atom("a"), Atom("b"), Atom("c")}
	var gen func(depth int) Formula
	gen = func(depth int) Formula {
		if depth == 0 || rng.Intn(3) == 0 {
			return atoms[rng.Intn(len(atoms))]
		}
		switch rng.Intn(3) {
		case 0:
			return Not{X: gen(depth - 1)}
		case 1:
			return And{Xs: []Formula{gen(depth - 1), gen(depth - 1)}}
		default:
			return Or{Xs: []Formula{gen(depth - 1), gen(depth - 1)}}
		}
	}
	for i := 0; i < 1000; i++ {
		once := Simplify(gen(4))
		twice := Simplify(once)
		if once.String() != twice.String() {
			t.Fatalf("not idempotent:\n  once  %s\n  twice %s", once.String(), twice.String())
		}
	}
}

// The absorption laws stated directly, so a failure names the rule rather than
// a random formula.
func TestSimplify_AbsorptionLaws(t *testing.T) {
	a, b := Atom("a"), Atom("b")
	cases := []struct {
		name string
		in   Formula
		want string
	}{
		{"a or (a and b) = a", Or{Xs: []Formula{a, And{Xs: []Formula{a, b}}}}, "a"},
		{"not(a) or (a and b) = not(a) or b", Or{Xs: []Formula{Not{X: a}, And{Xs: []Formula{a, b}}}}, "b or not(a)"},
		{"a and (a or b) = a", And{Xs: []Formula{a, Or{Xs: []Formula{a, b}}}}, "a"},
		{"a or not(a) = true", Or{Xs: []Formula{a, Not{X: a}}}, "true"},
		{"a and not(a) = false", And{Xs: []Formula{a, Not{X: a}}}, "false"},
		{"a or false = a", Or{Xs: []Formula{a, Lit(false)}}, "a"},
		{"a and true = a", And{Xs: []Formula{a, Lit(true)}}, "a"},
		{"not(not(a)) = a", Not{X: Not{X: a}}, "a"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Simplify(c.in)
			if got.String() != c.want {
				t.Errorf("got %q, want %q", got.String(), c.want)
			}
			assertEquivalent(t, c.in, got)
		})
	}
}

var _ = fmt.Sprintf
