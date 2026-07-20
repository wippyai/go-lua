package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// formalSparseTupleProjection names the exact physical fibers an operation
// reads from one tuple. The list is frozen by the operation adapter; this
// primitive never scans the product inventory to discover dependencies.
type formalSparseTupleProjection struct {
	tuple    formalRelationTuple
	ordinals []formalFiberOrdinal
	derived  []decisionRef
}

// formalSparseLeafView is one correlated alternative over an explicitly
// selected tuple projection. Unselected fibers are absent, not Bottom: their
// immutable directory subtrees remain structural carry at publication.
type formalSparseLeafView struct {
	algebra   *formalTupleAlgebra
	variable  relationVar
	span      formalFiberDescriptorSpan
	authority *formalComponentTerminalAuthority
	body      *relationProgramBody
	guard     decisionRef
	ordinals  []formalFiberOrdinal
	positions formalOrdinalPositions
	leaves    []decisionLeaf
	derived   []decisionLeaf
}

type formalSparseLeafPartition struct {
	guard decisionRef
	views []formalSparseLeafView
}

// formalFiberLeafSelection is the only leaf-addressing vocabulary used by a
// tuple evaluator. A selected physical Bottom is represented by (0, true);
// an unselected ordinal is (0, false). Keeping those cases distinct is what
// lets sparse transactions fail closed instead of silently manufacturing
// Bottom for an undeclared dependency.
//
// Dense selections borrow one complete descriptor-aligned row. Sparse
// selections borrow sorted ordinal/leaf pairs produced by the DD partitioner.
type formalFiberLeafSelection struct {
	span      formalFiberDescriptorSpan
	dense     []decisionLeaf
	ordinals  []formalFiberOrdinal
	positions formalOrdinalPositions
	leaves    []decisionLeaf
}

// formalOrdinalPositions is one sealed direct directory from a physical
// ordinal to its position in an operation projection. A negative entry means
// absent. The directory is built once per projection and shared by every DD
// region; it never owns or copies a leaf row.
type formalOrdinalPositions struct {
	ordinals  []formalFiberOrdinal
	positions []int
}

func sealFormalOrdinalPositions(count int, ordinals []formalFiberOrdinal) (formalOrdinalPositions, error) {
	if count <= 0 {
		return formalOrdinalPositions{}, errFormalComponentMalformed
	}
	positions := make([]int, count)
	for index := range positions {
		positions[index] = -1
	}
	for index, ordinal := range ordinals {
		if ordinal < 0 || int(ordinal) >= count || index > 0 && ordinals[index-1] >= ordinal {
			return formalOrdinalPositions{}, errFormalComponentMalformed
		}
		positions[ordinal] = index
	}
	return formalOrdinalPositions{ordinals: ordinals, positions: positions}, nil
}

func (p formalOrdinalPositions) position(ordinal formalFiberOrdinal) (int, bool) {
	if ordinal < 0 || int(ordinal) >= len(p.positions) {
		return 0, false
	}
	position := p.positions[ordinal]
	return position, position >= 0
}

func (p formalOrdinalPositions) validFor(count int, ordinals []formalFiberOrdinal) bool {
	if len(p.positions) != count || len(p.ordinals) != len(ordinals) {
		return false
	}
	if len(ordinals) == 0 {
		return true
	}
	return &p.ordinals[0] == &ordinals[0]
}

func denseFormalFiberLeafSelection(span formalFiberDescriptorSpan, leaves []decisionLeaf) (formalFiberLeafSelection, error) {
	if span.count == 0 || len(leaves) != span.count || leaves[0] != decisionLeaf(decisionTrue) {
		return formalFiberLeafSelection{}, errFormalComponentMalformed
	}
	return formalFiberLeafSelection{span: span, dense: leaves}, nil
}

