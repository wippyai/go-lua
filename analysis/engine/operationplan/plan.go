// Package operationplan compiles immutable transfer facts into a dense,
// point-indexed description of semantic work. The plan is metadata only: it
// deliberately does not execute facts or duplicate their payloads.
package operationplan

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// Class distinguishes directly executable transfer operations from facts
// consumed as sidecars by a composite operation and expression-level
// dependencies consulted while executing those operations.
type Class uint8

const (
	Executable Class = iota + 1
	CompositeSidecar
	Dependency
)

// Kind identifies one FactsInput field. Values are stable and declared in the
// same order as FactsInput so plan construction is independent of map order.
type Kind uint8

const (
	RootAssignment Kind = iota + 1
	PathAssignment
	PathStaticMemberWrite
	DynamicIndexWrite
	PathDescendantInvalidation
	CovariantExposure
	NoNormalReturn
	BranchEdgeReachability
	BranchConditionSource
	BranchRefinement
	BranchPresenceRelation
	BranchPathRelation
	BranchPathEvidence
	BranchSufficientLiteralCase
	PathValuePresenceImplication
	ChannelSelect
	PostconditionRefinement
	PostconditionPathRelation
	CallResultValue
	ReturnPresenceRelation
	Return
	CallSite
	ObjectLiteral
	ExpressionValue
	ExpressionOperation
	ExpressionFunction
	ExpressionRefinement
	ExpressionPath
	DynamicIndexExpression
	ExpressionCondition
)

// Cell describes one non-empty fact family at a CFG point. Payloads remain in
// the Plan's single immutable Facts snapshot.
type Cell struct {
	kind  Kind
	class Class
}

func (c Cell) Kind() Kind   { return c.kind }
func (c Cell) Class() Class { return c.class }

type row struct {
	start uint32
	end   uint32
}

// Plan owns the immutable Facts snapshot and a packed index over point-local
// fact families. Its fields are never exposed as mutable slices.
type Plan struct {
	facts factflow.Facts
	rows  []row
	cells []Cell
}

// New creates the only immutable Facts snapshot for input and indexes all
// point-local operations and sidecars. Expression-keyed dependencies remain in
// Facts and are represented in the plan's exhaustive kind catalog rather than
// duplicated into per-point rows.
func New(graph cfg.Graph, input factflow.FactsInput) *Plan {
	size := 0
	if graph != nil {
		size = graph.Size()
	}
	p := &Plan{facts: factflow.NewFacts(input), rows: make([]row, size)}
	for point := 0; point < size; point++ {
		p.rows[point].start = uint32(len(p.cells))
		appendPointCells(&p.cells, cfg.Point(point), input)
		p.rows[point].end = uint32(len(p.cells))
	}
	return p
}

// Facts returns the plan-owned immutable transfer-fact snapshot. Facts has
// value semantics over immutable maps, so this does not copy fact payloads.
func (p *Plan) Facts() factflow.Facts {
	if p == nil {
		return factflow.Facts{}
	}
	return p.facts
}

// Cursor returns a zero-allocation cursor over the fact families at point.
func (p *Plan) Cursor(point cfg.Point) Cursor {
	if p == nil || uint64(point) >= uint64(len(p.rows)) {
		return Cursor{}
	}
	r := p.rows[point]
	return Cursor{cells: p.cells, next: r.start, end: r.end}
}

// PointCount reports the number of dense CFG rows.
func (p *Plan) PointCount() int {
	if p == nil {
		return 0
	}
	return len(p.rows)
}

// Classification returns the declared role of a fact family. It is the
// fail-closed catalog used by exhaustiveness tests when FactsInput grows.
func Classification(kind Kind) (Class, bool) {
	for _, d := range descriptors {
		if d.kind == kind {
			return d.class, true
		}
	}
	return 0, false
}

// Cursor traverses a point row without allocating or exposing backing storage.
type Cursor struct {
	cells []Cell
	next  uint32
	end   uint32
}

func (c *Cursor) Next() (Cell, bool) {
	if c == nil || c.next >= c.end {
		return Cell{}, false
	}
	cell := c.cells[c.next]
	c.next++
	return cell, true
}

type descriptor struct {
	field string
	kind  Kind
	class Class
	at    func(factflow.FactsInput, cfg.Point) bool
}

