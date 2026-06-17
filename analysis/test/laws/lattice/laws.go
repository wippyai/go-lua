package latticelaws

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
)

// reporter is the minimal subset of testing.TB the harness uses. reporter
// satisfies this automatically; tests can substitute a capture-only stand-in
// to verify the harness's own diagnostics without bringing in reporter.
type reporter interface {
	Helper()
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
}

// LawSuite drives the standard lattice laws against a domain.
//
// A test file per domain instantiates LawSuite with that domain's Lattice
// implementation and a Sample of representative elements. Calling Run
// from a Test function asserts every law and fails the test with a
// diagnostic naming the violated law and the offending element(s).
//
// Sample must include Bottom and Top, and should cover a structural
// cross-section of the domain (small/typical/large; recursive elements
// if the domain admits them). Coverage is the author's responsibility;
// the harness only verifies the laws against the provided sample.
type LawSuite[T any] struct {
	// Name identifies the domain in failure messages.
	Name string

	// Domain is the abstract domain under test.
	Domain lattice.Lattice[T]

	// Sample is the set of representative elements. Must include the
	// implementation's Bottom and Top; should cover structural variety.
	Sample []T

	// Format optionally renders an element for diagnostic messages.
	// If nil, fmt.Sprintf("%+v", e) is used.
	Format func(T) string
}

// Run asserts every lattice law against Sample and reports per-law,
// per-element violations through t.Errorf. The test does not abort on
// the first failure — running all laws gives a richer picture of how a
// domain is broken than a single fail-fast.
//
// Meet-side laws (idempotency, commutativity, associativity, lower-bound,
// absorption) are gated on Domain.Meet != nil; a forward-only domain that
// leaves Meet nil exercises only the join-side, partial-order, and widening
// laws.
func (s LawSuite[T]) Run(t reporter) {
	t.Helper()
	if s.Name == "" {
		t.Fatalf("latticelaws.LawSuite: Name is required")
	}
	if !s.domainValid() {
		t.Fatalf("latticelaws.LawSuite[%s]: Domain is required (all function fields must be set)", s.Name)
	}
	if len(s.Sample) < 2 {
		t.Fatalf("latticelaws.LawSuite[%s]: Sample must contain at least Bottom and Top", s.Name)
	}

	s.checkBottomTop(t)
	s.checkPartialOrderReflexive(t)
	s.checkPartialOrderAntisymmetric(t)
	s.checkPartialOrderTransitive(t)
	s.checkJoinIdempotent(t)
	s.checkJoinCommutative(t)
	s.checkJoinAssociative(t)
	s.checkJoinUpperBound(t)
	s.checkJoinLeastUpperBound(t)
	s.checkMeetIdempotent(t)
	s.checkMeetCommutative(t)
	s.checkMeetAssociative(t)
	s.checkMeetLowerBound(t)
	s.checkAbsorption(t)
	s.checkWideningOverApproximates(t)
	s.checkWideningChainTerminates(t)
}

func (s LawSuite[T]) domainValid() bool {
	d := s.Domain
	// Meet is optional: forward-only domains (e.g. AbstractValue, where no
	// analyzer surface consumes a greatest lower bound) may leave it nil.
	// LawSuite skips the meet-side laws when Meet is nil; see the per-law
	// guards in checkMeet* and checkAbsorption.
	return d.Bottom != nil && d.Top != nil && d.Equal != nil && d.LessOrEq != nil &&
		d.Join != nil && d.Widen != nil
}

func (s LawSuite[T]) fmt(x T) string {
	if s.Format != nil {
		return s.Format(x)
	}
	return fmt.Sprintf("%+v", x)
}

func (s LawSuite[T]) report(t reporter, law string, msg string, args ...any) {
	t.Helper()
	t.Errorf("%s law %s: "+msg, append([]any{s.Name, law}, args...)...)
}

func (s LawSuite[T]) checkBottomTop(t reporter) {
	t.Helper()
	bot := s.Domain.Bottom()
	top := s.Domain.Top()
	for _, x := range s.Sample {
		if !s.Domain.LessOrEq(bot, x) {
			s.report(t, "Bottom", "Bottom ⊑ %s does not hold", s.fmt(x))
		}
		if !s.Domain.LessOrEq(x, top) {
			s.report(t, "Top", "%s ⊑ Top does not hold", s.fmt(x))
		}
	}
}

func (s LawSuite[T]) checkPartialOrderReflexive(t reporter) {
	t.Helper()
	for _, x := range s.Sample {
		if !s.Domain.LessOrEq(x, x) {
			s.report(t, "Reflexivity (⊑)", "%s ⊑ %s does not hold", s.fmt(x), s.fmt(x))
		}
		if !s.Domain.Equal(x, x) {
			s.report(t, "Reflexivity (=)", "%s = %s does not hold", s.fmt(x), s.fmt(x))
		}
	}
}

func (s LawSuite[T]) checkPartialOrderAntisymmetric(t reporter) {
	t.Helper()
	for _, a := range s.Sample {
		for _, b := range s.Sample {
			if s.Domain.LessOrEq(a, b) && s.Domain.LessOrEq(b, a) && !s.Domain.Equal(a, b) {
				s.report(t, "Antisymmetry", "%s ⊑ %s and %s ⊑ %s but not equal", s.fmt(a), s.fmt(b), s.fmt(b), s.fmt(a))
			}
		}
	}
}

