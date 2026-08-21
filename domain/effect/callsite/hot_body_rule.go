package callsite

import (
	"slices"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
)

const bodyOperandDomain = "wippy.analysis.effect.body-call.v1\x00"

// BodyHotRule is the interprocedural Effect reducer. Its operand is Effect's
// canonical mounted row; the one domain-owned body-role route table remains
// the cross-Factor join used by selection and transfer.
type BodyHotRule struct {
	implementation *effectowner.RuleImplementation[effectfactor.MountedCall]
	binding        *engine.SchemaBinding
	fragment       *BodySchemaFragment
	calls          *callowner.HotOwner
	effects        *effectowner.HotOwner
	callRead       engine.Read[engine.OrderedCells[calldomain.Value]]
	summary        engine.Read[engine.Selection[uint64, engine.OrderedCells[effectfactor.Value]]]
	routes         map[calldomain.TargetRoleID]uint32
	all            []bodyRoute
}

// BindBodyHot seals the sole body-role route table and binds the exact Call
// predecessor, dependent selected Effect summaries, and exact Effect output.
func BindBodyHot(binding *engine.SchemaBinding, fragment *BodySchemaFragment, calls *callowner.HotOwner, effects *effectowner.HotOwner) (*BodyHotRule, bool) {
	if binding == nil || fragment == nil || fragment.core == nil || fragment.core.slot == nil || calls == nil || !calls.MatchesBinding(binding) || calls.Algebra() == nil || effects == nil || !effects.MatchesBinding(binding) || effects.Algebra() == nil ||
		!calls.Algebra().LinkOwner().Matches(effects.Algebra().LinkOwner()) || !fragment.core.semantic.Available() {
		return nil, false
	}
	hot := &BodyHotRule{
		binding: binding, fragment: fragment, calls: calls, effects: effects,
		routes: make(map[calldomain.TargetRoleID]uint32),
	}
	bodies := calls.Algebra().Bodies()
	roles := make([]calldomain.TargetRoleID, 0, bodies.Count())
	collected := make([]bodyRoute, 0, bodies.Count())
	for index := 0; index < bodies.Count(); index++ {
		body, bodyOK := bodies.At(index)
		role, roleOK := body.RoleID()
		moduleKey, moduleOK := body.ModuleKey()
		programID, programOK := body.ProgramID()
		bodyID, bodyIDOK := body.BodyPath()
		root, rootOK := effects.Algebra().RootForMountedBodyID(moduleKey, programID, bodyID)
		rootIndex, indexOK := effects.Algebra().RootIndex(root)
		if !bodyOK || !moduleOK || !programOK || !bodyIDOK || !roleOK || role.Kind() != calldomain.TargetRoleBody || !rootOK || !indexOK || rootIndex < 0 {
			return nil, false
		}
		if _, duplicate := hot.routes[role]; duplicate {
			return nil, false
		}
		hot.routes[role] = 0
		roles = append(roles, role)
		collected = append(collected, bodyRoute{tag: uint64(rootIndex), root: root})
	}
	ordered, slots, orderOK := orderBodyRoutes(collected)
	if !orderOK || len(slots) != len(roles) {
		return nil, false
	}
	hot.all = ordered
	for index, role := range roles {
		hot.routes[role] = slots[index]
	}

	var runtimeCall engine.Read[engine.OrderedCells[calldomain.Value]]
	var runtimeSummary engine.Read[engine.Selection[uint64, engine.OrderedCells[effectfactor.Value]]]
	implementation, bound := effectowner.BindSelectedRuleDirect(effects, fragment.core.slot, fragment.core.write, engine.HotRuleSpec[effectfactor.Value, effectfactor.MountedCall]{
		OperandContent:  hot.operandContent,
		OperandResolver: hot.resolveOperand,
		Fold: func(frame engine.Frame[effectfactor.Value, effectfactor.MountedCall]) engine.RuleResult[effectfactor.Value] {
			return hot.fold(frame, runtimeCall, runtimeSummary)
		},
	}, func(mounted effectfactor.MountedCall) (uint64, bool) {
		_, _, root, ok := mountedCallRows(binding, calls, effects, mounted)
		index, indexOK := effects.Algebra().RootIndex(root)
		return uint64(index), ok && indexOK && index >= 0
	})
	if !bound {
		return nil, false
	}
	callRead, callOK := effectowner.AddSelectedRuleDirectExactRead(implementation, fragment.core.callRead, calls.FactorRef(), func(mounted effectfactor.MountedCall) (uint64, bool) {
		_, key, _, ok := mountedCallRows(binding, calls, effects, mounted)
		index, indexOK := calls.Algebra().KeyIndex(key)
		return uint64(index), ok && indexOK && index >= 0
	})
	if !callOK {
		return nil, false
	}
	summary, summaryOK := effectowner.AddSelectedRuleDirectOperandRead[effectfactor.MountedCall, effectfactor.Value, uint64](implementation, fragment.effectRead, effects.FactorRef(), hot.locate)
	if !summaryOK {
		return nil, false
	}
	hot.callRead, hot.summary = callRead, summary
	runtimeCall, runtimeSummary = callRead, summary
	hot.implementation = implementation
	return hot, true
}

