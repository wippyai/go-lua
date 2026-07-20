package transformer

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// formalTupleLeafRegion is one exact guard region and its descriptor-aligned
// complete formal product row. The row borrows decision-kernel output for the
// duration of one Apply/Definition transaction; it retains no caller state.
type formalTupleLeafRegion struct {
	guard     decisionRef
	evaluator formalTupleLeafEvaluator
}

// formalQualifiedGuardDemand is one immutable Arena guard in its exact
// lexical owner and loop lifetime. A bare decisionRef is deliberately not a
// public partition input: decision capabilities are run-local and equal term
// ordinals in different arenas/scopes must never alias.
type formalQualifiedGuardDemand struct {
	owner relationVar
	scope loopMuTerm
	arena *Arena
	guard Guard
}

// formalTupleLeafEvaluator specializes arena-qualified symbolic bindings from
// one ordinal-addressed leaf selection. Whole-tuple consumers receive a dense
// selection; sparse transactions receive exactly their frozen dependency
// projection. An unselected fiber is an error, never an implicit Bottom. A
// product.Value is returned whole, so adding or removing an axis does not
// change this adapter.
type formalTupleLeafEvaluator struct {
	algebra   *formalTupleAlgebra
	variable  relationVar
	span      formalFiberDescriptorSpan
	authority *formalComponentTerminalAuthority
	body      *relationProgramBody
	values    formalFiberGroupDescriptor
	layout    *formalFactorExecutionLayout
	leaves    formalFiberLeafSelection
	guard     decisionRef
}

// formalEvaluatedBinding is the concrete boundary actual selected by one
// correlated symbolic alternative. Path absence remains distinct from an
// empty/malformed path.
type formalEvaluatedBinding struct {
	value       product.Value
	path        pathdom.Path
	pathPresent bool
}

// tupleLeafRegions aligns Care and every descriptor once. Consumers such as
// formal Apply/Definition may then instantiate all contracts in a region from
// exact leaf evaluators without reconstructing State or a BindingCursor.
func (a *formalTupleAlgebra) tupleLeafRegions(tuple formalRelationTuple) ([]formalTupleLeafRegion, error) {
	return a.tupleLeafRegionsWithGuardDemands(tuple, nil)
}

// tupleLeafRegionsWithGuardDemands performs the one complete DD partition for
// a tuple plus any symbolic guards whose truth must be decided by a consumer.
// The evaluator receives the descriptor prefix only; demand leaves merely
// refine row Care, so all consumers share the same canonical specialization
// path without exposing raw decision roots.
func (a *formalTupleAlgebra) tupleLeafRegionsWithGuardDemands(tuple formalRelationTuple, demands []formalQualifiedGuardDemand) ([]formalTupleLeafRegion, error) {
	if err := a.validateTuple(tuple); err != nil || tuple.bottom() {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("transformer: formal tuple leaf source is Bottom")
	}
	span, directory, _, ok := a.span(tuple.variable)
	if !ok {
		return nil, errFormalComponentForeignOwner
	}
	care, err := a.care(tuple)
	if err != nil {
		return nil, err
	}
	unique := make([]formalQualifiedGuardDemand, 0, len(demands))
	seen := make(map[formalScopedGuardKey]struct{}, len(demands))
	for _, demand := range demands {
		key := formalScopedGuardKey{variable: demand.owner, scope: demand.scope, arena: demand.arena, guard: demand.guard}
		if demand.owner == 0 || demand.arena == nil || demand.guard == 0 || int(demand.guard) >= len(demand.arena.guards) || !demand.arena.Sealed() {
			return nil, fmt.Errorf("transformer: formal tuple guard demand is unowned")
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, demand)
	}
	roots := make([]decisionRef, span.count+len(unique))
	for ordinal := 0; ordinal < span.count; ordinal++ {
		value, readErr := directory.valueAt(tuple.root, formalFiberOrdinal(ordinal))
		if readErr != nil {
			return nil, readErr
		}
		roots[ordinal] = decisionRef(value)
	}
	for index, demand := range unique {
		decision, decisionErr := a.decisionForGuard(demand.owner, demand.scope, demand.arena, demand.guard)
		if decisionErr != nil {
			return nil, decisionErr
		}
		roots[span.count+index] = decision
	}
	rows, err := a.decisions.partitionLeafTuplesUnderCare(a.ctx, care, roots)
	if err != nil {
		return nil, err
	}
	regions := make([]formalTupleLeafRegion, len(rows))
	for index, row := range rows {
		if len(row.leaves) != len(roots) {
			return nil, errDecisionMalformed
		}
		evaluator, evaluatorErr := a.newTupleLeafEvaluator(tuple.variable, row.leaves[:span.count], row.care)
		if evaluatorErr != nil {
			return nil, evaluatorErr
		}
		regions[index] = formalTupleLeafRegion{guard: row.care, evaluator: evaluator}
	}
	return regions, nil
}