func (s LawSuite[T]) checkPartialOrderTransitive(t reporter) {
	t.Helper()
	for _, a := range s.Sample {
		for _, b := range s.Sample {
			if !s.Domain.LessOrEq(a, b) {
				continue
			}
			for _, c := range s.Sample {
				if s.Domain.LessOrEq(b, c) && !s.Domain.LessOrEq(a, c) {
					s.report(t, "Transitivity", "%s ⊑ %s ⊑ %s but %s ⊑ %s fails", s.fmt(a), s.fmt(b), s.fmt(c), s.fmt(a), s.fmt(c))
				}
			}
		}
	}
}

func (s LawSuite[T]) checkJoinIdempotent(t reporter) {
	t.Helper()
	for _, x := range s.Sample {
		j := s.Domain.Join(x, x)
		if !s.Domain.Equal(j, x) {
			s.report(t, "Join idempotency", "Join(%s, %s) = %s, expected %s", s.fmt(x), s.fmt(x), s.fmt(j), s.fmt(x))
		}
	}
}

func (s LawSuite[T]) checkJoinCommutative(t reporter) {
	t.Helper()
	for _, a := range s.Sample {
		for _, b := range s.Sample {
			ab := s.Domain.Join(a, b)
			ba := s.Domain.Join(b, a)
			if !s.Domain.Equal(ab, ba) {
				s.report(t, "Join commutativity", "Join(%s, %s) = %s ≠ Join(%s, %s) = %s", s.fmt(a), s.fmt(b), s.fmt(ab), s.fmt(b), s.fmt(a), s.fmt(ba))
			}
		}
	}
}

func (s LawSuite[T]) checkJoinAssociative(t reporter) {
	t.Helper()
	// O(n³); cap sample to avoid combinatorial blowup on large suites.
	cap := len(s.Sample)
	if cap > 12 {
		cap = 12
	}
	sub := s.Sample[:cap]
	for _, a := range sub {
		for _, b := range sub {
			for _, c := range sub {
				left := s.Domain.Join(s.Domain.Join(a, b), c)
				right := s.Domain.Join(a, s.Domain.Join(b, c))
				if !s.Domain.Equal(left, right) {
					s.report(t, "Join associativity", "Join(Join(%s,%s),%s)=%s ≠ Join(%s,Join(%s,%s))=%s", s.fmt(a), s.fmt(b), s.fmt(c), s.fmt(left), s.fmt(a), s.fmt(b), s.fmt(c), s.fmt(right))
				}
			}
		}
	}
}

func (s LawSuite[T]) checkJoinUpperBound(t reporter) {
	s.checkUpperBound(t, s.Domain.Join, "Join", "Join upper-bound", "Join upper-bound")
}

// checkUpperBound verifies that op(a,b) is an upper bound of both operands,
// reporting failures of a under labelA and of b under labelB.
func (s LawSuite[T]) checkUpperBound(t reporter, op func(T, T) T, opName, labelA, labelB string) {
	t.Helper()
	for _, a := range s.Sample {
		for _, b := range s.Sample {
			r := op(a, b)
			if !s.Domain.LessOrEq(a, r) {
				s.report(t, labelA, "%s ⊑ "+opName+"(%s,%s)=%s fails", s.fmt(a), s.fmt(a), s.fmt(b), s.fmt(r))
			}
			if !s.Domain.LessOrEq(b, r) {
				s.report(t, labelB, "%s ⊑ "+opName+"(%s,%s)=%s fails", s.fmt(b), s.fmt(a), s.fmt(b), s.fmt(r))
			}
		}
	}
}

func (s LawSuite[T]) checkJoinLeastUpperBound(t reporter) {
	t.Helper()
	for _, a := range s.Sample {
		for _, b := range s.Sample {
			j := s.Domain.Join(a, b)
			for _, c := range s.Sample {
				if s.Domain.LessOrEq(a, c) && s.Domain.LessOrEq(b, c) && !s.Domain.LessOrEq(j, c) {
					s.report(t, "Join least-upper-bound", "%s ⊑ %s and %s ⊑ %s but Join(%s,%s)=%s ⊑ %s fails", s.fmt(a), s.fmt(c), s.fmt(b), s.fmt(c), s.fmt(a), s.fmt(b), s.fmt(j), s.fmt(c))
				}
			}
		}
	}
}

func (s LawSuite[T]) checkMeetIdempotent(t reporter) {
	t.Helper()
	if s.Domain.Meet == nil {
		return
	}
	for _, x := range s.Sample {
		m := s.Domain.Meet(x, x)
		if !s.Domain.Equal(m, x) {
			s.report(t, "Meet idempotency", "Meet(%s, %s) = %s, expected %s", s.fmt(x), s.fmt(x), s.fmt(m), s.fmt(x))
		}
	}
}

