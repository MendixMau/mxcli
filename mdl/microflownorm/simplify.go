// SPDX-License-Identifier: Apache-2.0

package microflownorm

import "sort"

// Simplify reduces a folded guard to something a person would have written.
//
// It is deliberately a small set of sound rewrites rather than a general
// boolean minimiser. Every rule below preserves meaning in both directions, so
// the worst outcome of a rule not firing is a verbose condition — never a wrong
// one. That asymmetry is the whole design: a describer that renders an ugly but
// correct guard is a cosmetic problem, one that renders a pretty wrong guard is
// #923 again.
//
// The rule that earns its place is absorption-with-negation,
// `¬a ∨ (a ∧ b) ≡ ¬a ∨ b`. It is what takes the reporter's graph from
// `¬c1 ∨ (c1 ∧ c2)` — which is what walking the graph produces — to the
// `¬c1 ∨ c2` the proposal promises.
//
// Simplify is idempotent: simplifying twice gives the same formula, which is
// what lets DESCRIBE be a fixed point.
func Simplify(f Formula) Formula {
	for i := 0; i < 10; i++ { // converges in one or two passes; the bound is a guard against a rule cycling
		next := simplifyOnce(f)
		if key(next) == key(f) {
			return next
		}
		f = next
	}
	return f
}

func simplifyOnce(f Formula) Formula {
	switch v := f.(type) {
	case Not:
		return negate(simplifyOnce(v.X))
	case And:
		return simplifyAnd(v)
	case Or:
		return simplifyOr(v)
	}
	return f
}

// flatten pulls the children of a same-kind node up into the parent, so
// `a ∨ (b ∨ c)` is one three-term disjunction and the pairwise rules below see
// every pair.
func flatten(xs []Formula, isOr bool) []Formula {
	var out []Formula
	for _, x := range xs {
		x = simplifyOnce(x)
		if isOr {
			if o, ok := x.(Or); ok {
				out = append(out, o.Xs...)
				continue
			}
		} else {
			if a, ok := x.(And); ok {
				out = append(out, a.Xs...)
				continue
			}
		}
		out = append(out, x)
	}
	return out
}

// dedupe removes repeated terms (`a ∨ a ≡ a`) and keeps a stable order so the
// rendered condition does not depend on map iteration.
func dedupe(xs []Formula) []Formula {
	seen := map[string]bool{}
	var out []Formula
	for _, x := range xs {
		k := key(x)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, x)
	}
	sort.SliceStable(out, func(i, j int) bool { return key(out[i]) < key(out[j]) })
	return out
}

func simplifyOr(o Or) Formula {
	xs := dedupe(flatten(o.Xs, true))

	// Constants: `x ∨ true ≡ true`, and false terms drop out.
	var kept []Formula
	for _, x := range xs {
		if l, ok := x.(Lit); ok {
			if bool(l) {
				return Lit(true)
			}
			continue
		}
		kept = append(kept, x)
	}
	xs = kept

	// Complementary pair: `a ∨ ¬a ≡ true`.
	present := map[string]bool{}
	for _, x := range xs {
		present[key(x)] = true
	}
	for _, x := range xs {
		if present[key(negate(x))] {
			return Lit(true)
		}
	}

	xs = absorb(xs, true)

	switch len(xs) {
	case 0:
		return Lit(false)
	case 1:
		return xs[0]
	}
	return Or{Xs: xs}
}

func simplifyAnd(a And) Formula {
	xs := dedupe(flatten(a.Xs, false))

	var kept []Formula
	for _, x := range xs {
		if l, ok := x.(Lit); ok {
			if !bool(l) {
				return Lit(false)
			}
			continue
		}
		kept = append(kept, x)
	}
	xs = kept

	present := map[string]bool{}
	for _, x := range xs {
		present[key(x)] = true
	}
	for _, x := range xs {
		if present[key(negate(x))] {
			return Lit(false)
		}
	}

	xs = absorb(xs, false)

	switch len(xs) {
	case 0:
		return Lit(true)
	case 1:
		return xs[0]
	}
	return And{Xs: xs}
}

// absorb applies the two absorption laws to every pair of terms.
//
// For a disjunction (isOr), with `t` a term and `c` a conjunction:
//
//	t ∨ (t ∧ …)   ≡ t          -- plain absorption, drop the conjunction
//	¬t ∨ (t ∧ b)  ≡ ¬t ∨ b     -- absorption with negation, drop t from inside
//
// and dually for a conjunction. The second is the one that matters: it is what
// removes the guard a path has already decided, which is precisely the
// redundancy that walking a recombinable graph introduces.
func absorb(xs []Formula, isOr bool) []Formula {
	changed := true
	for changed {
		changed = false
		for i := 0; i < len(xs) && !changed; i++ {
			for j := 0; j < len(xs); j++ {
				if i == j {
					continue
				}
				// Inside a disjunction the absorbable terms are conjunctions,
				// and dually. Passing !isOr here looks at the wrong node type
				// and quietly disables every absorption.
				inner, ok := childTerms(xs[j], isOr)
				if !ok {
					continue
				}
				ti, tneg := key(xs[i]), key(negate(xs[i]))

				// t ∨ (t ∧ …) ≡ t
				for _, c := range inner {
					if key(c) == ti {
						xs = append(xs[:j], xs[j+1:]...)
						changed = true
						break
					}
				}
				if changed {
					break
				}

				// ¬t ∨ (t ∧ b) ≡ ¬t ∨ b
				var rest []Formula
				dropped := false
				for _, c := range inner {
					if key(c) == tneg {
						dropped = true
						continue
					}
					rest = append(rest, c)
				}
				if !dropped {
					continue
				}
				var replacement Formula
				switch len(rest) {
				case 0:
					// The whole conjunction was the complement, so the term
					// contributes nothing beyond what xs[i] already covers.
					replacement = Lit(!isOr)
				case 1:
					replacement = rest[0]
				default:
					if isOr {
						replacement = And{Xs: rest}
					} else {
						replacement = Or{Xs: rest}
					}
				}
				xs[j] = replacement
				changed = true
				break
			}
		}
		if changed {
			// Re-normalise: the replacement may itself be absorbable.
			xs = dedupe(flatten(xs, isOr))
			var kept []Formula
			for _, x := range xs {
				if l, ok := x.(Lit); ok {
					if isOr && !bool(l) {
						continue
					}
					if !isOr && bool(l) {
						continue
					}
				}
				kept = append(kept, x)
			}
			xs = kept
		}
	}
	return xs
}

// childTerms returns the operands of a conjunction (wantAnd) or disjunction.
func childTerms(f Formula, wantAnd bool) ([]Formula, bool) {
	if wantAnd {
		if a, ok := f.(And); ok {
			return a.Xs, true
		}
		return nil, false
	}
	if o, ok := f.(Or); ok {
		return o.Xs, true
	}
	return nil, false
}