func (rule *BodyHotRule) valid() bool {
	return rule != nil && rule.fragment != nil && rule.fragment.core != nil && rule.fragment.core.slot != nil && rule.implementation != nil &&
		callsiteOwnersValid(rule.binding, rule.calls, rule.effects)
}

func (rule *BodyHotRule) mountedForOccurrence(mount, occurrence identity.ContentID) (effectfactor.MountedCall, bool) {
	if !rule.valid() || !mount.Available() || !occurrence.Available() {
		return effectfactor.MountedCall{}, false
	}
	ordinal, ordinalOK := rule.effects.Algebra().MountedCallOrdinalForOccurrence(mount, occurrence)
	mounted, mountedOK := rule.effects.Algebra().MountedCallAt(ordinal)
	_, _, _, rowsOK := mountedCallRows(rule.binding, rule.calls, rule.effects, mounted)
	application, module, callOccurrence, identityOK := rule.effects.Algebra().MountedCallIdentity(mounted)
	return mounted, ordinalOK && ordinal >= 0 && mountedOK && rowsOK && identityOK && application.Available() && module == mount && callOccurrence == occurrence
}

func (rule *BodyHotRule) resolveOperand(coords engine.OperandCoords) (effectfactor.MountedCall, bool) {
	return rule.mountedForOccurrence(coords.Mount, coords.Occurrence)
}

func (rule *BodyHotRule) operandContent(mounted effectfactor.MountedCall) (effectfactor.MountedCall, [32]byte, bool) {
	if rule == nil {
		return effectfactor.MountedCall{}, [32]byte{}, false
	}
	_, key, root, rowsOK := mountedCallRows(rule.binding, rule.calls, rule.effects, mounted)
	if !rowsOK {
		return effectfactor.MountedCall{}, [32]byte{}, false
	}
	id, idOK := mountedOperandID(bodyOperandDomain, rule.calls.Algebra(), rule.effects.Algebra(), key, root)
	if !idOK {
		return effectfactor.MountedCall{}, [32]byte{}, false
	}
	return mounted, [32]byte(id), true
}

func (rule *BodyHotRule) locate(context engine.SelectorContext, mounted effectfactor.MountedCall) bool {
	if !rule.valid() {
		return false
	}
	_, key, _, siteOK := mountedCallRows(rule.binding, rule.calls, rule.effects, mounted)
	if !siteOK {
		return false
	}
	cells, readable := engine.SelectorRead(context, rule.callRead)
	if !readable || cells.Count() != 1 {
		return false
	}
	fact, present, available := cells.At(0)
	if !available {
		return false
	}
	if !present {
		return true
	}
	_, ok := rule.routesFor(key, fact, func(_ int, route bodyRoute) bool {
		return effectowner.SelectRouteTyped(rule.effects, context, route.root, route.tag)
	})
	return ok
}