func sparseFormalFiberLeafSelection(view formalSparseLeafView) (formalFiberLeafSelection, error) {
	if view.span.count == 0 || view.span.variable != view.variable || len(view.ordinals) != len(view.leaves) {
		return formalFiberLeafSelection{}, errFormalComponentMalformed
	}
	for index, ordinal := range view.ordinals {
		position, present := view.positions.position(ordinal)
		if int(ordinal) >= view.span.count || index > 0 && view.ordinals[index-1] >= ordinal || !present || position != index {
			return formalFiberLeafSelection{}, errFormalComponentMalformed
		}
	}
	if !view.positions.validFor(view.span.count, view.ordinals) {
		return formalFiberLeafSelection{}, errFormalComponentMalformed
	}
	return formalFiberLeafSelection{span: view.span, ordinals: view.ordinals, positions: view.positions, leaves: view.leaves}, nil
}

func (s formalFiberLeafSelection) valid(span formalFiberDescriptorSpan) bool {
	if s.span.variable != span.variable || s.span.first != span.first || s.span.count != span.count || span.count == 0 {
		return false
	}
	if s.dense != nil {
		return len(s.dense) == span.count && len(s.ordinals) == 0 && len(s.leaves) == 0 && s.dense[0] == decisionLeaf(decisionTrue)
	}
	if len(s.ordinals) == 0 || len(s.ordinals) != len(s.leaves) || !s.positions.validFor(span.count, s.ordinals) {
		return false
	}
	for index, ordinal := range s.ordinals {
		position, present := s.positions.position(ordinal)
		if int(ordinal) >= span.count || index > 0 && s.ordinals[index-1] >= ordinal || !present || position != index {
			return false
		}
	}
	return true
}

func (s formalFiberLeafSelection) leaf(ordinal formalFiberOrdinal) (decisionLeaf, bool) {
	if int(ordinal) >= s.span.count {
		return 0, false
	}
	if s.dense != nil {
		return s.dense[ordinal], true
	}
	index, present := s.positions.position(ordinal)
	if !present || index >= len(s.leaves) {
		return 0, false
	}
	return s.leaves[index], true
}

func (s formalFiberLeafSelection) position(ordinal formalFiberOrdinal) (int, bool) {
	if ordinal < 0 || int(ordinal) >= s.span.count {
		return 0, false
	}
	if s.dense != nil {
		return int(ordinal), true
	}
	return s.positions.position(ordinal)
}

func (s formalFiberLeafSelection) group(group formalFiberGroupDescriptor) ([]decisionLeaf, error) {
	if !group.valid() || group.variable != s.span.variable {
		return nil, errFormalComponentForeignOwner
	}
	leaves := make([]decisionLeaf, len(group.members))
	for index, ordinal := range group.members {
		leaf, present := s.leaf(ordinal)
		if !present {
			return nil, fmt.Errorf("transformer: formal leaf selection is missing declared ordinal %d", ordinal)
		}
		leaves[index] = leaf
	}
	return leaves, nil
}

func (s formalFiberLeafSelection) complete() ([]decisionLeaf, error) {
	if s.dense == nil || len(s.dense) != s.span.count {
		return nil, fmt.Errorf("transformer: formal leaf selection is not complete")
	}
	return s.dense, nil
}

func (l formalSparseLeafView) laneFactor(group formalFiberGroupDescriptor) (state.LaneFactor, error) {
	if l.algebra == nil || l.authority == nil || !group.valid() || group.variable != l.variable {
		return state.LaneFactor{}, errFormalComponentForeignOwner
	}
	leaves := make([]decisionLeaf, len(group.members))
	for index, ordinal := range group.members {
		leaf, present := l.leaf(ordinal)
		if !present {
			return state.LaneFactor{}, errFormalComponentMalformed
		}
		leaves[index] = leaf
	}
	if group.kind != formalFiberGroupOrdinaryLane && group.kind != formalFiberGroupCoordinateLane {
		return state.LaneFactor{}, errFormalComponentMalformed
	}
	return l.algebra.formalFactorSpelling(l.authority, group, leaves)
}

