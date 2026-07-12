// Package operationplan compiles immutable transfer facts into a dense,
// point-indexed description of semantic work. The plan is metadata only: it
// deliberately does not execute facts or duplicate their payloads.
package operationplan

import (
	"fmt"
	"math/bits"

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
	p := &Plan{facts: factflow.NewFacts(input)}
	p.rows, p.cells = compileIndex(size, input)
	return p
}

func compileIndex(size int, input factflow.FactsInput) ([]row, []Cell) {
	rows := make([]row, size)
	if size == 0 {
		return rows, nil
	}

	// Facts are sparse in real bodies. Record each present family once instead
	// of probing every family at every graph point. A bit set is also what makes
	// packing independent of Go's randomized map iteration order.
	masks := make([]uint32, size)
	markMap(masks, RootAssignment, input.RootAssignments)
	markMap(masks, PathAssignment, input.PathAssignments)
	markMap(masks, PathStaticMemberWrite, input.PathStaticMemberWrites)
	markMap(masks, DynamicIndexWrite, input.DynamicIndexWrites)
	markMap(masks, PathDescendantInvalidation, input.PathDescendantInvalidations)
	markNonEmpty(masks, CovariantExposure, input.CovariantExposures)
	markMap(masks, NoNormalReturn, input.NoNormalReturns)
	markMap(masks, BranchEdgeReachability, input.BranchEdgeReachability)
	markMap(masks, BranchConditionSource, input.BranchConditionSources)
	markMap(masks, BranchRefinement, input.BranchRefinements)
	markMap(masks, BranchPresenceRelation, input.BranchPresenceRelations)
	markMap(masks, BranchPathRelation, input.BranchPathRelations)
	markMap(masks, BranchPathEvidence, input.BranchPathEvidence)
	markMap(masks, BranchSufficientLiteralCase, input.BranchSufficientLiteralCases)
	markMap(masks, PathValuePresenceImplication, input.PathValuePresenceImplications)
	markMap(masks, ChannelSelect, input.ChannelSelects)
	markMap(masks, PostconditionRefinement, input.PostconditionRefinements)
	markNonEmpty(masks, PostconditionPathRelation, input.PostconditionPathRelations)
	markMap(masks, CallResultValue, input.CallResultValues)
	markMap(masks, ReturnPresenceRelation, input.ReturnPresenceRelations)
	markMap(masks, Return, input.Returns)
	markMap(masks, CallSite, input.CallSites)
	cellCount := 0
	for _, mask := range masks {
		cellCount += bits.OnesCount32(mask)
	}
	cells := make([]Cell, 0, cellCount)
	for point, mask := range masks {
		rows[point].start = uint32(len(cells))
		for mask != 0 {
			index := bits.TrailingZeros32(mask)
			d := descriptors[index]
			cells = append(cells, Cell{kind: d.kind, class: d.class})
			mask &^= uint32(1) << index
		}
		rows[point].end = uint32(len(cells))
	}
	return rows, cells
}

func markMap[V any](masks []uint32, kind Kind, facts map[cfg.Point]V) {
	bit := uint32(1) << (kind - 1)
	for point := range facts {
		if uint64(point) < uint64(len(masks)) {
			masks[point] |= bit
		}
	}
}

func markNonEmpty[V any](masks []uint32, kind Kind, facts map[cfg.Point][]V) {
	bit := uint32(1) << (kind - 1)
	for point, values := range facts {
		if len(values) != 0 && uint64(point) < uint64(len(masks)) {
			masks[point] |= bit
		}
	}
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
}

// descriptors is also the exhaustiveness contract with factflow.FactsInput.
// Dependencies are expression-keyed and therefore have no dense point cell.
var descriptors = [...]descriptor{
	{"RootAssignments", RootAssignment, Executable},
	{"PathAssignments", PathAssignment, Executable},
	{"PathStaticMemberWrites", PathStaticMemberWrite, Executable},
	{"DynamicIndexWrites", DynamicIndexWrite, Executable},
	{"PathDescendantInvalidations", PathDescendantInvalidation, Executable},
	{"CovariantExposures", CovariantExposure, Executable},
	{"NoNormalReturns", NoNormalReturn, Executable},
	{"BranchEdgeReachability", BranchEdgeReachability, Executable},
	{"BranchConditionSources", BranchConditionSource, CompositeSidecar},
	{"BranchRefinements", BranchRefinement, Executable},
	{"BranchPresenceRelations", BranchPresenceRelation, Executable},
	{"BranchPathRelations", BranchPathRelation, Executable},
	{"BranchPathEvidence", BranchPathEvidence, Executable},
	{"BranchSufficientLiteralCases", BranchSufficientLiteralCase, CompositeSidecar},
	{"PathValuePresenceImplications", PathValuePresenceImplication, Executable},
	{"ChannelSelects", ChannelSelect, Executable},
	{"PostconditionRefinements", PostconditionRefinement, Executable},
	{"PostconditionPathRelations", PostconditionPathRelation, Executable},
	{"CallResultValues", CallResultValue, CompositeSidecar},
	{"ReturnPresenceRelations", ReturnPresenceRelation, CompositeSidecar},
	{"Returns", Return, Executable},
	{"CallSites", CallSite, CompositeSidecar},
	{"ObjectLiterals", ObjectLiteral, Dependency},
	{"ExpressionValues", ExpressionValue, Dependency},
	{"ExpressionOperations", ExpressionOperation, Dependency},
	{"ExpressionFunctions", ExpressionFunction, Dependency},
	{"ExpressionRefinements", ExpressionRefinement, Dependency},
	{"ExpressionPaths", ExpressionPath, Dependency},
	{"DynamicIndexExpressions", DynamicIndexExpression, Dependency},
	{"ExpressionConditions", ExpressionCondition, Dependency},
}

func (k Kind) String() string {
	for _, d := range descriptors {
		if d.kind == k {
			return d.field
		}
	}
	return fmt.Sprintf("Kind(%d)", k)
}
