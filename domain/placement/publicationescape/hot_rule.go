package publicationescape

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// HotRule is the mounted Effect publication-escape consumer. The retained
// authorities are owner-fenced; no raw Target, Pack, Effect Factor, or
// runtime context enters a selector or fold callback.
type HotRule struct {
	program       engine.ProgramRule
	pending       *placementowner.RuleImplementation[effectfactor.MountedPublicationBatch]
	owner         *placementowner.HotOwner
	values        *valueowner.HotOwner
	calls         *callowner.HotOwner
	effects       *effectowner.HotOwner
	batchIndex    *effectfactor.MountedPublicationBatchIndex
	preparedByID  map[identity.ContentID]*preparedBatch
	callRead      engine.Read[engine.OrderedCells[calldomain.Value]]
	valueRead     engine.Read[engine.Selection[sourceTag, engine.OrderedCells[valuedomain.Value]]]
	placementRead engine.Read[engine.Selection[routeTag, engine.OrderedCells[placementdomain.Placement]]]
}

// BindHot binds the exact Call/Value predecessors and Placement route write.
// Effect is retained only as the authority that issued the sealed operand
// batch; the batch itself is not a new Factor read.
func BindHot(
	binding *engine.SchemaBinding,
	fragment *SchemaFragment,
	owner *placementowner.HotOwner,
	values *valueowner.HotOwner,
	calls *callowner.HotOwner,
	effects *effectowner.HotOwner,
) (*HotRule, bool) {
	if binding == nil || fragment == nil || fragment.slot == nil || owner == nil || !owner.MatchesBinding(binding) ||
		values == nil || !values.MatchesBinding(binding) || calls == nil || !calls.MatchesBinding(binding) ||
		effects == nil || !effects.MatchesBinding(binding) || !owner.Schema().Valid() || values.Schema() == nil || !values.Schema().Valid() ||
		calls.Algebra() == nil || !calls.Algebra().Valid() || effects.Algebra() == nil || !effects.Algebra().Valid() ||
		!values.Schema().OwnsHeapSchema(owner.Schema().Heap()) {
		return nil, false
	}
	linkOwner := calls.Algebra().LinkOwner()
	if !linkOwner.Available() || !values.Schema().LinkOwner().Matches(linkOwner) || !owner.Schema().Heap().LinkOwner().Matches(linkOwner) || !effects.Algebra().LinkOwner().Matches(linkOwner) {
		return nil, false
	}
	batchIndex, indexOK := effectfactor.NewMountedPublicationBatchIndex(effects.Algebra())
	if !indexOK || batchIndex == nil || !batchIndex.Valid() {
		return nil, false
	}
	rule := &HotRule{
		owner:        owner,
		values:       values,
		calls:        calls,
		effects:      effects,
		batchIndex:   batchIndex,
		preparedByID: make(map[identity.ContentID]*preparedBatch, batchIndex.Count()),
	}
	for index := 0; index < batchIndex.Count(); index++ {
		batch, batchOK := batchIndex.BatchAt(index)
		if !batchOK {
			return nil, false
		}
		prepared, preparedOK := rule.prepareBatch(batch)
		if !preparedOK {
			return nil, false
		}
		if _, duplicate := rule.preparedByID[prepared.id]; duplicate {
			return nil, false
		}
		rule.preparedByID[prepared.id] = prepared
	}
	pending, ok := placementowner.BindSelectedRouteRuleDirect(owner, fragment.slot, fragment.carry, fragment.write, owner.FactorRef(), engine.HotRuleSpec[placementdomain.Placement, effectfactor.MountedPublicationBatch]{
		OperandContent:  rule.operandContent,
		OperandResolver: rule.resolveOperand,
		Fold:            rule.fold,
	}, engine.HotCarrySpec[placementdomain.Placement, effectfactor.MountedPublicationBatch]{}, nil)
	if !ok || pending == nil {
		return nil, false
	}
	callRead, callOK := placementowner.AddSelectedRuleDirectExactRead(pending, fragment.callRead, calls.FactorRef(), rule.projectCall)
	if !callOK {
		return nil, false
	}
	valueRead, valueOK := placementowner.AddSelectedRuleDirectOperandRead[effectfactor.MountedPublicationBatch, valuedomain.Value, sourceTag](pending, fragment.valueRead, values.FactorRef(), rule.locateValues)
	if !valueOK {
		return nil, false
	}
	placementRead, placementOK := placementowner.AddSelectedRuleDirectOperandRead[effectfactor.MountedPublicationBatch, placementdomain.Placement, routeTag](pending, fragment.placementRead, owner.FactorRef(), rule.locatePlacement)
	if !placementOK {
		return nil, false
	}
	rule.pending, rule.callRead, rule.valueRead, rule.placementRead = pending, callRead, valueRead, placementRead
	return rule, true
}

