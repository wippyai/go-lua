// Package operationplan compiles immutable transfer facts into a dense,
// point-indexed description of semantic work. The plan is metadata only: it
// deliberately does not execute facts or duplicate their payloads.
package operationplan

import (
	"fmt"
	"math/bits"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// Class distinguishes directly executable transfer operations from facts
// consumed as sidecars by a composite operation and expression-level
// dependencies consulted while executing those operations.
type Class uint8

const (
	Executable Class = iota + 1
	Composite
	CompositeSidecar
	Dependency
)

// Phase identifies the production transfer which owns a point-local family.
type Phase uint8

const (
	Node Phase = iota + 1
	Edge
)

// Barrier is a semantic sequencing boundary in the current fact applicators.
// Values are metadata, not an execution ABI; cursorOrder is the ordering
// authority rather than Kind or FactsInput declaration order.
type Barrier uint8

const (
	N0Materialize Barrier = iota + 1
	N1NoReturn
	N2ImplicationClosure
	N3Postconditions
	N4Writes
	N5Return
	N6CovariantFinalizer
	E0Reachability
	E1Refinements
	E2ImplicationClosure
	E3Relations
	E4Evidence
	E5CallEffects
	N7BodySemantics
)

// BarrierSet records exact non-contiguous stages of a composite operation.
type BarrierSet uint16

func barriers(values ...Barrier) BarrierSet {
	var set BarrierSet
	for _, barrier := range values {
		set |= 1 << (barrier - 1)
	}
	return set
}

func (s BarrierSet) Has(barrier Barrier) bool {
	return barrier != 0 && s&(1<<(barrier-1)) != 0
}

// OwnerSet identifies the composite transactions which consume a sidecar or
// expression dependency. Kinds remain below 32 by the catalog gate.
type OwnerSet uint32

func owners(kinds ...Kind) OwnerSet {
	var set OwnerSet
	for _, kind := range kinds {
		set |= 1 << (kind - 1)
	}
	return set
}

func (s OwnerSet) Has(kind Kind) bool {
	return kind != 0 && s&(1<<(kind-1)) != 0
}

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
	kind Kind
}

func (c Cell) Kind() Kind       { return c.kind }
func (c Cell) Class() Class     { return descriptorForKind(c.kind).class }
func (c Cell) Phase() Phase     { return descriptorForKind(c.kind).phase }
func (c Cell) Barrier() Barrier { return descriptorForKind(c.kind).barrier }
func (c Cell) Stages() BarrierSet {
	d := descriptorForKind(c.kind)
	if d.stages != 0 {
		return d.stages
	}
	return barriers(d.barrier)
}
func (c Cell) Owners() OwnerSet { return descriptorForKind(c.kind).owners }

type row struct {
	start uint32
	end   uint32
}

// Plan owns the immutable Facts snapshot and a packed index over point-local
// fact families. Its fields are never exposed as mutable slices.
type Plan struct {
	facts                        factflow.Facts
	rows                         []row
	cells                        []Cell
	dependencies                 []Kind
	extensionRows                []extensionRow
	extensionCells               []ExtensionCell
	boundaryParams               []symbol.ID
	boundaryCaptures             []symbol.ID
	boundaryCapturesValid        bool
	boundaryReturns              []product.Value
	signatureRefs                []uint32
	signatures                   []SignatureCallOperation
	signatureAllocationRefs      []uint32
	signatureAllocationOrdinals  []uint32
	signatureAllocationOwners    []uint64
	signatureAllocationTemplates []signature.ReturnAllocationTemplate
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
	p := &Plan{facts: factflow.NewFacts(input), dependencies: compileDependencies(input), boundaryCapturesValid: true}
	p.rows, p.cells = compileIndex(size, input)
	return p
}

func compileDependencies(input factflow.FactsInput) []Kind {
	out := make([]Kind, 0, 8)
	if len(input.ObjectLiterals) != 0 {
		out = append(out, ObjectLiteral)
	}
	if len(input.ExpressionValues) != 0 {
		out = append(out, ExpressionValue)
	}
	if len(input.ExpressionOperations) != 0 {
		out = append(out, ExpressionOperation)
	}
	if len(input.ExpressionFunctions) != 0 {
		out = append(out, ExpressionFunction)
	}
	if len(input.ExpressionRefinements) != 0 {
		out = append(out, ExpressionRefinement)
	}
	if len(input.ExpressionPaths) != 0 {
		out = append(out, ExpressionPath)
	}
	if len(input.DynamicIndexExpressions) != 0 {
		out = append(out, DynamicIndexExpression)
	}
	if len(input.ExpressionConditions) != 0 {
		out = append(out, ExpressionCondition)
	}
	return out
}

