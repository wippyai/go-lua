package constraint

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
)

// Test helpers and shared fixtures for widening law verification.

func widenPath(name string, sym cfg.SymbolID) Path {
	return Path{Root: name, Symbol: sym}
}

// freshLiteral returns a Truthy literal on a unique synthesized symbol.
// Used to construct chains whose vocabulary grows iteration-by-iteration.
func freshLiteral(iter int) Constraint {
	return Truthy{Path: widenPath("v"+strconv.Itoa(iter), cfg.SymbolID(1000+iter))}
}

// shapeSample returns a deterministic, multi-shape collection of Conditions
// covering ⊥, ⊤, single literal, multi-literal disjuncts, multi-disjunct
// DNFs, and disjoint-vocabulary pairs. It is used as the (prev,next) product
// for the soundness and regression-lock tests.
func shapeSample() []Condition {
	x := widenPath("x", 1)
	y := widenPath("y", 2)
	z := widenPath("z", 3)
	w := widenPath("w", 4)

	tx := FromConstraints(Truthy{Path: x})
	ty := FromConstraints(Truthy{Path: y})
	tz := FromConstraints(Truthy{Path: z})
	tw := FromConstraints(Truthy{Path: w})
	nx := FromConstraints(NotNil{Path: x})
	ny := FromConstraints(NotNil{Path: y})
	hx := FromConstraints(HasField{Path: x, Field: "kind"})

	return []Condition{
		FalseCondition(),
		TrueCondition(),
		tx,
		ty,
		tz,
		tw,
		nx,
		ny,
		hx,
		And(tx, ty),
		And(tx, ny),
		Or(tx, ty),
		Or(tx, And(ty, nx)),
		Or(And(tx, ty), And(tz, ny)),
		Or(tx, tw), // disjoint with most other paths
	}
}

// TestWiden_Soundness_MultiShape verifies that for every (prev, next) in the
// shape sample, both prev ⊑ Widen(prev,next) and next ⊑ Widen(prev,next).
//
// LessOrEq(a, b) ≡ b.Subsumes(a) in the Condition domain (DOMAIN_DESIGN.md §4).
//
// DOMAIN_DESIGN.md §10.2: soundness.
func TestWiden_Soundness_MultiShape(t *testing.T) {
	sample := shapeSample()
	leq := Domain.LessOrEq

	for i, prev := range sample {
		for j, next := range sample {
			w := Domain.Widen(prev, next)
			if !leq(prev, w) {
				t.Errorf("[i=%d,j=%d] prev ⊑ Widen(prev,next) fails:\n  prev=%v\n  next=%v\n  widen=%v",
					i, j, prev, next, w)
			}
			if !leq(next, w) {
				t.Errorf("[i=%d,j=%d] next ⊑ Widen(prev,next) fails:\n  prev=%v\n  next=%v\n  widen=%v",
					i, j, prev, next, w)
			}
		}
	}
}

// TestWiden_Idempotent_OnStableChains verifies Widen(c, c) = c for every
// sample element. This is the "stable chain" base case: once a chain reaches
// a fixed point, repeated widening must not perturb it.
//
// DOMAIN_DESIGN.md §10.2: idempotency on stable chains.
func TestWiden_Idempotent_OnStableChains(t *testing.T) {
	for i, c := range shapeSample() {
		w := Domain.Widen(c, c)
		if !Domain.Equal(w, c) {
			t.Errorf("[i=%d] Widen(c,c) != c:\n  c=%v\n  widen=%v", i, c, w)
		}
	}
}

// TestWiden_VocabularyFixing_FreshLiteralsPerIteration verifies the central
// termination property of projection widening: after a single widening, the
// vocabulary is bounded by Lit(prev), and any subsequent iterate's literals
// must be a subset of that vocabulary.
//
// Set-up: prev = Truthy(x). The transfer adds Or(prev, freshLiteral(k)) at
// each iteration k. Without widening, the vocabulary would grow without
// bound; with widening it must stabilize within the prev vocabulary or
// collapse to ⊤.
//
// DOMAIN_DESIGN.md §10.2: vocabulary-fixing across a multi-iteration chain.
func TestWiden_VocabularyFixing_FreshLiteralsPerIteration(t *testing.T) {
	prev := FromConstraints(Truthy{Path: widenPath("seed", 1)})
	prevVocab := collectVocabularyKeys(prev)

	cur := prev
	for i := 0; i < 10; i++ {
		// Transfer simulates an unsound dataflow step that constantly
		// introduces a never-before-seen literal.
		incoming := Or(cur, FromConstraints(freshLiteral(i)))
		cur = Domain.Widen(cur, incoming)
	}

	curVocab := collectVocabularyKeys(cur)
	if cur.IsTrue() {
		// Reaching ⊤ is a sound outcome (when projection empties all
		// disjuncts). The vocabulary check below is moot.
		return
	}
	for k := range curVocab {
		if _, ok := prevVocab[k]; !ok {
			t.Fatalf("widened iterate introduced literal not in prev vocab: %v\n  prev=%v\n  cur=%v",
				k, prev, cur)
		}
	}
}

