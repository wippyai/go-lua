// Package cfgfacts stores Lua sidecar facts for CFG points.
package cfgfacts

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// LoopKind identifies the structural loop form associated with a CFG point.
type LoopKind uint8

// Loop kind constants represent recognizable loop shapes.
const (
	LoopKindUnknown LoopKind = iota
	LoopKindConditional
	LoopKindNumericFor
	LoopKindGenericFor
)

// AssignmentFact describes an assignment target.
type AssignmentFact struct {
	Target symbol.ID
}

// LoopFact describes loop structure associated with a CFG point.
type LoopFact struct {
	Kind                 LoopKind
	Vars                 []symbol.ID
	Locals               []symbol.ID
	DirectModifiedOuters []symbol.ID
	Preheader            cfg.Point
	HasPreheader         bool
}

// Metadata stores Lua sidecar facts keyed by CFG point.
type Metadata struct {
	assignments map[cfg.Point]AssignmentFact
	loops       map[cfg.Point]LoopFact
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
	fact.DirectModifiedOuters = append([]symbol.ID(nil), fact.DirectModifiedOuters...)
	return fact
}
