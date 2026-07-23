package factapply

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// CallOutcomeCorrelationBinding binds one capability-declared structural
// shape to its caller-space trigger and target paths. Product values are not
// part of this freeze-time address.
type CallOutcomeCorrelationBinding struct {
	Shape   callpayload.CallOutcomeCorrelationShape
	Trigger keyspace.Key
	Target  keyspace.Key
}

type callOutcomeCorrelationFactorBinding struct {
	template pathevidence.PathPresenceImplication
	slot     state.CoordinateSlot
}

// callOutcomeCorrelationAddress is the comparable execution-time identity of
// one capability-sealed correlation shape. Abstract target values are scalar
// payload and therefore deliberately absent.
type callOutcomeCorrelationAddress struct {
	kind            callpayload.CallOutcomeCorrelationKind
	returnIndex     int
	returnValue     bool
	target          pathdom.PathKey
	targetIndex     int
	triggerPresence presence.Value
	targetPresence  presence.Value
}

// CallOutcomeCorrelationFactorProgram publishes all three call correlation
// DTO fields into the one canonical path-evidence implication carrier.
type CallOutcomeCorrelationFactorProgram struct {
	domain    state.ProductDomain
	keys      *keyspace.KeySpace
	lane      state.ProductLane
	bindings  []callOutcomeCorrelationFactorBinding
	index     map[callOutcomeCorrelationAddress]int
	slots     []state.CoordinateSlot
	authority state.CoordinatePathEvidenceAuthority[statekey.Value]
}

// PrepareCallOutcomeCorrelationFactorProgramAtSite is the sole call-site
// binding law for provider correlations. It deliberately uses argument-only
// placeholders (a receiver never shifts $N here) and the same return-target
// inventory as concrete call application. Formal execution supplies the same
// BoundaryRoots relation after its ordinary structural rekey.
func PrepareCallOutcomeCorrelationFactorProgramAtSite(
	authority *PathSemanticAuthority,
	domain state.ProductDomain,
	facts factflow.Facts,
	site factflow.CallSiteView,
	point cfg.Point,
	shapes []callpayload.CallOutcomeCorrelationShape,
) (CallOutcomeCorrelationFactorProgram, error) {
	if authority == nil || !authority.Valid() || !domain.Valid() || point == 0 {
		return CallOutcomeCorrelationFactorProgram{}, fmt.Errorf("factapply: invalid call correlation site authority")
	}
	paths, err := authority.CallBoundaryPathBindings(facts, site)
	if err != nil {
		return CallOutcomeCorrelationFactorProgram{}, err
	}
	roots := make(state.BoundaryRoots, 0, site.ResultTargetCount())
	site.ForEachResultTarget(func(target factflow.CallResultTargetView) bool {
		if target.ResultIndex() < 0 || target.TargetPathEmpty() {
			return true
		}
		resolved, exact := callOutcomeKeyspaceKeyAt(authority.resolver, point, paths, target.TargetPathRef())
		if exact {
			roots = append(roots, state.BoundaryRoot{
				Slot: statekey.CallResult(uint32(point), uint32(target.ResultIndex())),
				Path: resolved,
			})
		}
		return true
	})
	return PrepareCallOutcomeCorrelationFactorProgramAtBoundary(authority, domain, point, paths, roots, shapes)
}