func (s LawSuite[T]) checkMeetCommutative(t reporter) {
	t.Helper()
	if s.Domain.Meet == nil {
		return
	}
	for _, a := range s.Sample {
		for _, b := range s.Sample {
			ab := s.Domain.Meet(a, b)
			ba := s.Domain.Meet(b, a)
			if !s.Domain.Equal(ab, ba) {
				s.report(t, "Meet commutativity", "Meet(%s, %s) = %s ≠ Meet(%s, %s) = %s", s.fmt(a), s.fmt(b), s.fmt(ab), s.fmt(b), s.fmt(a), s.fmt(ba))
			}
		}
	}
}

func (s LawSuite[T]) checkMeetAssociative(t reporter) {
	t.Helper()
	if s.Domain.Meet == nil {
		return
	}
	cap := len(s.Sample)
	if cap > 12 {
		cap = 12
	}
	sub := s.Sample[:cap]
	for _, a := range sub {
		for _, b := range sub {
			for _, c := range sub {
				left := s.Domain.Meet(s.Domain.Meet(a, b), c)
				right := s.Domain.Meet(a, s.Domain.Meet(b, c))
				if !s.Domain.Equal(left, right) {
					s.report(t, "Meet associativity", "Meet(Meet(%s,%s),%s)=%s ≠ Meet(%s,Meet(%s,%s))=%s", s.fmt(a), s.fmt(b), s.fmt(c), s.fmt(left), s.fmt(a), s.fmt(b), s.fmt(c), s.fmt(right))
				}
			}
		}
	}
}

func (s LawSuite[T]) checkMeetLowerBound(t reporter) {
	t.Helper()
	if s.Domain.Meet == nil {
		return
	}
	for _, a := range s.Sample {
		for _, b := range s.Sample {
			m := s.Domain.Meet(a, b)
			if !s.Domain.LessOrEq(m, a) {
				s.report(t, "Meet lower-bound", "Meet(%s,%s)=%s ⊑ %s fails", s.fmt(a), s.fmt(b), s.fmt(m), s.fmt(a))
			}
			if !s.Domain.LessOrEq(m, b) {
				s.report(t, "Meet lower-bound", "Meet(%s,%s)=%s ⊑ %s fails", s.fmt(a), s.fmt(b), s.fmt(m), s.fmt(b))
			}
		}
	}
}

func (s LawSuite[T]) checkAbsorption(t reporter) {
	t.Helper()
	// Absorption requires both Join and Meet. Skip when Meet is absent.
	if s.Domain.Meet == nil {
		return
	}
	for _, a := range s.Sample {
		for _, b := range s.Sample {
			// a ⊔ (a ⊓ b) = a
			left := s.Domain.Join(a, s.Domain.Meet(a, b))
			if !s.Domain.Equal(left, a) {
				s.report(t, "Absorption (Join over Meet)", "Join(%s, Meet(%s,%s)) = %s, expected %s", s.fmt(a), s.fmt(a), s.fmt(b), s.fmt(left), s.fmt(a))
			}
			// a ⊓ (a ⊔ b) = a
			right := s.Domain.Meet(a, s.Domain.Join(a, b))
			if !s.Domain.Equal(right, a) {
				s.report(t, "Absorption (Meet over Join)", "Meet(%s, Join(%s,%s)) = %s, expected %s", s.fmt(a), s.fmt(a), s.fmt(b), s.fmt(right), s.fmt(a))
			}
		}
	}
}

func (s LawSuite[T]) checkWideningOverApproximates(t reporter) {
	s.checkUpperBound(t, s.Domain.Widen, "Widen", "Widen over-approximates prev", "Widen over-approximates next")
}

// chainTerminationBound caps how many widening iterations we permit before
// declaring the chain non-terminating. A correct widening on any domain
// realized in this checker must stabilize well under this bound for the
// ascending chains driven by sample inputs; exceeding it indicates a
// missing widening operator. (Cousot widening guarantees termination but
// not a fixed worst-case bound; this is a generous engineering limit.)
const chainTerminationBound = 256

func (s LawSuite[T]) checkWideningChainTerminates(t reporter) {
	t.Helper()
	// For every pair (seed, growth), simulate the ascending chain
	//   sᵢ₊₁ = Widen(sᵢ, Join(sᵢ, growth))
	// and verify it stabilizes within chainTerminationBound iterations.
	for _, seed := range s.Sample {
		for _, growth := range s.Sample {
			cur := seed
			stable := false
			for i := 0; i < chainTerminationBound; i++ {
				next := s.Domain.Widen(cur, s.Domain.Join(cur, growth))
				if s.Domain.Equal(next, cur) {
					stable = true
					break
				}
				cur = next
			}
			if !stable {
				s.report(t, "Widen ascending-chain termination", "chain from seed=%s growth=%s did not stabilize within %d iterations (final=%s) — domain lacks a proper widening operator", s.fmt(seed), s.fmt(growth), chainTerminationBound, s.fmt(cur))
			}
		}
	}
}

// Note: a "Meet representational-bound" timing check existed in rev 1 of the
// lattice harness. That bound conflicts with the design premise: widening
// happens at the worklist iterate, not inside Meet/And. The check was removed
// as part of the Condition close.
