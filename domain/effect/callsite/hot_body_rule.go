package callsite

import (
	"crypto/sha256"
	"slices"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
)

// hotBodyOperand is the runtime-only body-summary operand. Mounted Project
// and Program proofs are fully consumed while issuing its receipt.
type hotBodyOperand struct {
	receipt *bodyCallReceipt
	key     calldomain.Key
	root    effectfactor.Root
	id      identity.ContentID
}

func newHotBodyOperand(effects *effectfactor.Algebra, calls *calldomain.Algebra, root effectfactor.Root, key calldomain.Key) (hotBodyOperand, bool) {
	if effects == nil || !effects.Valid() || calls == nil || !calls.Valid() || !effects.LinkOwner().Matches(calls.LinkOwner()) || !calls.OwnsKey(key) || !effects.Admit(root, effects.Bottom()) {
		return hotBodyOperand{}, false
	}
	callID, callOK := key.ContentID()
	rootID, rootOK := effects.RootID(root)
	if !callOK || !rootOK || !callID.Available() || !rootID.Available() {
		return hotBodyOperand{}, false
	}
	const prefix = "wippy.analysis.effect.body-call.v1\x00"
	var payload [len(prefix) + 2*sha256.Size]byte
	copy(payload[:], prefix)
	copy(payload[len(prefix):], callID[:])
	copy(payload[len(prefix)+sha256.Size:], rootID[:])
	id := identity.ContentID(sha256.Sum256(payload[:]))
	return hotBodyOperand{key: key, root: root, id: id}, id.Available()
}

// bodyCallReceipt is one immutable mounted caller operand. Body routes are
// deliberately absent: BodyHotRule seals the sole cross-Factor route table
// once, rather than copying every possible callee into every application.
type bodyCallReceipt struct {
	owner   *BodyHotRule
	binding *engine.SchemaBinding
	key     calldomain.Key
	root    effectfactor.Root
	id      [32]byte
	sealed  bool
}

func (receipt *bodyCallReceipt) valid() bool {
	return receipt != nil && receipt.sealed && receipt.owner != nil && receipt.binding != nil && receipt.owner.binding == receipt.binding && receipt.binding.Sealed() &&
		receipt.owner.calls != nil && receipt.owner.calls.Algebra() != nil && receipt.owner.effects != nil && receipt.owner.effects.Algebra() != nil &&
		receipt.owner.calls.Algebra().LinkOwner().Matches(receipt.owner.effects.Algebra().LinkOwner()) && receipt.key.Valid() &&
		receipt.owner.effects.Algebra().Admit(receipt.root, receipt.owner.effects.Algebra().Bottom()) && receipt.id != [32]byte{}
}

func (rule *BodyHotRule) operandContent(value hotBodyOperand) (hotBodyOperand, [32]byte, bool) {
	receipt := value.receipt
	if rule == nil || !receipt.valid() || receipt.owner != rule || [32]byte(value.id) != receipt.id || value.key != receipt.key || value.root != receipt.root {
		return hotBodyOperand{}, [32]byte{}, false
	}
	return value, receipt.id, true
}

// BodyHotRule is the receipt-native interprocedural Effect transport. The
// route table is the one sealed join of Call body roles to Effect roots. Hot
// selection and transfer use only that table, exact Factor receipts, and the
// already-admitted Call value; they never reopen Project, Program, or Flow.
type BodyHotRule struct {
	implementation      *effectowner.RuleImplementation[hotBodyOperand]
	binding             *engine.SchemaBinding
	fragment            *BodySchemaFragment
	calls               *callowner.HotOwner
	effects             *effectowner.HotOwner
	callRead            engine.Read[engine.OrderedCells[calldomain.Value]]
	summary             engine.Read[engine.Selection[uint64, engine.OrderedCells[effectfactor.Value]]]
	routes              map[calldomain.TargetRoleID]uint32
	all                 []bodyRoute
	receiptsSealed      bool
	receipts            []hotBodyOperand
}

