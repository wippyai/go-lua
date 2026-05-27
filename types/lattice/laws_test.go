package lattice

import (
	"fmt"
	"strings"
	"testing"
)

// presence is a minimal 3-element lattice used to verify the law harness
// itself. It is independent of any production domain so a regression here
// signals a harness bug, not a production-domain regression.
//
//   Bottom ⊏ {Nil, NonNil} ⊏ Top (= Unknown)
//
// Nil and NonNil are incomparable.
type presence int

const (
	pBottom presence = iota
	pNil
	pNonNil
	pTop
)

func presenceEqual(a, b presence) bool { return a == b }

func presenceLessOrEq(a, b presence) bool {
	if a == pBottom || b == pTop {
		return true
	}
	return a == b
}

func presenceJoin(a, b presence) presence {
	if a == b {
		return a
	}
	if a == pBottom {
		return b
	}
	if b == pBottom {
		return a
	}
	return pTop
}

func presenceMeet(a, b presence) presence {
	if a == b {
		return a
	}
	if a == pTop {
		return b
	}
	if b == pTop {
		return a
	}
	return pBottom
}

// presenceLattice returns the canonical Lattice value for the presence domain.
// Finite height → Widen = Join is a valid widening.
func presenceLattice() Lattice[presence] {
	return Lattice[presence]{
		Bottom:   func() presence { return pBottom },
		Top:      func() presence { return pTop },
		Equal:    presenceEqual,
		LessOrEq: presenceLessOrEq,
		Join:     presenceJoin,
		Meet:     presenceMeet,
		Widen:    presenceJoin,
	}
}

func TestLawSuite_PassesOnPresenceLattice(t *testing.T) {
	suite := LawSuite[presence]{
		Name:   "presence",
		Domain: presenceLattice(),
		Sample: []presence{pBottom, pNil, pNonNil, pTop},
	}
	suite.Run(t)
}

// brokenJoinLattice violates idempotency: Join(x,x) returns Top instead of x.
// The harness must catch this with a clear diagnostic.
func brokenJoinLattice() Lattice[presence] {
	d := presenceLattice()
	d.Join = func(a, b presence) presence {
		if a == b {
			return pTop
		}
		return presenceJoin(a, b)
	}
	return d
}

// brokenLessOrEq violates antisymmetry: declares Bottom ⊑ Top and Top ⊑ Bottom.
func brokenLessOrEqLattice() Lattice[presence] {
	d := presenceLattice()
	d.LessOrEq = func(a, b presence) bool { return true }
	return d
}

// nonTerminatingDomain generates a chain that never stabilizes by returning
// (prev+1) % 256 — guaranteed to spin until the harness bound fires.
func nonTerminatingDomain() Lattice[int] {
	return Lattice[int]{
		Bottom:   func() int { return 0 },
		Top:      func() int { return 255 },
		Equal:    func(a, b int) bool { return a == b },
		LessOrEq: func(a, b int) bool { return a <= b },
		Join:     max2,
		Meet:     min2,
		Widen:    func(prev, next int) int { return (prev + 1) & 0xff },
	}
}

func max2(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestLawSuite_CatchesBrokenJoin(t *testing.T) {
	mock := &mockT{}
	suite := LawSuite[presence]{
		Name:   "broken-join",
		Domain: brokenJoinLattice(),
		Sample: []presence{pBottom, pNil, pNonNil, pTop},
	}
	suite.Run(mock)
	if !mock.hasLaw("Join idempotency") {
		t.Errorf("harness missed Join idempotency violation; got messages: %v", mock.messages)
	}
}

func TestLawSuite_CatchesBrokenLessOrEq(t *testing.T) {
	mock := &mockT{}
	suite := LawSuite[presence]{
		Name:   "broken-leq",
		Domain: brokenLessOrEqLattice(),
		Sample: []presence{pBottom, pNil, pNonNil, pTop},
	}
	suite.Run(mock)
	if !mock.hasLaw("Antisymmetry") {
		t.Errorf("harness missed antisymmetry violation; got messages: %v", mock.messages)
	}
}

func TestLawSuite_CatchesNonTerminatingWiden(t *testing.T) {
	mock := &mockT{}
	suite := LawSuite[int]{
		Name:   "non-terminating",
		Domain: nonTerminatingDomain(),
		Sample: []int{0, 1, 2, 255},
	}
	suite.Run(mock)
	if !mock.hasLaw("Widen ascending-chain termination") {
		t.Errorf("harness missed non-termination; got messages: %v", mock.messages)
	}
}

// mockT is a minimal *testing.T stand-in that captures Errorf calls so
// the harness's own tests can assert which laws fire on broken domains.
// It implements only the methods the harness calls.
type mockT struct {
	messages []string
	fatal    bool
}

func (m *mockT) Helper()                           {}
func (m *mockT) Errorf(format string, args ...any) { m.messages = append(m.messages, sprintf(format, args...)) }
func (m *mockT) Fatalf(format string, args ...any) {
	m.messages = append(m.messages, sprintf(format, args...))
	m.fatal = true
}
func (m *mockT) hasLaw(law string) bool {
	for _, msg := range m.messages {
		if strings.Contains(msg, law) {
			return true
		}
	}
	return false
}

func sprintf(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}