// descriptors is also the exhaustiveness contract with factflow.FactsInput.
// Dependencies are expression-keyed and therefore have no dense point cell.
var descriptors = [...]descriptor{
	{"RootAssignments", RootAssignment, Executable, func(in factflow.FactsInput, p cfg.Point) bool { _, ok := in.RootAssignments[p]; return ok }},
	{"PathAssignments", PathAssignment, Executable, func(in factflow.FactsInput, p cfg.Point) bool { _, ok := in.PathAssignments[p]; return ok }},
	{"PathStaticMemberWrites", PathStaticMemberWrite, Executable, func(in factflow.FactsInput, p cfg.Point) bool { _, ok := in.PathStaticMemberWrites[p]; return ok }},
	{"DynamicIndexWrites", DynamicIndexWrite, Executable, func(in factflow.FactsInput, p cfg.Point) bool { _, ok := in.DynamicIndexWrites[p]; return ok }},
	{"PathDescendantInvalidations", PathDescendantInvalidation, Executable, func(in factflow.FactsInput, p cfg.Point) bool { _, ok := in.PathDescendantInvalidations[p]; return ok }},
	{"CovariantExposures", CovariantExposure, Executable, func(in factflow.FactsInput, p cfg.Point) bool { return len(in.CovariantExposures[p]) != 0 }},
	{"NoNormalReturns", NoNormalReturn, Executable, func(in factflow.FactsInput, p cfg.Point) bool { _, ok := in.NoNormalReturns[p]; return ok }},
	{"BranchEdgeReachability", BranchEdgeReachability, Executable, func(in factflow.FactsInput, p cfg.Point) bool { _, ok := in.BranchEdgeReachability[p]; return ok }},
	{"BranchConditionSources", BranchConditionSource, CompositeSidecar, func(in factflow.FactsInput, p cfg.Point) bool { _, ok := in.BranchConditionSources[p]; return ok }},
	{"BranchRefinements", BranchRefinement, Executable, func(in factflow.FactsInput, p cfg.Point) bool { _, ok := in.BranchRefinements[p]; return ok }},
	{"BranchPresenceRelations", BranchPresenceRelation, Executable, func(in factflow.FactsInput, p cfg.Point) bool { _, ok := in.BranchPresenceRelations[p]; return ok }},
	{"BranchPathRelations", BranchPathRelation, Executable, func(in factflow.FactsInput, p cfg.Point) bool { _, ok := in.BranchPathRelations[p]; return ok }},
	{"BranchPathEvidence", BranchPathEvidence, Executable, func(in factflow.FactsInput, p cfg.Point) bool { _, ok := in.BranchPathEvidence[p]; return ok }},
	{"BranchSufficientLiteralCases", BranchSufficientLiteralCase, CompositeSidecar, func(in factflow.FactsInput, p cfg.Point) bool { _, ok := in.BranchSufficientLiteralCases[p]; return ok }},
	{"PathValuePresenceImplications", PathValuePresenceImplication, Executable, func(in factflow.FactsInput, p cfg.Point) bool {
		_, ok := in.PathValuePresenceImplications[p]
		return ok
	}},
	{"ChannelSelects", ChannelSelect, Executable, func(in factflow.FactsInput, p cfg.Point) bool { _, ok := in.ChannelSelects[p]; return ok }},
	{"PostconditionRefinements", PostconditionRefinement, Executable, func(in factflow.FactsInput, p cfg.Point) bool { _, ok := in.PostconditionRefinements[p]; return ok }},
	{"PostconditionPathRelations", PostconditionPathRelation, Executable, func(in factflow.FactsInput, p cfg.Point) bool { return len(in.PostconditionPathRelations[p]) != 0 }},
	{"CallResultValues", CallResultValue, CompositeSidecar, func(in factflow.FactsInput, p cfg.Point) bool { _, ok := in.CallResultValues[p]; return ok }},
	{"ReturnPresenceRelations", ReturnPresenceRelation, CompositeSidecar, func(in factflow.FactsInput, p cfg.Point) bool { _, ok := in.ReturnPresenceRelations[p]; return ok }},
	{"Returns", Return, Executable, func(in factflow.FactsInput, p cfg.Point) bool { _, ok := in.Returns[p]; return ok }},
	{"CallSites", CallSite, CompositeSidecar, func(in factflow.FactsInput, p cfg.Point) bool { _, ok := in.CallSites[p]; return ok }},
	{"ObjectLiterals", ObjectLiteral, Dependency, nil},
	{"ExpressionValues", ExpressionValue, Dependency, nil},
	{"ExpressionOperations", ExpressionOperation, Dependency, nil},
	{"ExpressionFunctions", ExpressionFunction, Dependency, nil},
	{"ExpressionRefinements", ExpressionRefinement, Dependency, nil},
	{"ExpressionPaths", ExpressionPath, Dependency, nil},
	{"DynamicIndexExpressions", DynamicIndexExpression, Dependency, nil},
	{"ExpressionConditions", ExpressionCondition, Dependency, nil},
}

func appendPointCells(dst *[]Cell, point cfg.Point, in factflow.FactsInput) {
	for _, d := range descriptors {
		if d.at != nil && d.at(in, point) {
			*dst = append(*dst, Cell{kind: d.kind, class: d.class})
		}
	}
}

func (k Kind) String() string {
	for _, d := range descriptors {
		if d.kind == k {
			return d.field
		}
	}
	return fmt.Sprintf("Kind(%d)", k)
}