// BindBodyHot seals the sole body-role route table and binds the exact Call
// predecessor, dependent selected Effect summaries, and exact Effect output.
func BindBodyHot(binding *engine.SchemaBinding, fragment *BodySchemaFragment, calls *callowner.HotOwner, effects *effectowner.HotOwner) (*BodyHotRule, bool) {
	if binding == nil || fragment == nil || fragment.core == nil || fragment.core.slot == nil || calls == nil || !calls.MatchesBinding(binding) || calls.Algebra() == nil || effects == nil || !effects.MatchesBinding(binding) || effects.Algebra() == nil ||
		!calls.Algebra().LinkOwner().Matches(effects.Algebra().LinkOwner()) || !fragment.core.semantic.Available() || !fragment.core.evidence.Available() {
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
	implementation, bound := effectowner.BindSelectedRuleDirect(effects, fragment.core.slot, fragment.core.write, engine.HotRuleSpec[effectfactor.Value, hotBodyOperand]{
		OperandContent: hot.operandContent,
		Admission:      engine.AdmitRuleByDerivation(fragment.core.evidence, hot.check),
		Transfer: func(access engine.Access[effectfactor.Value, hotBodyOperand]) bool {
			return hot.transfer(access, runtimeCall, runtimeSummary)
		},
	}, func(operand hotBodyOperand) (uint64, bool) {
		index, ok := effects.Algebra().RootIndex(operand.root)
		return uint64(index), ok && index >= 0
	})
	if !bound {
		return nil, false
	}
	callRead, callOK := effectowner.AddSelectedRuleDirectExactRead(implementation, fragment.core.callRead, calls.FactorRef(), func(operand hotBodyOperand) (uint64, bool) {
		index, ok := calls.Algebra().KeyIndex(operand.key)
		return uint64(index), ok && index >= 0
	})
	if !callOK {
		return nil, false
	}
	summary, summaryOK := effectowner.AddSelectedRuleDirectOperandRead[hotBodyOperand, effectfactor.Value, uint64](implementation, fragment.effectRead, effects.FactorRef(), hot.locate)
	if !summaryOK {
		return nil, false
	}
	hot.callRead, hot.summary = callRead, summary
	runtimeCall, runtimeSummary = callRead, summary
	hot.implementation = implementation
	if !implementation.InstallOperandResolver(hot.resolveOperand) {
		return nil, false
	}
	return hot, true
}

func (rule *BodyHotRule) resolveOperand(coords engine.OperandCoords) (hotBodyOperand, bool) {
	return rule.receiptForOccurrence(coords.Mount, coords.Occurrence)
}

// Receipt consumes one exact mounted caller proof after the shared binding
// has sealed. The returned operand contains no Project or Program capability.
func (rule *BodyHotRule) Receipt(mounted effectfactor.MountedCall) (hotBodyOperand, bool) {
	if rule == nil || rule.binding == nil || !rule.binding.Sealed() || rule.calls == nil || rule.effects == nil {
		return hotBodyOperand{}, false
	}
	applicationID, _, _, mountedOK := rule.effects.Algebra().MountedCallIdentity(mounted)
	key, keyOK := rule.calls.Algebra().KeyForApplicationID(applicationID)
	root, rootOK := rule.effects.Algebra().RootForMountedCall(mounted)
	base, baseOK := newHotBodyOperand(rule.effects.Algebra(), rule.calls.Algebra(), root, key)
	if !mountedOK || !keyOK || !rootOK || !baseOK {
		return hotBodyOperand{}, false
	}
	receipt := &bodyCallReceipt{
		owner: rule, binding: rule.binding, key: base.key, root: base.root,
		id: [32]byte(base.id), sealed: true,
	}
	if !receipt.valid() {
		return hotBodyOperand{}, false
	}
	base.receipt = receipt
	return base, true
}

func (rule *BodyHotRule) accepts(value hotBodyOperand) bool {
	receipt := value.receipt
	return rule != nil && rule.binding != nil && rule.calls != nil && rule.effects != nil && receipt.valid() && receipt.binding == rule.binding &&
		receipt.owner == rule && value.key == receipt.key && value.root == receipt.root &&
		[32]byte(value.id) == receipt.id
}

func (rule *BodyHotRule) locate(context engine.SelectorContext, value hotBodyOperand) bool {
	if rule == nil || !rule.accepts(value) {
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
	_, ok := rule.routesFor(value.receipt.key, fact, func(_ int, route bodyRoute) bool {
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

func (rule *BodyHotRule) transfer(access engine.Access[effectfactor.Value, hotBodyOperand], callRead engine.Read[engine.OrderedCells[calldomain.Value]], summary engine.Read[engine.Selection[uint64, engine.OrderedCells[effectfactor.Value]]]) bool {
	value, operandOK := engine.Operand(access)
	if !operandOK || !rule.accepts(value) {
		return false
	}
	return engine.Product(access, func(row engine.Row) bool {
		callCells, callOK := engine.ReadValue(access, row, callRead)
		selection, selectionOK := engine.ReadValue(access, row, summary)
		if !callOK || !selectionOK || callCells.Count() != 1 {
			return false
		}
		fact, present, available := callCells.At(0)
		if !available {
			return false
		}
		selectionCount, countOK := engine.SelectionCount(access, row, selection)
		if !countOK {
			return false
		}
		if !present {
			return selectionCount == 0 && engine.NoCandidate(access, row)
		}
		atoms := make([]effectfactor.Atom, 0)
		top := false
		routeCount, routesOK := rule.routesFor(value.receipt.key, fact, func(ordinal int, route bodyRoute) bool {
			if ordinal >= selectionCount {
				return false
			}
			tag, cells, selected := engine.SelectionAt(access, row, selection, ordinal)
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
			transported, transportOK := rule.effects.Algebra().Transport(part, value.receipt.root)
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
			return false
		}
		if top {
			return engine.StageValue(access, row, rule.effects.Algebra().Top())
		}
		result, resultOK := rule.effects.Algebra().FromAtoms(atoms)
		if !resultOK {
			return false
		}
		if rule.effects.Algebra().Equal(result, rule.effects.Algebra().Bottom()) {
			return engine.NoCandidate(access, row)
		}
		return engine.StageValue(access, row, result)
	})
}

func (rule *BodyHotRule) reduceEvidence(derivation engine.RuleDerivation[effectfactor.Value, hotBodyOperand], disposition engine.RuleDisposition[effectfactor.Value], value hotBodyOperand, fact calldomain.Value) (effectfactor.Value, bool) {
	count, countOK := engine.DerivationDispositionSelectionCount(derivation, disposition, rule.summary)
	if !countOK {
		return effectfactor.Value{}, false
	}
	atoms := make([]effectfactor.Atom, 0)
	top := false
	routeCount, routesOK := rule.routesFor(value.receipt.key, fact, func(ordinal int, route bodyRoute) bool {
		if ordinal >= count {
			return false
		}
		tag, cells, selected := engine.DerivationDispositionSelectionAt(derivation, disposition, rule.summary, ordinal)
		if !selected || tag != route.tag || cells.Count() != 1 || !effectowner.SelectionMatches(rule.effects, derivation, disposition, rule.summary, ordinal, route.root) {
			return false
		}
		part, present, available := cells.At(0)
		if !available {
			return false
		}
		if !present || top {
			return true
		}
		if rule.effects.Algebra().Equal(part, rule.effects.Algebra().Top()) {
			top = true
			return true
		}
		transported, transportOK := rule.effects.Algebra().Transport(part, value.receipt.root)
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
	if !routesOK || routeCount != count {
		return effectfactor.Value{}, false
	}
	if top {
		return rule.effects.Algebra().Top(), true
	}
	return rule.effects.Algebra().FromAtoms(atoms)
}

func (rule *BodyHotRule) check(derivation engine.RuleDerivation[effectfactor.Value, hotBodyOperand]) (engine.RuleEvidence, bool) {
	// A derivation read is one resolved observation, not one declared read
	// slot. This rule resolves the exact Call read plus the Effect summary of
	// every route its Product selected, so its read surface is bounded by the
	// sealed route table: a call value denoting every body reaches every
	// route, and one denoting none reaches only the Call read.
	readCount := derivation.ReadCount()
	if rule == nil || rule.fragment == nil || rule.fragment.core == nil || derivation.Rule() != rule.fragment.core.semantic || derivation.InputCount() != 1 || readCount < 1 || readCount > 1+len(rule.all) || derivation.DispositionCount() == 0 {
		return engine.RuleEvidence{}, false
	}
	value, operandOK := derivation.Operand()
	input, inputOK := derivation.InputAt(0)
	if !operandOK || !rule.accepts(value) || !derivation.OperandContentMatches(value.receipt.id) || !inputOK || input.Guard().Empty() ||
		!callowner.ReadMatches(rule.calls, derivation, rule.callRead, value.receipt.key) {
		return engine.RuleEvidence{}, false
	}
	for index := 0; index < derivation.DispositionCount(); index++ {
		disposition, dispositionOK := derivation.DispositionAt(index)
		if !dispositionOK || disposition.Guard().Empty() {
			return engine.RuleEvidence{}, false
		}
		if _, transformed := disposition.CarryTransform(); transformed || disposition.TransformOnly() {
			return engine.RuleEvidence{}, false
		}
		callCells, callOK := engine.DerivationDispositionReadValue(derivation, disposition, rule.callRead)
		if !callOK || callCells.Count() != 1 {
			return engine.RuleEvidence{}, false
		}
		fact, present, available := callCells.At(0)
		if !available {
			return engine.RuleEvidence{}, false
		}
		if !present {
			count, selected := engine.DerivationDispositionSelectionCount(derivation, disposition, rule.summary)
			if !selected || count != 0 || disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
				return engine.RuleEvidence{}, false
			}
			continue
		}
		expected, expectedOK := rule.reduceEvidence(derivation, disposition, value, fact)
		if !expectedOK {
			return engine.RuleEvidence{}, false
		}
		if rule.effects.Algebra().Equal(expected, rule.effects.Algebra().Bottom()) {
			if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
				return engine.RuleEvidence{}, false
			}
			continue
		}
		actual, actualOK := disposition.Value()
		target, targetOK := disposition.TargetAt(0)
		if disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || !actualOK || !targetOK ||
			!rule.effects.Algebra().Equal(actual, expected) || !rule.effects.TargetMatches(target, value.receipt.root) {
			return engine.RuleEvidence{}, false
		}
	}
	return derivation.Accept()
}

func (rule *BodyHotRule) Implementation() (*effectowner.RuleImplementation[hotBodyOperand], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := effectowner.ResolveRuleImplementationFor(rule.effects, rule.implementation)
	return rule.implementation, ok
}

// SealProgramRule is this typed rule's schema registration.
func SealBodyProgramRule(rule *BodyHotRule) (engine.ProgramRule, bool) {
	if rule == nil {
		return engine.ProgramRule{}, false
	}
	implementation, ok := effectowner.ResolveRuleImplementationFor(rule.effects, rule.implementation)
	if !ok {
		return engine.ProgramRule{}, false
	}
	return engine.SealProgramRule(implementation)
}