// factorFormalSparseLane returns one registered non-Values factor to its exact
// frozen fiber group. It is shared by every sparse operation adapter; no
// operation-specific carrier or axis switch is permitted here.
func (a *formalTupleAlgebra) factorFormalSparseLane(
	authority *formalComponentTerminalAuthority,
	span formalFiberDescriptorSpan,
	group formalFiberGroupDescriptor,
	factor state.LaneFactor,
) ([]decisionLeaf, error) {
	switch group.kind {
	case formalFiberGroupOrdinaryLane:
		return a.factorOrdinaryGroup(authority, group, factor)
	case formalFiberGroupCoordinateLane:
		return a.factorCoordinateGroup(authority, span, group, factor)
	default:
		return nil, errFormalComponentMalformed
	}
}

func (l formalSparseLeafView) leaf(ordinal formalFiberOrdinal) (decisionLeaf, bool) {
	index, present := l.positions.position(ordinal)
	if !present || index >= len(l.leaves) {
		return 0, false
	}
	return l.leaves[index], true
}

func (l *formalSparseLeafView) setLeaf(ordinal formalFiberOrdinal, leaf decisionLeaf) bool {
	if l == nil {
		return false
	}
	index, present := l.positions.position(ordinal)
	if !present || index >= len(l.leaves) {
		return false
	}
	l.leaves[index] = leaf
	return true
}

func (l formalSparseLeafView) value(member formalFiberGroupMember, top formalFiberGroupMember) (product.Value, bool) {
	topOrdinal, topOK := top.address(top.group)
	ordinal, memberOK := member.address(member.group)
	if !topOK || !memberOK {
		return product.Value{}, false
	}
	if leaf, present := l.leaf(topOrdinal); !present {
		return product.Value{}, false
	} else if leaf == 1 {
		return product.Top(), true
	} else if leaf > 1 {
		return product.Value{}, false
	}
	leaf, present := l.leaf(ordinal)
	if !present {
		return product.Value{}, false
	}
	if leaf == 0 {
		return product.Bottom(l.authority.product.Registry()), true
	}
	terminal, err := l.authority.terminal(leaf)
	if err != nil || terminal.kind != formalComponentGroundValue ||
		!product.BelongsToRegistry(l.authority.product.Registry(), terminal.ground) {
		return product.Value{}, false
	}
	return terminal.ground, true
}

func (l formalSparseLeafView) exactGuard(owner relationVar, arena *Arena, scope loopMuTerm, guard Guard) (bool, bool, bool) {
	if l.algebra == nil || owner != l.variable || arena != l.authority.terms || guard == 0 {
		return false, false, false
	}
	decision, err := l.algebra.decisionForGuard(owner, scope, arena, guard)
	if err != nil {
		return false, false, false
	}
	not, err := formalDecisionBooleanNot(l.algebra, decision)
	if err != nil {
		return false, false, false
	}
	truth, err := l.algebra.decisions.apply(l.algebra.ctx, uint8(decisionAnd), true, l.guard, decision, decisionLeafAnd)
	if err != nil {
		return false, false, false
	}
	falsity, err := l.algebra.decisions.apply(l.algebra.ctx, uint8(decisionAnd), true, l.guard, not, decisionLeafAnd)
	return truth != decisionFalse, falsity != decisionFalse, err == nil
}

