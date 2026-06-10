// Package cfgmeta stores semantic facts for CFG points.
package cfgmeta

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// BranchCheckKind identifies the type of condition check in a branch.
type BranchCheckKind uint8

// Branch check kind constants represent recognizable branch patterns.
const (
	CheckNone      BranchCheckKind = iota // Complex expression, no simple constraint
	CheckTruthy                           // if x then: narrows x to truthy values
	CheckFalsy                            // if not x then: narrows x to falsy values
	CheckNil                              // x == nil: narrows x to nil
	CheckNotNil                           // x ~= nil: narrows x to non-nil
	CheckLimit                            // Numeric for loop limit (i <= n)
	CheckTypeEqual                        // type(x) == "typename": narrows to that type
	CheckTypeNot                          // type(x) ~= "typename": excludes that type
)

// BranchCheck represents a condition check in a branch fact.
type BranchCheck struct {
	Kind     BranchCheckKind
	TypeName string // Only for CheckTypeEqual/CheckTypeNot
}

// BranchFact describes a branch condition.
type BranchFact struct {
	Symbol symbol.ID
	Check  BranchCheck
}

// AssignmentFact describes an assignment target.
type AssignmentFact struct {
	Target symbol.ID
}

// CallFact describes a call target when it can be named.
type CallFact struct {
	CalleeName string
}

// LoopFact describes loop structure associated with a CFG point.
type LoopFact struct {
	Vars         []symbol.ID
	Locals       []symbol.ID
	Preheader    cfg.Point
	HasPreheader bool
}

// ScopeExitFact describes the branch origin for a copied scope-exit guard.
type ScopeExitFact struct {
	CondOrigin    cfg.Point
	HasCondOrigin bool
}

// Metadata stores semantic facts keyed by CFG point.
type Metadata struct {
	branches    map[cfg.Point]BranchFact
	assignments map[cfg.Point]AssignmentFact
	calls       map[cfg.Point]CallFact
	loops       map[cfg.Point]LoopFact
	scopeExits  map[cfg.Point]ScopeExitFact
}

// Branch returns the branch fact for point.
func (m Metadata) Branch(point cfg.Point) (BranchFact, bool) {
	fact, ok := m.branches[point]
	return fact, ok
}

// SetBranch records a branch fact for point.
func (m *Metadata) SetBranch(point cfg.Point, fact BranchFact) {
	if m.branches == nil {
		m.branches = make(map[cfg.Point]BranchFact)
	}
	m.branches[point] = fact
}

// Assignment returns the assignment fact for point.
func (m Metadata) Assignment(point cfg.Point) (AssignmentFact, bool) {
	fact, ok := m.assignments[point]
	return fact, ok
}

// SetAssignment records an assignment fact for point.
func (m *Metadata) SetAssignment(point cfg.Point, fact AssignmentFact) {
	if m.assignments == nil {
		m.assignments = make(map[cfg.Point]AssignmentFact)
	}
	m.assignments[point] = fact
}

// Call returns the call fact for point.
func (m Metadata) Call(point cfg.Point) (CallFact, bool) {
	fact, ok := m.calls[point]
	return fact, ok
}

// SetCall records a call fact for point.
func (m *Metadata) SetCall(point cfg.Point, fact CallFact) {
	if m.calls == nil {
		m.calls = make(map[cfg.Point]CallFact)
	}
	m.calls[point] = fact
}

// Loop returns the loop fact for point.
func (m Metadata) Loop(point cfg.Point) (LoopFact, bool) {
	fact, ok := m.loops[point]
	if !ok {
		return LoopFact{}, false
	}
	return copyLoopFact(fact), true
}

// SetLoop records a loop fact for point.
func (m *Metadata) SetLoop(point cfg.Point, fact LoopFact) {
	if m.loops == nil {
		m.loops = make(map[cfg.Point]LoopFact)
	}
	m.loops[point] = copyLoopFact(fact)
}

// ScopeExit returns the scope-exit fact for point.
func (m Metadata) ScopeExit(point cfg.Point) (ScopeExitFact, bool) {
	fact, ok := m.scopeExits[point]
	return fact, ok
}

// SetScopeExit records a scope-exit fact for point.
func (m *Metadata) SetScopeExit(point cfg.Point, fact ScopeExitFact) {
	if m.scopeExits == nil {
		m.scopeExits = make(map[cfg.Point]ScopeExitFact)
	}
	m.scopeExits[point] = fact
}

func copyLoopFact(fact LoopFact) LoopFact {
	fact.Vars = append([]symbol.ID(nil), fact.Vars...)
	fact.Locals = append([]symbol.ID(nil), fact.Locals...)
	return fact
}