// PrepareCallOutcomeCorrelationFactorProgramAtBoundary binds provider return
// correlations through the same finite slot/path relation used by boundary
// transport. CallResult is the scalar identity; Path is its structural image.
// This keeps ret[N] as payload syntax only: correlation preparation neither
// parses nor manufactures a textual return root.
func PrepareCallOutcomeCorrelationFactorProgramAtBoundary(
	authority *PathSemanticAuthority,
	domain state.ProductDomain,
	point cfg.Point,
	paths callboundary.PathBindings,
	roots state.BoundaryRoots,
	shapes []callpayload.CallOutcomeCorrelationShape,
) (CallOutcomeCorrelationFactorProgram, error) {
	if authority == nil || !authority.Valid() || !domain.Valid() || point == 0 {
		return CallOutcomeCorrelationFactorProgram{}, fmt.Errorf("factapply: invalid call correlation boundary authority")
	}
	returns := make(map[int]keyspace.Key, len(roots))
	for index, root := range roots {
		owner, slot, exact := statekey.ParseCallResult(root.Slot)
		if !exact || owner != uint32(point) || root.Path.Kind == keyspace.KindInvalid ||
			authority.KeySpace().FormatReadOnly(root.Path) == "" {
			return CallOutcomeCorrelationFactorProgram{}, fmt.Errorf("factapply: invalid call correlation boundary root %d", index)
		}
		ordinal := int(slot)
		if prior, duplicate := returns[ordinal]; duplicate && prior != root.Path {
			return CallOutcomeCorrelationFactorProgram{}, fmt.Errorf("factapply: ambiguous call correlation return slot %d", ordinal)
		}
		returns[ordinal] = root.Path
	}
	resolveReturn := func(index int) (keyspace.Key, error) {
		resolved, ok := returns[index]
		if index < 0 || !ok {
			return keyspace.Key{}, fmt.Errorf("factapply: unresolved call correlation return slot %d", index)
		}
		return resolved, nil
	}
	resolve := func(path pathdom.Path) (keyspace.Key, error) {
		resolved, ok := callOutcomeKeyspaceKeyAt(authority.resolver, point, paths, path)
		if !ok {
			return keyspace.Key{}, fmt.Errorf("factapply: unresolved call correlation path %s", path.Key())
		}
		return resolved, nil
	}
	bindings := make([]CallOutcomeCorrelationBinding, len(shapes))
	for index, shape := range shapes {
		trigger, err := resolveReturn(shape.ReturnIndex)
		if err != nil {
			return CallOutcomeCorrelationFactorProgram{}, err
		}
		var target keyspace.Key
		switch shape.Kind {
		case callpayload.CallOutcomeReturnConditionPath:
			target, err = resolve(shape.Target)
		case callpayload.CallOutcomeReturnConditionSlot, callpayload.CallOutcomeReturnPresence:
			target, err = resolveReturn(shape.TargetIndex)
		default:
			err = fmt.Errorf("factapply: invalid call correlation shape %d", index)
		}
		if err != nil {
			return CallOutcomeCorrelationFactorProgram{}, err
		}
		bindings[index] = CallOutcomeCorrelationBinding{Shape: shape, Trigger: trigger, Target: target}
	}
	return PrepareCallOutcomeCorrelationFactorProgram(domain, authority.KeySpace(), bindings)
}