// partitionSparseLeafViewsUnderCare correlates only the requested fibers and
// guard decisions. It preserves tuple and sealed ordinal order, so callers can
// bind returned views without maps or inventory-dependent order.
func (a *formalTupleAlgebra) partitionSparseLeafViewsUnderCare(
	projections []formalSparseTupleProjection,
	demands []formalQualifiedGuardDemand,
) ([]formalSparseLeafPartition, error) {
	if a == nil || len(projections) == 0 {
		return nil, errFormalComponentForeignOwner
	}
	variable := projections[0].tuple.variable
	span, directory, authority, ok := a.span(variable)
	if !ok {
		return nil, errFormalComponentForeignOwner
	}
	care := decisionTrue
	canonical := make([][]formalFiberOrdinal, len(projections))
	positions := make([]formalOrdinalPositions, len(projections))
	total := len(demands)
	for index, projection := range projections {
		if err := a.validateTuple(projection.tuple); err != nil {
			return nil, err
		}
		if projection.tuple.bottom() || projection.tuple.variable != variable || projection.tuple.root.owner != directory {
			return nil, errFormalComponentForeignOwner
		}
		tupleCare, err := a.care(projection.tuple)
		if err != nil {
			return nil, err
		}
		care, err = a.decisions.apply(a.ctx, uint8(decisionAnd), true, care, tupleCare, decisionLeafAnd)
		if err != nil {
			return nil, err
		}
		for ordinalIndex, ordinal := range projection.ordinals {
			if err := directory.validateOrdinal(ordinal); err != nil {
				return nil, err
			}
			if ordinalIndex > 0 && projection.ordinals[ordinalIndex-1] >= ordinal {
				return nil, fmt.Errorf("transformer: sparse tuple projection is not sealed")
			}
		}
		canonical[index] = projection.ordinals
		positions[index], err = sealFormalOrdinalPositions(span.count, canonical[index])
		if err != nil {
			return nil, fmt.Errorf("transformer: sparse tuple projection has no direct positions: %w", err)
		}
		total += len(canonical[index]) + len(projection.derived)
	}
	for index, demand := range demands {
		if demand.owner == 0 || demand.arena == nil || demand.guard == 0 ||
			int(demand.guard) >= len(demand.arena.guards) || !demand.arena.Sealed() {
			return nil, fmt.Errorf("transformer: sparse tuple guard demand is unowned")
		}
		key := formalScopedGuardKey{variable: demand.owner, scope: demand.scope, arena: demand.arena, guard: demand.guard}
		for prior := 0; prior < index; prior++ {
			other := demands[prior]
			if key == (formalScopedGuardKey{variable: other.owner, scope: other.scope, arena: other.arena, guard: other.guard}) {
				return nil, fmt.Errorf("transformer: sparse tuple guard demands are not sealed")
			}
		}
	}
	roots := make([]decisionRef, 0, total)
	for index, projection := range projections {
		for _, ordinal := range canonical[index] {
			value, err := directory.valueAt(projection.tuple.root, ordinal)
			if err != nil {
				return nil, err
			}
			roots = append(roots, decisionRef(value))
		}
		roots = append(roots, projection.derived...)
	}
	for _, demand := range demands {
		root, err := a.decisionForGuard(demand.owner, demand.scope, demand.arena, demand.guard)
		if err != nil {
			return nil, err
		}
		roots = append(roots, root)
	}
	rows, err := a.decisions.partitionLeafTuplesUnderCare(a.ctx, care, roots)
	if err != nil {
		return nil, err
	}
	out := make([]formalSparseLeafPartition, len(rows))
	for rowIndex, row := range rows {
		if len(row.leaves) != len(roots) {
			return nil, errDecisionMalformed
		}
		cursor := 0
		views := make([]formalSparseLeafView, len(projections))
		for projectionIndex := range projections {
			width := len(canonical[projectionIndex])
			derivedWidth := len(projections[projectionIndex].derived)
			views[projectionIndex] = formalSparseLeafView{
				algebra: a, variable: variable, span: span, authority: authority,
				body: &a.program.bodies[variable-1], guard: row.care,
				ordinals: canonical[projectionIndex], leaves: row.leaves[cursor : cursor+width],
				positions: positions[projectionIndex],
				derived:   row.leaves[cursor+width : cursor+width+derivedWidth],
			}
			cursor += width + derivedWidth
		}
		out[rowIndex] = formalSparseLeafPartition{guard: row.care, views: views}
	}
	return out, nil
}