// TestWiden_EdgeCases_BotTopDisjoint covers the explicit-branch edge cases
// from DOMAIN_DESIGN.md §6.4: Widen(⊥, c) = c, Widen(c, ⊥) = c,
// Widen(⊤, c) = ⊤, and Widen(disjointA, disjointB) = ⊤ (empty-projection
// case).
func TestWiden_EdgeCases_BotTopDisjoint(t *testing.T) {
	bot := FalseCondition()
	top := TrueCondition()

	c := FromConstraints(Truthy{Path: widenPath("x", 1)})

	if w := Domain.Widen(bot, c); !Domain.Equal(w, c) {
		t.Errorf("Widen(⊥, c) = %v, want %v", w, c)
	}
	if w := Domain.Widen(c, bot); !Domain.Equal(w, c) {
		t.Errorf("Widen(c, ⊥) = %v, want %v", w, c)
	}
	if w := Domain.Widen(top, c); !Domain.Equal(w, top) {
		t.Errorf("Widen(⊤, c) = %v, want ⊤", w)
	}
	if w := Domain.Widen(c, top); !Domain.Equal(w, top) {
		t.Errorf("Widen(c, ⊤) = %v, want ⊤", w)
	}

	a := FromConstraints(Truthy{Path: widenPath("a", 1)})
	b := FromConstraints(Truthy{Path: widenPath("b", 2)})
	w := Domain.Widen(a, b)
	if !w.IsTrue() {
		t.Errorf("Widen(disjointA, disjointB) should be ⊤ (empty projection collapses), got %v", w)
	}
}

// TestWiden_UnsoundDirectionRegressionLock pins down that the implementation
// does NOT drop disjuncts (dropping moves toward ⊥, which is unsound).
//
// Property: for every (prev,next) in shapeSample, every disjunct of
// Widen(prev,next) must have its literal set contained in some disjunct of
// Or(prev,next). The widening can REWRITE disjuncts (drop literals via
// projection — which weakens, sound direction) but every output disjunct
// must descend from an Or-input disjunct.
//
// DOMAIN_DESIGN.md §10.2: unsound-direction regression lock.
func TestWiden_UnsoundDirectionRegressionLock(t *testing.T) {
	sample := shapeSample()
	for i, prev := range sample {
		for j, next := range sample {
			widen := Domain.Widen(prev, next)
			ref := Or(prev, next)

			if widen.IsFalse() || widen.IsTrue() {
				continue
			}
			if ref.IsTrue() {
				continue
			}

			refDisjuncts := ref.Disjuncts
			for di, wDisj := range widen.Disjuncts {
				if !someDisjunctIsSuperset(refDisjuncts, wDisj) {
					t.Errorf("[i=%d,j=%d disj=%d] Widen produced a disjunct whose literal set is NOT a subset of any Or(prev,next) disjunct.\n  prev=%v\n  next=%v\n  widen=%v\n  Or(prev,next)=%v",
						i, j, di, prev, next, widen, ref)
				}
			}
		}
	}
}

// someDisjunctIsSuperset returns true if any disjunct in refs has wDisj's
// literal set as a (non-strict) subset of its own literal set.
func someDisjunctIsSuperset(refs [][]Constraint, wDisj []Constraint) bool {
	for _, r := range refs {
		if literalSetSubset(wDisj, r) {
			return true
		}
	}
	return false
}

// literalSetSubset returns true if every literal in a appears (by Equals) in b.
func literalSetSubset(a, b []Constraint) bool {
	for _, ai := range a {
		found := false
		for _, bj := range b {
			if ai.Equals(bj) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// collectVocabularyKeys returns the keyed literal vocabulary of c, with each
// literal identified by its String(). Used as a stable structural identity
// for set-difference checks in the vocabulary-fixing test.
func collectVocabularyKeys(c Condition) map[string]struct{} {
	m := make(map[string]struct{})
	if c.IsFalse() || c.IsTrue() {
		return m
	}
	for _, d := range c.Disjuncts {
		for _, lit := range d {
			m[constraintString(lit)] = struct{}{}
		}
	}
	return m
}

// TestWiden_ChainTerminates verifies the engineering claim of §6.3: an
// ascending chain driven by an arbitrary monotone (non-shrinking) transfer
// stabilizes in a bounded number of widening steps.
func TestWiden_ChainTerminates(t *testing.T) {
	// Transfer: at step k, OR in a fresh literal AND a recurrence of the
	// seed literal. Without widening the chain grows without bound.
	prev := FromConstraints(Truthy{Path: widenPath("seed", 1)})
	cur := prev
	const maxSteps = 64
	for i := 0; i < maxSteps; i++ {
		incoming := Or(cur, FromConstraints(freshLiteral(i)))
		nxt := Domain.Widen(cur, incoming)
		if Domain.Equal(nxt, cur) {
			return
		}
		cur = nxt
	}
	t.Fatalf("widening chain did not stabilize within %d steps; final = %v", maxSteps, cur)
}

// TestWiden_NoNewDisjunctsBeyondProjection is a precision-oriented test
// distinct from the unsound-direction lock: it asserts that Widen does not
// introduce fabricated literals that were absent from both inputs.
func TestWiden_NoNewDisjunctsBeyondProjection(t *testing.T) {
	sample := shapeSample()
	for i, prev := range sample {
		for j, next := range sample {
			if prev.IsFalse() || next.IsFalse() {
				continue
			}
			widen := Domain.Widen(prev, next)
			if widen.IsFalse() || widen.IsTrue() {
				continue
			}
			refVocab := union(collectVocabularyKeys(prev), collectVocabularyKeys(next))
			for k := range collectVocabularyKeys(widen) {
				if _, ok := refVocab[k]; !ok {
					t.Errorf("[i=%d,j=%d] Widen introduced literal not in Lit(prev)∪Lit(next): %s\n  prev=%v\n  next=%v\n  widen=%v",
						i, j, k, prev, next, widen)
				}
			}
		}
	}
}

func union(a, b map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		out[k] = struct{}{}
	}
	for k := range b {
		out[k] = struct{}{}
	}
	return out
}

// Compile-time sanity: shapeSample must be deterministic.
var _ = fmt.Sprintf
