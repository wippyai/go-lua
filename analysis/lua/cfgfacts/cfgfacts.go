// Package cfgfacts stores Lua sidecar facts for CFG points.
package cfgfacts

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

// LoopFact describes loop structure associated with a CFG point.
type LoopFact struct {
	Vars         []symbol.ID
	Locals       []symbol.ID
	Preheader    cfg.Point
	HasPreheader bool
}

// Metadata stores Lua sidecar facts keyed by CFG point.
type Metadata struct {
	branches    map[cfg.Point]BranchFact
	assignments map[cfg.Point]AssignmentFact
	loops       map[cfg.Point]LoopFact
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

func copyLoopFact(fact LoopFact) LoopFact {
	fact.Vars = append([]symbol.ID(nil), fact.Vars...)
	fact.Locals = append([]symbol.ID(nil), fact.Locals...)
	return fact
}