func (rule *HotRule) operandContent(batch effectfactor.MountedPublicationBatch) (effectfactor.MountedPublicationBatch, [32]byte, bool) {
	if rule == nil || rule.preparedFor(batch) == nil {
		return effectfactor.MountedPublicationBatch{}, [32]byte{}, false
	}
	id, ok := batch.SealedContentID()
	if !ok || !id.Available() {
		return effectfactor.MountedPublicationBatch{}, [32]byte{}, false
	}
	return batch, [32]byte(id), true
}

func (rule *HotRule) preparedFor(batch effectfactor.MountedPublicationBatch) *preparedBatch {
	if rule == nil || rule.batchIndex == nil || rule.preparedByID == nil || !rule.batchIndex.Owns(batch) {
		return nil
	}
	id, ok := batch.SealedContentID()
	if !ok {
		return nil
	}
	prepared := rule.preparedByID[id]
	if prepared == nil || prepared.id != id {
		return nil
	}
	return prepared
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (effectfactor.MountedPublicationBatch, bool) {
	if rule == nil || rule.batchIndex == nil || !coords.Mount.Available() || !coords.Occurrence.Available() {
		return effectfactor.MountedPublicationBatch{}, false
	}
	batch, batchOK := rule.batchIndex.BatchForCall(coords.Mount, coords.Occurrence)
	if !batchOK || rule.preparedFor(batch) == nil {
		return effectfactor.MountedPublicationBatch{}, false
	}
	return batch, true
}

func (rule *HotRule) projectCall(batch effectfactor.MountedPublicationBatch) (uint64, bool) {
	if rule == nil || rule.calls == nil || rule.calls.Algebra() == nil {
		return 0, false
	}
	key, keyOK := rule.callKeyForBatch(batch)
	if !keyOK {
		return 0, false
	}
	index, indexOK := rule.calls.Algebra().KeyIndex(key)
	return uint64(index), indexOK && index >= 0
}

func (rule *HotRule) locatePlacement(context engine.SelectorContext, batch effectfactor.MountedPublicationBatch) bool {
	value, present, callOK := rule.callValueSelector(context, batch)
	if !callOK {
		return false
	}
	prepared := rule.preparedFor(batch)
	if prepared == nil || rule.owner == nil {
		return false
	}
	if !present {
		return true
	}
	gate, gateOK := rule.operationGateForBatch(prepared, value)
	if !gateOK {
		return false
	}
	selection, selectionOK := engine.SelectorRead(context, rule.valueRead)
	if !selectionOK {
		return false
	}
	sources := prepared.sourcesForGate(gate)
	facts, factsOK := rule.collectFacts(context, sources, selection)
	if !factsOK {
		return false
	}
	routes, routesOK := rule.routeSet(rule.owner.Schema(), prepared, gate, facts)
	if !routesOK {
		return false
	}
	for index := 0; index < routes.len(); index++ {
		route, routeOK := routes.at(index)
		if !routeOK {
			return false
		}
		if !placementowner.SelectRouteTyped(rule.owner, context, route.key, route.tag) {
			return false
		}
	}
	return true
}

func (rule *HotRule) fold(frame engine.Frame[placementdomain.Placement, effectfactor.MountedPublicationBatch]) engine.RuleResult[placementdomain.Placement] {
	batch, batchOK := engine.Operand(frame)
	if !batchOK || rule == nil || rule.owner == nil {
		return engine.RuleResult[placementdomain.Placement]{}
	}
	placementSelection, placementOK := engine.ReadValue(frame, rule.placementRead)
	callValue, present, callOK := rule.callValueFrame(frame, batch)
	if !placementOK || !callOK {
		return engine.RuleResult[placementdomain.Placement]{}
	}
	if !present {
		count, countOK := engine.SelectionCount(frame, placementSelection)
		if !countOK || count != 0 {
			return engine.RuleResult[placementdomain.Placement]{}
		}
		return engine.NoSelection(frame, placementSelection)
	}
	prepared := rule.preparedFor(batch)
	if prepared == nil {
		return engine.RuleResult[placementdomain.Placement]{}
	}
	gate, gateOK := rule.operationGateForBatch(prepared, callValue)
	if !gateOK {
		return engine.RuleResult[placementdomain.Placement]{}
	}
	valueSelection, valueOK := engine.ReadValue(frame, rule.valueRead)
	if !valueOK {
		return engine.RuleResult[placementdomain.Placement]{}
	}
	sources := prepared.sourcesForGate(gate)
	facts, factsOK := rule.collectFrameFacts(frame, sources, valueSelection)
	if !factsOK {
		return engine.RuleResult[placementdomain.Placement]{}
	}
	routes, routesOK := rule.routeSet(rule.owner.Schema(), prepared, gate, facts)
	if !routesOK {
		return engine.RuleResult[placementdomain.Placement]{}
	}
	count, countOK := engine.SelectionCount(frame, placementSelection)
	if !countOK || count != routes.len() {
		return engine.RuleResult[placementdomain.Placement]{}
	}
	if count == 0 {
		return engine.NoSelection(frame, placementSelection)
	}
	return engine.Routed(frame, placementSelection, func(tag routeTag, cells engine.OrderedCells[placementdomain.Placement]) (placementdomain.Placement, bool) {
		var route plannedRoute
		found := false
		for index := 0; index < routes.len(); index++ {
			candidate, candidateOK := routes.at(index)
			if !candidateOK {
				return placementdomain.Bottom, false
			}
			if candidate.tag == tag {
				route, found = candidate, true
				break
			}
		}
		if !found || cells.Count() != 1 {
			return placementdomain.Bottom, false
		}
		current, present, available := cells.At(0)
		if !available {
			return placementdomain.Bottom, false
		}
		if !present {
			current = placementdomain.Bottom
		}
		return applyRoute(route, current), true
	})
}

// Finalize seals the owner wrapper after SchemaBinding reaches its sealed
// phase and publishes the opaque engine ProgramRule.
func (rule *HotRule) Finalize(owner *placementowner.HotOwner) bool {
	if rule == nil || owner == nil || rule.pending == nil || rule.program.Available() || rule.owner != owner {
		return false
	}
	implementation, ok := placementowner.ResolveRuleImplementationFor(owner, rule.pending)
	if !ok || implementation == nil {
		return false
	}
	program, ok := engine.SealProgramRule(implementation)
	if !ok {
		return false
	}
	rule.program = program
	return true
}

// Implementation returns the sealed engine issuer for composition.
func (rule *HotRule) Implementation() (engine.ProgramRule, bool) {
	return rule.program, rule != nil && rule.program.Available()
}

// SealProgramRule publishes the exact mounted ProgramRule primitive.
func SealProgramRule(rule *HotRule) (engine.ProgramRule, bool) {
	if rule == nil || !rule.program.Available() {
		return engine.ProgramRule{}, false
	}
	return rule.program, true
}