func (a *formalTupleAlgebra) newTupleLeafEvaluator(variable relationVar, leaves []decisionLeaf, guard decisionRef) (formalTupleLeafEvaluator, error) {
	span, _, authority, ok := a.span(variable)
	if !ok || a.program == nil || int(variable) > len(a.program.bodies) || len(leaves) != span.count || len(leaves) == 0 || leaves[0] != 1 || int(guard) >= len(a.decisions.nodes) {
		return formalTupleLeafEvaluator{}, errFormalComponentForeignOwner
	}
	selection, err := denseFormalFiberLeafSelection(span, leaves)
	if err != nil {
		return formalTupleLeafEvaluator{}, err
	}
	// Closed-world tuple construction validates descriptor ownership at every
	// publication edge (defaults, scalar/group writes, combines and full-span
	// transactions). Partitioning only borrows those already-owned leaves, so
	// rescanning the entire descriptor span here would repeat a proof that no
	// raw tuple constructor can bypass.
	layout := &authority.body.factors
	if !layout.validFor(authority.product, variable) {
		return formalTupleLeafEvaluator{}, fmt.Errorf("transformer: formal tuple leaf has no complete Values group")
	}
	evaluator := formalTupleLeafEvaluator{
		algebra: a, variable: variable, span: span, authority: authority,
		body: &a.program.bodies[variable-1], values: layout.values, layout: layout, leaves: selection, guard: guard,
	}
	return evaluator, nil
}

func (a *formalTupleAlgebra) newSparseTupleLeafEvaluator(view formalSparseLeafView) (formalTupleLeafEvaluator, error) {
	span, _, authority, ok := a.span(view.variable)
	if !ok || a.program == nil || int(view.variable) > len(a.program.bodies) ||
		view.algebra != a || view.span.variable != view.variable || view.authority != authority ||
		view.body != &a.program.bodies[view.variable-1] || int(view.guard) >= len(a.decisions.nodes) {
		return formalTupleLeafEvaluator{}, errFormalComponentForeignOwner
	}
	selection, err := sparseFormalFiberLeafSelection(view)
	if err != nil {
		return formalTupleLeafEvaluator{}, err
	}
	layout := &authority.body.factors
	if !layout.validFor(authority.product, view.variable) {
		return formalTupleLeafEvaluator{}, fmt.Errorf("transformer: formal sparse tuple leaf has no complete Values group")
	}
	return formalTupleLeafEvaluator{
		algebra: a, variable: view.variable, span: span, authority: authority,
		body: view.body, values: layout.values, layout: layout, leaves: selection, guard: view.guard,
	}, nil
}