// DependencyCursor traverses present expression-keyed fact families without
// allocating. Dependencies are not point cells but remain semantic inputs.
type DependencyCursor struct {
	kinds []Kind
	next  uint8
}

func (p *Plan) DependencyCursor() DependencyCursor {
	if p == nil {
		return DependencyCursor{}
	}
	return DependencyCursor{kinds: p.dependencies}
}
func (c *DependencyCursor) Next() (Kind, bool) {
	if c == nil || int(c.next) >= len(c.kinds) {
		return 0, false
	}
	kind := c.kinds[c.next]
	c.next++
	return kind, true
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
			cells = append(cells, Cell{kind: cursorOrder[index]})
			mask &^= uint32(1) << index
		}
		rows[point].end = uint32(len(cells))
	}
	return rows, cells
}

func markMap[V any](masks []uint32, kind Kind, facts map[cfg.Point]V) {
	bit := uint32(1) << cursorBit(kind)
	for point := range facts {
		if uint64(point) < uint64(len(masks)) {
			masks[point] |= bit
		}
	}
}

func markNonEmpty[V any](masks []uint32, kind Kind, facts map[cfg.Point][]V) {
	bit := uint32(1) << cursorBit(kind)
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

// Kinds returns the immutable fact-family catalog in canonical order. Callers
// building exhaustive cross-product registries must derive from this list
// rather than assuming the current last numeric constant remains last.
func Kinds() []Kind {
	out := make([]Kind, len(descriptors))
	for i, descriptor := range descriptors {
		out[i] = descriptor.kind
	}
	return out
}

// Metadata is the behavior-neutral semantic classification of a fact family.
// Dependencies have no Phase/Barrier but do carry their consuming Owners.
type Metadata struct {
	Class   Class
	Phase   Phase
	Barrier Barrier
	Stages  BarrierSet
	Owners  OwnerSet
}

// Describe returns the canonical semantic metadata for kind.
func Describe(kind Kind) (Metadata, bool) {
	d := descriptorForKind(kind)
	if d.kind == 0 {
		return Metadata{}, false
	}
	stages := d.stages
	if stages == 0 && d.barrier != 0 {
		stages = barriers(d.barrier)
	}
	return Metadata{Class: d.class, Phase: d.phase, Barrier: d.barrier, Stages: stages, Owners: d.owners}, true
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
	field   string
	kind    Kind
	class   Class
	phase   Phase
	barrier Barrier
	stages  BarrierSet
	owners  OwnerSet
}

// descriptors is also the exhaustiveness contract with factflow.FactsInput.
// Dependencies are expression-keyed and therefore have no dense point cell.
var descriptors = [...]descriptor{
	{"RootAssignments", RootAssignment, Composite, Node, N4Writes, 0, 0},
	{"PathAssignments", PathAssignment, Composite, Node, N4Writes, 0, 0},
	{"PathStaticMemberWrites", PathStaticMemberWrite, Composite, Node, N4Writes, 0, 0},
	{"DynamicIndexWrites", DynamicIndexWrite, Composite, Node, N4Writes, 0, 0},
	{"PathDescendantInvalidations", PathDescendantInvalidation, Executable, Node, N3Postconditions, 0, 0},
	{"CovariantExposures", CovariantExposure, Executable, Node, N6CovariantFinalizer, 0, 0},
	{"NoNormalReturns", NoNormalReturn, Executable, Node, N1NoReturn, 0, 0},
	{"BranchEdgeReachability", BranchEdgeReachability, Composite, Edge, E0Reachability, 0, 0},
	{"BranchConditionSources", BranchConditionSource, CompositeSidecar, Edge, E0Reachability, 0, owners(BranchEdgeReachability)},
	{"BranchRefinements", BranchRefinement, Composite, Edge, E1Refinements, barriers(E1Refinements, E3Relations), 0},
	{"BranchPresenceRelations", BranchPresenceRelation, Executable, Edge, E3Relations, 0, 0},
	{"BranchPathRelations", BranchPathRelation, Executable, Edge, E3Relations, 0, 0},
	{"BranchPathEvidence", BranchPathEvidence, Executable, Edge, E4Evidence, 0, 0},
	{"BranchSufficientLiteralCases", BranchSufficientLiteralCase, CompositeSidecar, Edge, E1Refinements, 0, owners(BranchRefinement)},
	{"PathValuePresenceImplications", PathValuePresenceImplication, Executable, Node, N2ImplicationClosure, 0, 0},
	{"ChannelSelects", ChannelSelect, Composite, Node, N0Materialize, barriers(N0Materialize, N3Postconditions), 0},
	{"PostconditionRefinements", PostconditionRefinement, Executable, Node, N3Postconditions, 0, 0},
	{"PostconditionPathRelations", PostconditionPathRelation, Executable, Node, N3Postconditions, 0, 0},
	{"CallResultValues", CallResultValue, CompositeSidecar, Node, N0Materialize, 0, owners(CallSite)},
	{"ReturnPresenceRelations", ReturnPresenceRelation, CompositeSidecar, Node, N5Return, 0, owners(Return)},
	{"Returns", Return, Composite, Node, N5Return, 0, 0},
	{"CallSites", CallSite, Composite, Node, N0Materialize, barriers(N0Materialize, E5CallEffects), 0},
	{"ObjectLiterals", ObjectLiteral, Dependency, 0, 0, 0, owners(CallSite, RootAssignment, PathAssignment)},
	{"ExpressionValues", ExpressionValue, Dependency, 0, 0, 0, owners(CallSite, RootAssignment, PathAssignment, PathStaticMemberWrite, DynamicIndexWrite, Return)},
	{"ExpressionOperations", ExpressionOperation, Dependency, 0, 0, 0, owners(CallSite, RootAssignment, PathAssignment, PathStaticMemberWrite, DynamicIndexWrite, Return)},
	{"ExpressionFunctions", ExpressionFunction, Dependency, 0, 0, 0, owners(CallSite, RootAssignment, PathAssignment, PathStaticMemberWrite, DynamicIndexWrite, Return)},
	{"ExpressionRefinements", ExpressionRefinement, Dependency, 0, 0, 0, owners(CallSite, RootAssignment, PathAssignment, PathStaticMemberWrite, DynamicIndexWrite, Return)},
	{"ExpressionPaths", ExpressionPath, Dependency, 0, 0, 0, owners(CallSite, RootAssignment, PathAssignment, PathStaticMemberWrite, DynamicIndexWrite, Return, BranchEdgeReachability)},
	{"DynamicIndexExpressions", DynamicIndexExpression, Dependency, 0, 0, 0, owners(CallSite, RootAssignment, PathAssignment, PathStaticMemberWrite, DynamicIndexWrite, Return)},
	{"ExpressionConditions", ExpressionCondition, Dependency, 0, 0, 0, owners(BranchRefinement)},
}

// cursorOrder is the current concrete semantic order. E2 implication closure
// and E5 call effects are executor barriers with no independently keyed family;
// BranchRefinement and CallSite advertise those stages in descriptor.stages.
var cursorOrder = [...]Kind{
	CallSite, CallResultValue, ChannelSelect,
	NoNormalReturn,
	PathValuePresenceImplication,
	PathDescendantInvalidation, PostconditionRefinement, PostconditionPathRelation,
	DynamicIndexWrite, RootAssignment, PathAssignment, PathStaticMemberWrite,
	Return, ReturnPresenceRelation,
	CovariantExposure,
	BranchEdgeReachability, BranchConditionSource,
	BranchRefinement, BranchSufficientLiteralCase,
	BranchPresenceRelation, BranchPathRelation,
	BranchPathEvidence,
}

func descriptorForKind(kind Kind) descriptor {
	if kind == 0 || int(kind) > len(descriptors) {
		return descriptor{}
	}
	return descriptors[kind-1]
}

func cursorBit(kind Kind) uint {
	for bit, candidate := range cursorOrder {
		if candidate == kind {
			return uint(bit)
		}
	}
	panic("operationplan: non-point kind in cursor index")
}

func (k Kind) String() string {
	for _, d := range descriptors {
		if d.kind == k {
			return d.field
		}
	}
	return fmt.Sprintf("Kind(%d)", k)
}
