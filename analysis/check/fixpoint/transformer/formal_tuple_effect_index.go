package transformer

import (
	"fmt"
	"math"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

type formalIndexMutationStep struct {
	table, keyPath, valuePath keyspace.Key
	structural                factapply.ResolvedStructuralPath
	tableRoot                 Root
	tableSymbol               symbol.ID
	key, value                ValueTerm
	admission                 dynamicindex.Admission
	appendMode                bool
	point                     cfg.Point
	demands                   []formalQualifiedGuardDemand
	valueAccess               state.TransferInputAccess
	effectDelta               state.EffectDeltaFactorPlan
	variable                  relationVar
}

func freezeFormalIndexMutationStep(program *RelationProgram, variable relationVar, operator formalRelationOperatorRef) (*formalIndexMutationStep, error) {
	if program == nil || variable == 0 || int(variable) > len(program.bodies) || operator.kind != formalRelationCellStep || operator.code == nil || operator.root == 0 || operator.step == 0 || int(operator.root) >= len(operator.code.nodes) || int(operator.step) > len(operator.code.nodes[operator.root].steps) {
		return nil, nil
	}
	step := operator.code.nodes[operator.root].steps[operator.step-1]
	if step.kind != boundaryStepEffect || step.effect == 0 || operator.code.effects == nil || int(step.effect) >= len(operator.code.effects.nodes) {
		return nil, nil
	}
	node := operator.code.effects.nodes[step.effect]
	if node.kind != EffectIndexMutation {
		return nil, nil
	}
	if node.invalidation.Scope != InvalidationScopeDescendants || node.invalidation.Precise != nil || !node.invalidation.PreserveStructuralWitness || !node.invalidation.PreserveDynamicValueMemberships || node.table.kind != effectTargetPath || node.table.path == 0 || node.invalidation.Target != node.table || node.readback != factflow.DynamicIndexReadbackKeyAndValue {
		return nil, fmt.Errorf("transformer: formal IndexMutation requires the exact direct N3+N4 shape")
	}
	body := &program.bodies[variable-1]
	span, ok := program.formalFibers.span(variable)
	if !ok || body.relation.code != operator.code || operator.code.terms == nil || int(node.table.path) >= len(operator.code.terms.paths) {
		return nil, fmt.Errorf("transformer: formal IndexMutation has no formal owner")
	}
	table, tablePath, err := freezeFormalEffectPath(body, span, node.table.path)
	if err != nil {
		return nil, err
	}
	if tablePath.Symbol == 0 {
		return nil, fmt.Errorf("transformer: formal IndexMutation table is not exact")
	}
	structural, err := factapply.FreezeResolvedStructuralPath(span.keys, table, tablePath.Symbol)
	if err != nil {
		return nil, err
	}
	plan := &formalIndexMutationStep{
		table: table, structural: structural, tableRoot: operator.code.terms.paths[node.table.path].root,
		tableSymbol: tablePath.Symbol, key: node.key, value: node.value, admission: node.admission,
		appendMode: node.appendMode, point: step.point, variable: variable,
	}
	plan.valueAccess, err = body.valueTermFactorAccess(node.key, node.value)
	if err != nil {
		return nil, err
	}
	plan.effectDelta, err = body.productDomain.PrepareEffectDeltaFactorPlan(effectdelta.Key{
		Target: table, Site: callboundary.PathStructuralPreservingInvalidationEffectSite(), Kind: effectdelta.Mutation,
	}, effectdelta.Top())
	if err != nil {
		return nil, err
	}
	if node.keyPath != 0 {
		plan.keyPath, err = freezeFormalEffectPathKey(body, span, node.keyPath)
		if err != nil {
			return nil, err
		}
	}
	if node.valuePath != 0 {
		plan.valuePath, err = freezeFormalEffectPathKey(body, span, node.valuePath)
		if err != nil {
			return nil, err
		}
	}
	guards, err := reachableValueTermGuards(operator.code.terms, node.key)
	if err != nil {
		return nil, err
	}
	more, err := reachableValueTermGuards(operator.code.terms, node.value)
	if err != nil {
		return nil, err
	}
	guards = append(guards, more...)
	if step.guard != 0 {
		guards = append(guards, step.guard)
	}
	plan.demands = make([]formalQualifiedGuardDemand, len(guards))
	for index, guard := range guards {
		plan.demands[index] = formalQualifiedGuardDemand{owner: variable, scope: operator.scope, arena: operator.code.terms, guard: guard}
	}
	return plan, nil
}

type formalIndexPathReader struct {
	domain    state.ProductDomain
	keys      *keyspace.KeySpace
	evaluator formalTupleLeafEvaluator
	root      Root
	symbol    symbol.ID
	factors   map[state.ProductLane]state.LaneFactor
}

func (r formalIndexPathReader) ReadRootValue(id symbol.ID) (product.Value, bool) {
	if id != r.symbol {
		return product.Value{}, false
	}
	return r.evaluator.valueAtRoot(r.root)
}
func (r formalIndexPathReader) ReadLocalPathValue(path keyspace.Key) (product.Value, bool) {
	family, ok := r.domain.PathValueFamily()
	if !ok {
		return product.Value{}, false
	}
	value, present, err := r.domain.ReadPathValueFactor(r.factors[family.Lane()], r.keys, path)
	return value, present && err == nil
}
func (r formalIndexPathReader) ReadDynamicIndexTable(table keyspace.Key) (state.DynamicIndexTableEvidence, bool) {
	lane, ok := r.domain.ProductLane(state.LaneDynamicIndex)
	if !ok {
		return state.DynamicIndexTableEvidence{}, false
	}
	value, err := r.domain.ObserveDynamicIndexTableFactor(r.factors[lane], table)
	return value, err == nil
}
func (r formalIndexPathReader) ReadHeapObject(term identity.Term) (heapidentity.TableObject, bool) {
	lane, ok := r.domain.ProductLane(state.LaneHeapTableIdentity)
	if !ok {
		return heapidentity.BottomObject(r.domain.Registry()), false
	}
	value, err := r.domain.ReadHeapTableObjectTermFactor(r.factors[lane], term)
	return value, err == nil
}

func (a *formalTupleAlgebra) applyFormalIndexMutation(operator formalRelationOperatorRef, predecessor formalRelationTuple) (formalRelationTuple, error) {
	plan := operator.indexMutation
	if plan == nil || plan.variable != predecessor.variable {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal IndexMutation is unbound")
	}
	return a.applyFormalEffectStep(operator, predecessor, plan.demands, func(span formalFiberDescriptorSpan, evaluator formalTupleLeafEvaluator) ([]decisionLeaf, error) {
		return a.applyFormalIndexMutationLeaf(operator, span, evaluator, plan)
	})
}

func (a *formalTupleAlgebra) applyFormalIndexMutationLeaf(operator formalRelationOperatorRef, span formalFiberDescriptorSpan, evaluator formalTupleLeafEvaluator, plan *formalIndexMutationStep) ([]decisionLeaf, error) {
	if plan == nil || !evaluator.valid() || plan.variable != span.variable {
		return nil, errFormalComponentForeignOwner
	}
	domain := evaluator.authority.product
	_, original, err := evaluator.productFactors()
	if err != nil {
		return nil, err
	}
	capability := &formalValueFactorAccess{access: plan.valueAccess, factors: original}
	keyValue, ok := evaluator.evalArenaValueWithFactorAccess(span.variable, operator.code.terms, plan.key, operator.scope, formalApplyTermView{}, capability)
	if !ok {
		return nil, fmt.Errorf("transformer: formal IndexMutation key is unresolved")
	}
	storedValue, ok := evaluator.evalArenaValueWithFactorAccess(span.variable, operator.code.terms, plan.value, operator.scope, formalApplyTermView{}, capability)
	if !ok {
		return nil, fmt.Errorf("transformer: formal IndexMutation value is unresolved")
	}
	lanes := domain.NonValuesLaneInventory()
	current := append([]state.LaneFactor(nil), original...)
	positions := make(map[state.ProductLane]int, len(lanes))
	for index, lane := range lanes {
		positions[lane] = index
	}
	pathFamily, ok := domain.PathValueFamily()
	if !ok {
		return nil, errFormalComponentMalformed
	}
	pathIndex := positions[pathFamily.Lane()]
	pathSkeleton, pathScalars, err := domain.DecomposeCoordinateFamily(current[pathIndex], pathFamily, span.keys)
	if err != nil {
		return nil, err
	}
	invalidation, err := domain.PrepareCoordinatePathDescendantMutation(pathSkeleton, pathScalars, pathdom.PathKey(span.keys.FormatReadOnly(plan.table)))
	if err != nil {
		return nil, err
	}
	changed := make(map[int]bool)
	for _, lane := range domain.PathDescendantMutationParticipantLanes() {
		index := positions[lane]
		current[index], err = domain.ApplyPathDescendantMutationLane(invalidation, current[index])
		if err != nil {
			return nil, err
		}
		changed[index] = true
	}

	fact := dynamicindex.NewFact(domain.Registry(), dynamicindex.FactConfig{
		KeyValue: keyValue, HasKeyValue: true, Value: storedValue, HasValue: true, Admission: plan.admission,
	})
	definitelyPresent := factapply.DynamicIndexFactDefinitelyPresent(domain.Registry(), fact)
	definitelyAbsent := factapply.DynamicIndexFactDefinitelyAbsent(domain.Registry(), fact)
	membershipLane, ok := domain.ProductLane(state.LaneKeyMemberships)
	if !ok {
		return nil, errFormalComponentMalformed
	}
	membershipIndex := positions[membershipLane]
	query := state.DynamicIndexMembershipEvidenceQuery{Container: plan.table, TableStateKeys: []pathaddr.StateKey{pathaddr.StateKey(span.keys.FormatReadOnly(plan.table))}}
	if plan.keyPath.Kind != keyspace.KindInvalid {
		query.KeyStateKey = pathaddr.StateKey(span.keys.FormatReadOnly(plan.keyPath))
	}
	if plan.valuePath.Kind != keyspace.KindInvalid {
		query.SourceStateKeys = []pathaddr.StateKey{pathaddr.StateKey(span.keys.FormatReadOnly(plan.valuePath))}
	}
	evidence, err := domain.ObserveDynamicIndexMutationEvidence(original[membershipIndex], current[membershipIndex], query)
	if err != nil {
		return nil, err
	}
	restoreKeys := []pathaddr.StateKey(nil)
	if query.KeyStateKey != "" {
		restoreKeys = append(restoreKeys, query.KeyStateKey)
		equivalent, queryErr := domain.EquivalentPathStateKeysFactor(current[pathIndex], span.keys, plan.keyPath)
		if queryErr != nil {
			return nil, queryErr
		}
		restoreKeys = append(restoreKeys, equivalent...)
	}
	membershipPlan, err := domain.PrepareDynamicIndexMembershipFactorPlan(span.keys, state.DynamicIndexMembershipFactorConfig{
		Key: dynamicindex.Key{Table: plan.table, Site: dynamicindex.SiteForPoint(int(plan.point))}, Fact: fact,
		TableStateKeys: query.TableStateKeys, AllValueTables: evidence.AllValueTables,
		PendingRestores: evidence.PendingRestores, RestoreKeys: restoreKeys,
		KeyStateKey: query.KeyStateKey, MembershipTable: pathaddr.StateKey(span.keys.FormatReadOnly(plan.table)),
		SourceMemberships: evidence.SourceMemberships, TableSymbol: plan.tableSymbol,
		HasKeyStateKey: query.KeyStateKey != "", DefinitelyPresent: definitelyPresent,
		DefinitelyAbsent: definitelyAbsent, MayBeAbsent: !definitelyPresent,
	})
	if err != nil {
		return nil, err
	}
	for _, lane := range domain.DynamicIndexMembershipFactorLanes() {
		index := positions[lane]
		current[index], err = domain.ApplyDynamicIndexMembershipFactor(membershipPlan, current[index])
		if err != nil {
			return nil, err
		}
		changed[index] = true
	}

	factorMap := make(map[state.ProductLane]state.LaneFactor, len(lanes))
	for index, lane := range lanes {
		factorMap[lane] = current[index]
	}
	reader := formalIndexPathReader{domain: domain, keys: span.keys, evaluator: evaluator, root: plan.tableRoot, symbol: plan.tableSymbol, factors: factorMap}
	tableValue, resolved := factapply.ResolveStructuralPathFactorValue(domain.Registry(), reader, plan.structural)
	if resolved {
		if tableTerm, exact := identityvalue.ExactTerm(domain.Registry(), tableValue); exact {
			placementLane, _ := domain.ProductLane(state.LanePlacement)
			owner, readErr := domain.ReadPlacementTermFactor(current[positions[placementLane]], tableTerm)
			if readErr != nil {
				return nil, readErr
			}
			if owner == placement.OwnedHeap || owner == placement.SharedHeap || owner == placement.Unknown {
				reachability, prepareErr := domain.PreparePlacementReachabilityPlan(span.keys, []product.Value{storedValue}, owner)
				if prepareErr != nil {
					return nil, prepareErr
				}
				reachabilityLanes, lanesErr := domain.PlacementReachabilityLanes(reachability)
				if lanesErr != nil {
					return nil, lanesErr
				}
				selected := make([]state.LaneFactor, len(reachabilityLanes))
				for index, lane := range reachabilityLanes {
					selected[index] = current[positions[lane]]
				}
				selected, err = domain.ApplyPlacementReachabilityFactors(a.ctx, reachability, selected)
				if err != nil {
					return nil, err
				}
				for index, lane := range reachabilityLanes {
					position := positions[lane]
					current[position] = selected[index]
					changed[position] = true
				}
			}
			heapLane, _ := domain.ProductLane(state.LaneHeapTableIdentity)
			heapIndex := positions[heapLane]
			object, readErr := domain.ReadHeapTableObjectTermFactor(current[heapIndex], tableTerm)
			if readErr != nil {
				return nil, readErr
			}
			if !heapidentity.ObjectDomain(domain.Registry()).Equal(object, heapidentity.BottomObject(domain.Registry())) {
				dynamic := object.DynamicIndexFacts()
				if dynamic == nil {
					dynamic = make(map[dynamicindex.Key]dynamicindex.Fact, 1)
				}
				key := dynamicindex.Key{Table: plan.table, Site: dynamicindex.SiteForPoint(int(plan.point))}
				if prior, exists := dynamic[key]; exists {
					dynamic[key] = dynamicindex.Domain(domain.Registry()).Join(prior, fact)
				} else {
					dynamic[key] = fact
				}
				replacement := heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: object.Root(), StaticMembers: object.StaticMembers(), DynamicIndexFacts: dynamic})
				graph, prepareErr := domain.PrepareObjectGraphReplacePlan(span.keys, []state.ObjectGraphMutation{{Identity: tableTerm, Object: replacement}})
				if prepareErr != nil {
					return nil, prepareErr
				}
				for _, lane := range domain.ObjectMutationParticipantLanes() {
					index := positions[lane]
					current[index], err = domain.ApplyObjectGraphMutationFactor(graph, current[index])
					if err != nil {
						return nil, err
					}
					changed[index] = true
				}
			}
		}
	}
	if plan.appendMode {
		lengthFamily, ok := domain.LenFloorCoordinateFamily()
		if !ok {
			return nil, errFormalComponentMalformed
		}
		lengthIndex := positions[lengthFamily.Lane()]
		if floor, present, readErr := domain.ReadLengthFloorFactor(original[lengthIndex], span.keys, plan.table); readErr != nil {
			return nil, readErr
		} else if present && floor < math.MaxInt64 {
			lengthPlan, prepareErr := domain.PrepareLengthFloorFactorPlan(span.keys, plan.table, floor+1)
			if prepareErr != nil {
				return nil, prepareErr
			}
			current[lengthIndex], err = domain.ApplyLengthFloorFactor(lengthPlan, current[lengthIndex])
			if err != nil {
				return nil, err
			}
			changed[lengthIndex] = true
		}
	}
	for _, lane := range domain.EffectDeltaFactorLanes() {
		index := positions[lane]
		current[index], err = domain.ApplyEffectDeltaFactor(plan.effectDelta, current[index])
		if err != nil {
			return nil, err
		}
		changed[index] = true
	}

	complete, err := evaluator.completeLeaves()
	if err != nil {
		return nil, err
	}
	out := append([]decisionLeaf(nil), complete...)
	for index := range changed {
		group := evaluator.layout.nonValues[index]
		if err := a.factorFormalEffectGroup(evaluator.authority, span, group, state.ValueFactor[FormalSlot]{}, current[index], out); err != nil {
			return nil, err
		}
	}
	return out, nil
}