// evaluate specializes one authority-qualified symbolic alternative. The
// direct-root path performs one FormalSlot lookup and one terminal read. More
// complex terms enter Arena.evalValueCanonicalWithLeaves, the same interpreter
// used by concrete specialization.
func (e formalTupleLeafEvaluator) evaluate(binding formalQualifiedBinding) (formalEvaluatedBinding, error) {
	return e.evaluateWithFactorAccess(binding, nil)
}

func (e formalTupleLeafEvaluator) evaluateWithFactorAccess(binding formalQualifiedBinding, capability *formalValueFactorAccess) (formalEvaluatedBinding, error) {
	if !e.valid() || !binding.validForAuthority(e.authority) {
		return formalEvaluatedBinding{}, errFormalComponentForeignOwner
	}
	value, exact := e.evalQualifiedValueWithFactorAccess(binding, capability)
	if !exact || !product.BelongsToRegistry(e.authority.product.Registry(), value) {
		op := valueOp(0)
		slot := statekey.Value(0)
		if binding.value.arena != nil && binding.value.term != 0 && int(binding.value.term) < len(binding.value.arena.values) {
			node := binding.value.arena.values[binding.value.term]
			op, slot = node.op, node.slot
		}
		return formalEvaluatedBinding{}, fmt.Errorf("transformer: formal tuple leaf value is unsupported (owner=%d term=%d op=%d slot=%d)",
			binding.value.owner, binding.value.term, op, slot)
	}
	out := formalEvaluatedBinding{value: value, pathPresent: binding.pathPresent}
	if binding.pathPresent {
		out.path, exact = e.evalQualifiedPath(binding)
		if !exact || out.path.IsEmpty() {
			return formalEvaluatedBinding{}, fmt.Errorf("transformer: formal tuple leaf path is unsupported")
		}
	}
	return out, nil
}

func (e formalTupleLeafEvaluator) valid() bool {
	return e.algebra != nil && e.variable != 0 && e.span.variable == e.variable &&
		e.authority != nil && e.authority.variable == e.variable && e.body != nil &&
		e.body.variable == e.variable && e.values.valid() && e.values.variable == e.variable && e.layout != nil &&
		e.layout == &e.authority.body.factors && e.layout.validFor(e.authority.product, e.variable) &&
		e.leaves.valid(e.span) &&
		int(e.guard) < len(e.algebra.decisions.nodes)
}

func (e formalTupleLeafEvaluator) completeLeaves() ([]decisionLeaf, error) {
	if !e.valid() {
		return nil, errFormalComponentForeignOwner
	}
	return e.leaves.complete()
}

// valuesFactor exposes the complete registered Values carrier for this exact
// correlated leaf.  Apply and Definition share this one materialization edge;
// neither may reconstruct per-slot state or interpret Values independently.
func (e formalTupleLeafEvaluator) valuesFactor() (state.ValueFactor[FormalSlot], error) {
	if !e.valid() {
		return state.ValueFactor[FormalSlot]{}, errFormalComponentForeignOwner
	}
	leaves, err := e.leaves.group(e.values)
	if err != nil {
		return state.ValueFactor[FormalSlot]{}, err
	}
	return e.algebra.materializeValuesGroup(e.authority, e.values, leaves)
}

// laneFactor exposes one complete registered non-Values carrier for this
// exact correlated leaf.  ProductDomain registration, rather than an axis
// switch, selects ordinary versus coordinate-family materialization.
func (e formalTupleLeafEvaluator) laneFactor(group formalFiberGroupDescriptor) (state.LaneFactor, error) {
	if !e.valid() || !group.valid() || group.variable != e.variable || group.kind == formalFiberGroupValues {
		return state.LaneFactor{}, errFormalComponentForeignOwner
	}
	return e.algebra.formalSelectedFactorSpelling(e.authority, group, e.leaves)
}