// routesFor projects a Call value through the one sealed role table in the
// Selection order orderBodyRoutes fixed, so a route ordinal addresses the same
// staged route the engine published at that ordinal. Seed roles are owned by
// Selected/Opaque and skipped.
func (rule *BodyHotRule) routesFor(key calldomain.Key, value calldomain.Value, visit func(int, bodyRoute) bool) (int, bool) {
	if rule == nil || rule.calls == nil || !rule.calls.Algebra().Admits(key, value) || visit == nil {
		return 0, false
	}
	if value.IsTop() {
		for index, route := range rule.all {
			if !visit(index, route) {
				return 0, false
			}
		}
		return len(rule.all), true
	}
	var inline [8]uint32
	slots := inline[:0]
	if value.KnownTargetCount() > len(inline) {
		slots = make([]uint32, 0, value.KnownTargetCount())
	}
	for index := 0; index < value.KnownTargetCount(); index++ {
		target, targetOK := value.KnownTargetAt(index)
		role, roleOK := target.RoleID()
		if !targetOK || !roleOK {
			return 0, false
		}
		switch role.Kind() {
		case calldomain.TargetRoleSeed:
			continue
		case calldomain.TargetRoleBody:
			slot, found := rule.routes[role]
			if !found || slot == 0 || uint64(slot) > uint64(len(rule.all)) {
				return 0, false
			}
			slots = append(slots, slot)
		default:
			return 0, false
		}
	}
	slices.Sort(slots)
	for ordinal, slot := range slots {
		// A Selection carries one route per exact target, so two targets that
		// resolve to the same route have no second ordinal to address. The
		// projection is refused rather than folded twice.
		if ordinal > 0 && slot == slots[ordinal-1] {
			return 0, false
		}
		if !visit(ordinal, rule.all[slot-1]) {
			return 0, false
		}
	}
	return len(slots), true
}

func (rule *BodyHotRule) fold(frame engine.Frame[effectfactor.Value, effectfactor.MountedCall], callRead engine.Read[engine.OrderedCells[calldomain.Value]], summary engine.Read[engine.Selection[uint64, engine.OrderedCells[effectfactor.Value]]]) engine.RuleResult[effectfactor.Value] {
	if !rule.valid() {
		return engine.RuleResult[effectfactor.Value]{}
	}
	mounted, operandOK := engine.Operand(frame)
	_, key, root, siteOK := mountedCallRows(rule.binding, rule.calls, rule.effects, mounted)
	if !operandOK || !siteOK {
		return engine.RuleResult[effectfactor.Value]{}
	}
	callCells, callOK := engine.ReadValue(frame, callRead)
	selection, selectionOK := engine.ReadValue(frame, summary)
	if !callOK || !selectionOK || callCells.Count() != 1 {
		return engine.RuleResult[effectfactor.Value]{}
	}
	fact, present, available := callCells.At(0)
	if !available {
		return engine.RuleResult[effectfactor.Value]{}
	}
	selectionCount, countOK := engine.SelectionCount(frame, selection)
	if !countOK {
		return engine.RuleResult[effectfactor.Value]{}
	}
	if !present {
		if selectionCount != 0 {
			return engine.RuleResult[effectfactor.Value]{}
		}
		return engine.NoCandidate(frame)
	}
	atoms := make([]effectfactor.Atom, 0)
	top := false
	routeCount, routesOK := rule.routesFor(key, fact, func(ordinal int, route bodyRoute) bool {
		if ordinal >= selectionCount {
			return false
		}
		tag, cells, selected := engine.SelectionAt(frame, selection, ordinal)
		if !selected || tag != route.tag || cells.Count() != 1 {
			return false
		}
		part, partPresent, partAvailable := cells.At(0)
		if !partAvailable {
			return false
		}
		if !partPresent || top {
			return true
		}
		if rule.effects.Algebra().Equal(part, rule.effects.Algebra().Top()) {
			top = true
			return true
		}
		transported, transportOK := rule.effects.Algebra().Transport(part, root)
		if !transportOK {
			return false
		}
		for atomIndex := 0; ; atomIndex++ {
			atom, exists := rule.effects.Algebra().AtomAt(transported, atomIndex)
			if !exists {
				break
			}
			atoms = append(atoms, atom)
		}
		return true
	})
	if !routesOK || routeCount != selectionCount {
		return engine.RuleResult[effectfactor.Value]{}
	}
	if top {
		return engine.Staged(frame, rule.effects.Algebra().Top())
	}
	result, resultOK := rule.effects.Algebra().FromAtoms(atoms)
	if !resultOK {
		return engine.RuleResult[effectfactor.Value]{}
	}
	if rule.effects.Algebra().Equal(result, rule.effects.Algebra().Bottom()) {
		return engine.NoCandidate(frame)
	}
	return engine.Staged(frame, result)
}

func (rule *BodyHotRule) Implementation() (*effectowner.RuleImplementation[effectfactor.MountedCall], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := effectowner.ResolveRuleImplementationFor(rule.effects, rule.implementation)
	return rule.implementation, ok
}