func PrepareCallOutcomeCorrelationFactorProgram(
	domain state.ProductDomain,
	keys *keyspace.KeySpace,
	bindings []CallOutcomeCorrelationBinding,
) (CallOutcomeCorrelationFactorProgram, error) {
	family, ok := domain.PathEvidenceCoordinateFamily()
	if !domain.Valid() || keys == nil || !keys.Valid() || !ok {
		return CallOutcomeCorrelationFactorProgram{}, fmt.Errorf("factapply: invalid call correlation factor authority")
	}
	out := CallOutcomeCorrelationFactorProgram{
		domain: domain, keys: keys, lane: family.Lane(),
		index: make(map[callOutcomeCorrelationAddress]int, len(bindings)),
	}
	for index, binding := range bindings {
		if binding.Trigger == (keyspace.Key{}) || binding.Target == (keyspace.Key{}) {
			return CallOutcomeCorrelationFactorProgram{}, fmt.Errorf("factapply: unresolved call correlation binding %d", index)
		}
		address, valid := callOutcomeCorrelationShapeAddress(binding.Shape)
		if !valid {
			return CallOutcomeCorrelationFactorProgram{}, fmt.Errorf("factapply: invalid call correlation shape %d", index)
		}
		if _, duplicate := out.index[address]; duplicate {
			return CallOutcomeCorrelationFactorProgram{}, fmt.Errorf("factapply: duplicate call correlation shape %d", index)
		}
		var template pathevidence.PathPresenceImplication
		switch binding.Shape.Kind {
		case callpayload.CallOutcomeReturnConditionPath, callpayload.CallOutcomeReturnConditionSlot:
			template = pathevidence.NewPathTruthinessValueRefinementImplication(binding.Trigger, binding.Shape.ReturnValue, binding.Target, product.Bottom(domain.Registry()))
			// Value is clause payload and is replaced before publication. The
			// structural coordinate constructor strips it at freeze.
		case callpayload.CallOutcomeReturnPresence:
			template = pathevidence.NewPathPresenceImplication(binding.Trigger, binding.Shape.TriggerPresence, binding.Target, binding.Shape.TargetPresence)
		default:
			return CallOutcomeCorrelationFactorProgram{}, fmt.Errorf("factapply: invalid call correlation shape %d", index)
		}
		slot, err := domain.PresenceImplicationCoordinateSlot(keys, template)
		if err != nil {
			return CallOutcomeCorrelationFactorProgram{}, err
		}
		out.index[address] = len(out.bindings)
		out.bindings = append(out.bindings, callOutcomeCorrelationFactorBinding{template: template, slot: slot})
		out.slots = appendUniqueCoordinateSlot(domain, out.slots, slot)
	}
	if err := sortPresenceCoordinateSlots(domain, out.slots); err != nil {
		return CallOutcomeCorrelationFactorProgram{}, err
	}
	inventory, err := domain.SealCoordinateFactorInventory(keys, out.slots)
	if err != nil {
		return CallOutcomeCorrelationFactorProgram{}, err
	}
	out.authority, err = state.SealCoordinatePathEvidenceAuthority(
		domain, keys, nil, nil, inventory, inventory, false, true,
		func(statekey.Value) bool { return false },
	)
	if err != nil {
		return CallOutcomeCorrelationFactorProgram{}, err
	}
	return out, nil
}

func (p CallOutcomeCorrelationFactorProgram) Lane() state.ProductLane { return p.lane }
func (p CallOutcomeCorrelationFactorProgram) CoordinateSlots() []state.CoordinateSlot {
	return append([]state.CoordinateSlot(nil), p.slots...)
}

// ReturnPresenceCoordinateSlots selects only the implication coordinates
// owned by one producer-result/presence consequence.  It is the structural
// counterpart of a CallResult carrier query: callers may transport these
// slots to a consuming occurrence without admitting sibling correlation
// shapes from the same call.
func (p CallOutcomeCorrelationFactorProgram) ReturnPresenceCoordinateSlots(
	returnIndex int,
	triggerPresence presence.Value,
) []state.CoordinateSlot {
	if returnIndex < 0 || (!presence.Equal(triggerPresence, presence.Present()) && !presence.Equal(triggerPresence, presence.Absent())) {
		return nil
	}
	var out []state.CoordinateSlot
	for address, index := range p.index {
		if address.kind != callpayload.CallOutcomeReturnPresence || address.returnIndex != returnIndex ||
			!presence.Equal(address.triggerPresence, triggerPresence) || index < 0 || index >= len(p.bindings) {
			continue
		}
		out = append(out, p.bindings[index].slot)
	}
	if len(out) > 1 {
		_ = sortPresenceCoordinateSlots(p.domain, out)
	}
	return out
}