// productFactors materializes the complete factor-native tuple in the exact
// ProductDomain.NonValuesLaneInventory order required by identity image and
// boundary transactions. The registry remains the sole axis inventory.
func (e formalTupleLeafEvaluator) productFactors() (state.ValueFactor[FormalSlot], []state.LaneFactor, error) {
	values, err := e.valuesFactor()
	if err != nil {
		return state.ValueFactor[FormalSlot]{}, nil, err
	}
	factors := make([]state.LaneFactor, len(e.layout.nonValues))
	for index, group := range e.layout.nonValues {
		factor, factorErr := e.laneFactor(group)
		if factorErr != nil {
			return state.ValueFactor[FormalSlot]{}, nil, factorErr
		}
		factors[index] = factor
	}
	return values, factors, nil
}

func (e formalTupleLeafEvaluator) evalQualifiedValue(binding formalQualifiedBinding) (product.Value, bool) {
	return e.evalQualifiedValueWithFactorAccess(binding, nil)
}

func (e formalTupleLeafEvaluator) evalQualifiedValueWithFactorAccess(binding formalQualifiedBinding, capability *formalValueFactorAccess) (product.Value, bool) {
	// Root reads dominate formal operand evaluation. Keep them outside the
	// closure-bearing composite interpreter so the apply view remains a stack
	// value and a one-node read stays allocation-free.
	arena, term := binding.value.arena, binding.value.term
	if e.ownsArena(binding.value.owner, arena, binding.apply) && arena != nil && arena.Sealed() && term != 0 && int(term) < len(arena.values) {
		if node := arena.values[term]; node.op == valueRoot {
			return e.evalArenaRootValue(binding.value.owner, arena, node.root, binding.scope, binding.apply)
		}
	}
	return e.evalArenaValueWithFactorAccess(binding.value.owner, binding.value.arena, binding.value.term, binding.scope, binding.apply, capability)
}

func (e formalTupleLeafEvaluator) evalArenaValue(owner relationVar, arena *Arena, term ValueTerm, scope loopMuTerm, apply formalApplyTermView) (product.Value, bool) {
	return e.evalArenaValueWithFactorAccess(owner, arena, term, scope, apply, nil)
}

type formalValueFactorAccess struct {
	access  state.TransferInputAccess
	factors []state.LaneFactor
}

func (e formalTupleLeafEvaluator) materializeValueFactorAccess(access state.TransferInputAccess, groups []formalFiberGroupDescriptor) (*formalValueFactorAccess, error) {
	if !e.valid() || len(groups) != access.Lanes.Len() {
		return nil, fmt.Errorf("transformer: formal value factor access is incomplete")
	}
	if len(groups) == 0 {
		return nil, nil
	}
	factors := make([]state.LaneFactor, len(groups))
	for index, group := range groups {
		if !access.Lanes.Has(group.lane.ID()) {
			return nil, fmt.Errorf("transformer: formal value factor group %q is undeclared", group.lane.ID())
		}
		factor, err := e.laneFactor(group)
		if err != nil {
			return nil, err
		}
		factors[index] = factor
	}
	return &formalValueFactorAccess{access: access, factors: factors}, nil
}

func (e formalTupleLeafEvaluator) evalArenaValueWithFactorAccess(owner relationVar, arena *Arena, term ValueTerm, scope loopMuTerm, apply formalApplyTermView, capability *formalValueFactorAccess) (product.Value, bool) {
	if !e.ownsArena(owner, arena, apply) || term == 0 || int(term) >= len(arena.values) || !arena.Sealed() {
		return product.Value{}, false
	}
	node := arena.values[term]
	if node.op == valueRoot {
		return e.evalArenaRootValue(owner, arena, node.root, scope, apply)
	}
	var resolver valueNodeLeafResolver
	resolver = valueNodeLeafResolver{
		guard: func(guard Guard) (bool, bool, bool) {
			return e.exactGuardPossibilities(owner, arena, scope, guard)
		},
		root: func(root Root) (product.Value, bool) {
			return e.evalArenaRootValue(owner, arena, root, scope, apply)
		},
		slot: func(slot statekey.Value) (product.Value, bool) {
			root, ok := e.rootForValueSlot(owner, slot)
			if !ok {
				return product.Value{}, false
			}
			return e.evalArenaRootValue(owner, arena, root, scope, apply)
		},
		dynamicRead: func(node valueNode, args []product.Value) (product.Value, bool) {
			if capability == nil || apply.present() || owner != e.variable || arena != e.authority.terms {
				return product.Value{}, false
			}
			required, err := e.body.valueTermFactorAccess(term)
			if err != nil {
				return product.Value{}, false
			}
			for _, id := range required.Lanes.IDs() {
				if !capability.access.Lanes.Has(id) {
					return product.Value{}, false
				}
			}
			return resolveFormalDynamicValue(e.body, e.span, node, args, capability.factors, func(child ValueTerm) (product.Value, bool) {
				return arena.evalValueCanonicalWithLeaves(child, resolver)
			})
		},
		allocationResult: func(candidate valueNode) (product.Value, bool) {
			return arena.allocationResult(candidate.allocation, candidate.resultIndex)
		},
	}
	return arena.evalValueCanonicalWithLeaves(term, resolver)
}

func (e formalTupleLeafEvaluator) exactGuardPossibilities(owner relationVar, arena *Arena, scope loopMuTerm, guard Guard) (bool, bool, bool) {
	if !e.valid() || e.algebra.program.formalGuards == nil || !e.algebra.program.formalGuards.valid() ||
		owner == 0 || arena == nil || guard == 0 || int(guard) >= len(arena.guards) {
		return false, false, false
	}
	decision, err := e.algebra.decisionForGuard(owner, scope, arena, guard)
	if err != nil {
		return false, false, false
	}
	not, err := formalDecisionBooleanNot(e.algebra, decision)
	if err != nil {
		return false, false, false
	}
	trueCare, err := e.algebra.decisions.apply(e.algebra.ctx, uint8(decisionAnd), true, e.guard, decision, decisionLeafAnd)
	if err != nil {
		return false, false, false
	}
	falseCare, err := e.algebra.decisions.apply(e.algebra.ctx, uint8(decisionAnd), true, e.guard, not, decisionLeafAnd)
	if err != nil {
		return false, false, false
	}
	return trueCare != decisionFalse, falseCare != decisionFalse, true
}

func (e formalTupleLeafEvaluator) evalArenaRootValue(owner relationVar, arena *Arena, root Root, scope loopMuTerm, apply formalApplyTermView) (product.Value, bool) {
	if !apply.present() {
		if owner != e.variable || arena != e.authority.terms {
			return product.Value{}, false
		}
		return e.valueAtRoot(root)
	}
	binding := apply.binding
	if owner != binding.target || arena != binding.targetArena || !binding.targetShape.validateInput(root) {
		// A borrowed target MID/result requires a target tuple leaf. Silently
		// reading an equally-numbered caller root would conflate lexical owners.
		return product.Value{}, false
	}
	ref, ok := binding.inputValue(root)
	if !ok || ref.owner != e.variable || ref.arena != e.authority.terms {
		return product.Value{}, false
	}
	return e.evalArenaValue(ref.owner, ref.arena, ref.term, apply.callerScope, formalApplyTermView{})
}

func (e formalTupleLeafEvaluator) valueAtRoot(root Root) (product.Value, bool) {
	if !e.valid() || e.algebra.program.formalSlots == nil {
		return product.Value{}, false
	}
	slot, ok := e.algebra.program.formalSlots.Slot(e.body.body, root)
	if !ok {
		return product.Value{}, false
	}
	index, ok := e.values.valueSlotPosition[slot]
	if !ok || index < 0 || index >= len(e.values.valueSlots) {
		return product.Value{}, false
	}
	top, present := e.leaves.leaf(e.values.valueTop)
	if !present {
		return product.Value{}, false
	}
	if top == 1 {
		return product.Top(), true
	}
	entry := e.values.valueSlots[index]
	leaf, present := e.leaves.leaf(entry.ordinal)
	if !present {
		return product.Value{}, false
	}
	if leaf == 0 {
		return product.Bottom(e.authority.product.Registry()), true
	}
	terminal, err := e.authority.terminal(leaf)
	if err != nil || terminal.kind != formalComponentGroundValue || !product.BelongsToRegistry(e.authority.product.Registry(), terminal.ground) {
		return product.Value{}, false
	}
	return terminal.ground, true
}