func (p CallOutcomeCorrelationFactorProgram) Apply(factor state.LaneFactor, outcome callpayload.CallOutcome) (state.LaneFactor, error) {
	if !p.domain.Valid() || p.keys == nil || factor.Lane() != p.lane {
		return state.LaneFactor{}, fmt.Errorf("factapply: invalid call correlation factor")
	}
	var rows []pathevidence.PathPresenceImplication
	appendRow := func(shape callpayload.CallOutcomeCorrelationShape, value any) error {
		address, valid := callOutcomeCorrelationShapeAddress(shape)
		index, declared := p.index[address]
		if !valid || !declared {
			return fmt.Errorf("factapply: undeclared call correlation outcome")
		}
		binding := p.bindings[index]
		row := binding.template
		switch typed := value.(type) {
		case callpayload.CallReturnConditionRefinement:
			row.TargetValue = typed.Value
		case callpayload.CallReturnConditionSlotRefinement:
			row.TargetValue = typed.Value
		case callpayload.CallReturnPresenceRelation:
		default:
			return fmt.Errorf("factapply: invalid call correlation payload")
		}
		rows = append(rows, row)
		return nil
	}
	for _, value := range outcome.ReturnConditionRefinements {
		if err := appendRow(callpayload.ReturnConditionPathShape(value.ReturnIndex, value.ReturnValue, value.Target), value); err != nil {
			return factor, err
		}
	}
	for _, value := range outcome.ReturnConditionSlots {
		if err := appendRow(callpayload.ReturnConditionSlotShape(value.ReturnIndex, value.ReturnValue, value.TargetIndex), value); err != nil {
			return factor, err
		}
	}
	for _, value := range outcome.ReturnPresenceRelations {
		if err := appendRow(callpayload.ReturnPresenceShape(value.TriggerIndex, value.TriggerPresence, value.TargetIndex, value.TargetPresence), value); err != nil {
			return factor, err
		}
	}
	if len(rows) == 0 {
		return factor, nil
	}
	canonical, ok := pathevidence.CanonicalPathPresenceImplications(p.domain.Registry(), p.keys, rows)
	if !ok {
		return factor, fmt.Errorf("factapply: invalid call correlation clauses")
	}
	family, _ := p.domain.PathEvidenceCoordinateFamily()
	skeleton, scalars, err := p.domain.DecomposeCoordinateFamily(factor, family, p.keys)
	if err != nil {
		return factor, err
	}
	carrier, err := p.domain.OpenCoordinatePathEvidenceCarrier(
		skeleton, scalars, state.ValueLaneFactor{}, true, p.authority, state.PathDescendantMutationFactors{},
	)
	if err != nil {
		return factor, err
	}
	for _, row := range canonical {
		if _, valid := carrier.AddImplication(row); !valid {
			return factor, fmt.Errorf("factapply: call correlation publication rejected")
		}
	}
	nextSkeleton, nextScalars, _, _, _, _, err := carrier.Freeze()
	if err != nil {
		return factor, err
	}
	return p.domain.ReplaceCoordinateFamily(factor, nextSkeleton, nextScalars)
}

func callOutcomeCorrelationShapeAddress(shape callpayload.CallOutcomeCorrelationShape) (callOutcomeCorrelationAddress, bool) {
	if shape.ReturnIndex < 0 {
		return callOutcomeCorrelationAddress{}, false
	}
	address := callOutcomeCorrelationAddress{
		kind: shape.Kind, returnIndex: shape.ReturnIndex, returnValue: shape.ReturnValue,
		targetIndex: shape.TargetIndex, triggerPresence: shape.TriggerPresence, targetPresence: shape.TargetPresence,
	}
	switch shape.Kind {
	case callpayload.CallOutcomeReturnConditionPath:
		if shape.Target.IsEmpty() || shape.TargetIndex != 0 {
			return callOutcomeCorrelationAddress{}, false
		}
		address.target = shape.Target.Key()
	case callpayload.CallOutcomeReturnConditionSlot:
		if !shape.Target.IsEmpty() || shape.TargetIndex < 0 {
			return callOutcomeCorrelationAddress{}, false
		}
	case callpayload.CallOutcomeReturnPresence:
		if !shape.Target.IsEmpty() || shape.TargetIndex < 0 ||
			(!presence.Equal(shape.TriggerPresence, presence.Present()) && !presence.Equal(shape.TriggerPresence, presence.Absent())) ||
			(!presence.Equal(shape.TargetPresence, presence.Present()) && !presence.Equal(shape.TargetPresence, presence.Absent())) {
			return callOutcomeCorrelationAddress{}, false
		}
	default:
		return callOutcomeCorrelationAddress{}, false
	}
	return address, true
}