func (e formalTupleLeafEvaluator) evalQualifiedPath(binding formalQualifiedBinding) (pathdom.Path, bool) {
	return e.evalArenaPath(binding.path.owner, binding.path.arena, binding.path.term, binding.apply)
}

func (e formalTupleLeafEvaluator) evalArenaPath(owner relationVar, arena *Arena, term PathTerm, apply formalApplyTermView) (pathdom.Path, bool) {
	if !e.ownsArena(owner, arena, apply) || term == 0 || int(term) >= len(arena.paths) || !arena.Sealed() {
		return pathdom.Path{}, false
	}
	return arena.evalPathCanonicalWithRoot(term, func(root Root) (pathdom.Path, bool) {
		if !apply.present() {
			if owner != e.variable || arena != e.authority.terms {
				return pathdom.Path{}, false
			}
			return e.pathAtRoot(root)
		}
		binding := apply.binding
		if owner != binding.target || arena != binding.targetArena || !binding.targetShape.validateInput(root) {
			return pathdom.Path{}, false
		}
		ref, present, ok := binding.inputPath(root)
		if !ok || !present || ref.owner != e.variable || ref.arena != e.authority.terms {
			return pathdom.Path{}, false
		}
		return e.evalArenaPath(ref.owner, ref.arena, ref.term, formalApplyTermView{})
	})
}

func (e formalTupleLeafEvaluator) ownsArena(owner relationVar, arena *Arena, apply formalApplyTermView) bool {
	if !e.valid() || arena == nil {
		return false
	}
	if !apply.present() {
		return owner == e.variable && arena == e.authority.terms
	}
	binding := apply.binding
	return binding.validFor(e.variable, owner, binding.frame) && binding.callerArena == e.authority.terms &&
		binding.targetArena == arena
}

func (e formalTupleLeafEvaluator) pathAtRoot(root Root) (pathdom.Path, bool) {
	key, ok := e.body.rootPathKey(root)
	if !ok {
		return pathdom.Path{}, false
	}
	return e.body.keys.StatePath(key)
}

func (e formalTupleLeafEvaluator) rootForValueSlot(owner relationVar, slot statekey.Value) (Root, bool) {
	if !e.valid() || owner == 0 || int(owner) > len(e.algebra.program.bodies) || slot == 0 {
		return Root{}, false
	}
	body := &e.algebra.program.bodies[owner-1]
	// Environment values are evolving CFG-point state. A boundary symbol
	// therefore lawfully has two spellings: immutable IN provenance and the MID
	// register seeded from it. Value evaluation must select MID; treating the
	// pair as an ambiguity makes every captured/global environment term
	// unevaluable at Apply and Definition boundaries.
	if middle, ok := body.relation.arena.middleRoot(slot); ok {
		return middle, true
	}
	shape := body.relation.shape
	var found Root
	for _, kind := range []RootKind{RootParam, RootCapture, RootGlobal, RootAmbient, RootHeapTemplate} {
		count := shape.count(kind)
		for index := uint32(0); index < count; index++ {
			root := Root{Kind: kind, Index: index}
			candidate, ok := body.rootValueSlot(root)
			if !ok || candidate != slot {
				continue
			}
			if found.Kind != 0 {
				return Root{}, false
			}
			found = root
		}
	}
	return found, found.Kind != 0
}
