package callsite

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
)

// hotOperand is the runtime-only Callsite operand. It retains no Project
// Application or Program proof; those are consumed while issuing receipt.
type hotOperand struct {
	receipt *callsiteReceipt
	key     calldomain.Key
	root    effectfactor.Root
	id      identity.ContentID
}

func newHotOperand(effects *effectfactor.Algebra, calls *calldomain.Algebra, root effectfactor.Root, key calldomain.Key) (hotOperand, bool) {
	if effects == nil || !effects.Valid() || calls == nil || !calls.Valid() || !effects.LinkOwner().Matches(calls.LinkOwner()) || !calls.OwnsKey(key) || !effects.Admit(root, effects.Bottom()) {
		return hotOperand{}, false
	}
	callID, callOK := key.ContentID()
	rootID, rootOK := effects.RootID(root)
	if !callOK || !rootOK || !callID.Available() || !rootID.Available() {
		return hotOperand{}, false
	}
	const prefix = "wippy.analysis.effect.callsite.v2\x00"
	var payload [len(prefix) + 2*sha256.Size]byte
	copy(payload[:], prefix)
	copy(payload[len(prefix):], callID[:])
	copy(payload[len(prefix)+sha256.Size:], rootID[:])
	id := identity.ContentID(sha256.Sum256(payload[:]))
	return hotOperand{key: key, root: root, id: id}, id.Available()
}

// callsiteTargetReceipt is the seal-time meaning of one Call seed role for a
// particular mounted call. Selected rows retain exact beta receipts; opaque
// rows retain only their already-issued unknown atom. Invalid Target rows are
// retained explicitly so they fail only when selected, matching the semantic
// Rule rather than rejecting an unrelated application receipt.
type callsiteTargetReceipt struct {
	bindings     []effectfactor.AtomBinding
	publications []effectfactor.PublicationAtomBinding
	unknown      effectfactor.Atom
	unknownKnown bool
	applicable   bool
	valid        bool
}

// callsiteReceipt is the immutable hot operand proof for Selected or Opaque.
// The target map is built once from Call's stable seed roles. Hot transfer
// never opens Boundary, Target, Pack, Project, Program, or Flow.
type callsiteReceipt struct {
	owner             *HotRule
	binding           *engine.SchemaBinding
	key               calldomain.Key
	root              effectfactor.Root
	targets           map[calldomain.TargetRoleID]callsiteTargetReceipt
	opaqueAlternative effectfactor.Atom
	id                [32]byte
	opaque            bool
	sealed            bool
}

func (receipt *callsiteReceipt) valid() bool {
	return receipt != nil && receipt.sealed && receipt.owner != nil && receipt.binding != nil && receipt.owner.binding == receipt.binding && receipt.binding.Sealed() &&
		receipt.owner.calls != nil && receipt.owner.calls.Algebra() != nil && receipt.owner.effects != nil && receipt.owner.effects.Algebra() != nil &&
		receipt.owner.calls.Algebra().LinkOwner().Matches(receipt.owner.effects.Algebra().LinkOwner()) && receipt.key.Valid() &&
		receipt.owner.effects.Algebra().Admit(receipt.root, receipt.owner.effects.Algebra().Bottom()) && receipt.targets != nil && receipt.id != [32]byte{}
}

func (rule *HotRule) operandContent(value hotOperand) (hotOperand, [32]byte, bool) {
	receipt := value.receipt
	if rule == nil || !receipt.valid() || receipt.owner != rule || [32]byte(value.id) != receipt.id || value.key != receipt.key || value.root != receipt.root {
		return hotOperand{}, [32]byte{}, false
	}
	return value, receipt.id, true
}

// HotRule is the receipt-native Selected or Opaque Call-to-Effect Rule. The
// exact read capability and pending implementation remain owned by the
// package; callers may only issue and attach opaque operands.
type HotRule struct {
	implementation *effectowner.RuleImplementation[hotOperand]
	binding        *engine.SchemaBinding
	fragment       *schemaFragment[hotOperand]
	calls          *callowner.HotOwner
	effects        *effectowner.HotOwner
	read           engine.Read[engine.OrderedCells[calldomain.Value]]
	opaque         bool
	receiptsSealed bool
	receipts       []hotOperand
}

func BindSelectedHot(binding *engine.SchemaBinding, fragment *SelectedSchemaFragment, calls *callowner.HotOwner, effects *effectowner.HotOwner) (*HotRule, bool) {
	if fragment == nil {
		return nil, false
	}
	return bindHot(binding, fragment.core, calls, effects, false)
}

func BindOpaqueHot(binding *engine.SchemaBinding, fragment *OpaqueSchemaFragment, calls *callowner.HotOwner, effects *effectowner.HotOwner) (*HotRule, bool) {
	if fragment == nil {
		return nil, false
	}
	return bindHot(binding, fragment.core, calls, effects, true)
}

func bindHot(binding *engine.SchemaBinding, fragment *schemaFragment[hotOperand], calls *callowner.HotOwner, effects *effectowner.HotOwner, opaque bool) (*HotRule, bool) {
	if binding == nil || fragment == nil || fragment.slot == nil || calls == nil || !calls.MatchesBinding(binding) || calls.Algebra() == nil || effects == nil || !effects.MatchesBinding(binding) || effects.Algebra() == nil ||
		!calls.Algebra().LinkOwner().Matches(effects.Algebra().LinkOwner()) || !fragment.semantic.Available() || !fragment.evidence.Available() {
		return nil, false
	}
	hot := &HotRule{binding: binding, fragment: fragment, calls: calls, effects: effects, opaque: opaque}
	var runtimeRead engine.Read[engine.OrderedCells[calldomain.Value]]
	implementation, read, ok := effectowner.BindExactReadRule(effects, fragment.slot, fragment.callRead, calls.FactorRef(), fragment.write, engine.HotRuleSpec[effectfactor.Value, hotOperand]{
		OperandContent: hot.operandContent,
		Admission:      engine.AdmitRuleByDerivation(fragment.evidence, hot.check),
		Transfer: func(access engine.Access[effectfactor.Value, hotOperand]) bool {
			value, operandOK := engine.Operand(access)
			if !operandOK || !hot.accepts(value) {
				return false
			}
			return engine.Product(access, func(row engine.Row) bool {
				cells, readOK := engine.ReadValue(access, row, runtimeRead)
				if !readOK || cells.Count() != 1 {
					return false
				}
				fact, present, available := cells.At(0)
				if !available {
					return false
				}
				if !present {
					return engine.NoCandidate(access, row)
				}
				result, resultOK := hot.reduce(value.receipt, fact)
				if !resultOK {
					return false
				}
				if hot.effects.Algebra().Equal(result, hot.effects.Algebra().Bottom()) {
					return engine.NoCandidate(access, row)
				}
				return engine.StageValue(access, row, result)
			})
		},
	}, func(operand hotOperand) (uint64, bool) {
		index, ok := calls.Algebra().KeyIndex(operand.key)
		return uint64(index), ok && index >= 0
	}, func(operand hotOperand) (uint64, bool) {
		index, ok := effects.Algebra().RootIndex(operand.root)
		return uint64(index), ok && index >= 0
	})
	if !ok || implementation == nil {
		return nil, false
	}
	runtimeRead = read
	hot.implementation, hot.read = implementation, read
	if !implementation.InstallOperandResolver(hot.resolveOperand) {
		return nil, false
	}
	return hot, true
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (hotOperand, bool) {
	return rule.receiptForOccurrence(coords.Mount, coords.Occurrence)
}

// Receipt consumes Project's exact mounted-call proof and issues all Target
// and beta proofs once after the shared SchemaBinding has sealed.
func (rule *HotRule) Receipt(mounted effectfactor.MountedCall) (hotOperand, bool) {
	if rule == nil || rule.binding == nil || !rule.binding.Sealed() || rule.calls == nil || rule.effects == nil {
		return hotOperand{}, false
	}
	calls, effects := rule.calls.Algebra(), rule.effects.Algebra()
	applicationID, _, _, mountedOK := effects.MountedCallIdentity(mounted)
	key, keyOK := calls.KeyForApplicationID(applicationID)
	root, rootOK := effects.RootForMountedCall(mounted)
	base, baseOK := newHotOperand(effects, calls, root, key)
	if !mountedOK || !keyOK || !rootOK || !baseOK {
		return hotOperand{}, false
	}
	receipt := &callsiteReceipt{
		owner: rule, binding: rule.binding, key: base.key, root: base.root,
		targets: make(map[calldomain.TargetRoleID]callsiteTargetReceipt), id: [32]byte(base.id), opaque: rule.opaque,
	}
	seeds := calls.Seeds()
	for index := 0; index < seeds.Count(); index++ {
		target, targetOK := seeds.At(index)
		role, roleOK := target.RoleID()
		if !targetOK || !roleOK || role.Kind() != calldomain.TargetRoleSeed {
			return hotOperand{}, false
		}
		operation, operationOK := target.Operation()
		row := callsiteTargetReceipt{applicable: operationOK}
		if operationOK && rule.opaque {
			part, partOK := effects.SelectedMountedCallOpaque(base.root, mounted, operation)
			row.valid = partOK
			if partOK {
				row.unknown, row.unknownKnown = effects.AtomAt(part, 0)
				if _, extra := effects.AtomAt(part, 1); extra {
					return hotOperand{}, false
				}
			}
		} else if operationOK {
			row.bindings, row.valid = effects.SelectedCallEffectBindings(base.root, mounted, operation)
			if row.valid {
				row.publications, row.valid = effects.SelectedCallPublicationAtomBindings(base.root, mounted, operation)
			}
		}
		if _, duplicate := receipt.targets[role]; duplicate {
			return hotOperand{}, false
		}
		receipt.targets[role] = row
	}
	if rule.opaque {
		receipt.opaqueAlternative, rootOK = effects.MountedCallOpaqueUnknown(base.root, calls, mounted, calls.Top())
		if !rootOK {
			return hotOperand{}, false
		}
	}
	receipt.sealed = true
	if !receipt.valid() {
		return hotOperand{}, false
	}
	base.receipt = receipt
	return base, true
}

func (rule *HotRule) accepts(value hotOperand) bool {
	receipt := value.receipt
	return rule != nil && rule.binding != nil && rule.calls != nil && rule.effects != nil && receipt.valid() && receipt.binding == rule.binding &&
		receipt.owner == rule && receipt.opaque == rule.opaque &&
		value.key == receipt.key && value.root == receipt.root && [32]byte(value.id) == receipt.id
}

func (rule *HotRule) reduce(receipt *callsiteReceipt, value calldomain.Value) (effectfactor.Value, bool) {
	if rule == nil || !receipt.valid() || receipt.owner != rule || receipt.opaque != rule.opaque || !rule.calls.Algebra().Admits(receipt.key, value) {
		return effectfactor.Value{}, false
	}
	if value.IsTop() {
		return rule.effects.Algebra().Top(), true
	}
	atoms := make([]effectfactor.Atom, 0)
	for index := 0; index < value.KnownTargetCount(); index++ {
		target, targetOK := value.KnownTargetAt(index)
		role, roleOK := target.RoleID()
		if !targetOK || !roleOK {
			return effectfactor.Value{}, false
		}
		if role.Kind() == calldomain.TargetRoleBody {
			continue
		}
		row, found := receipt.targets[role]
		if !found {
			return effectfactor.Value{}, false
		}
		if !row.applicable {
			continue
		}
		if !row.valid {
			return effectfactor.Value{}, false
		}
		if rule.opaque {
			if row.unknownKnown {
				atoms = append(atoms, row.unknown)
			}
			continue
		}
		for _, binding := range row.bindings {
			atom, atomOK := binding.Atom()
			if !atomOK {
				return effectfactor.Value{}, false
			}
			atoms = append(atoms, atom)
		}
	}
	if rule.opaque && value.HasOpaqueAlternative() {
		atoms = append(atoms, receipt.opaqueAlternative)
	}
	return rule.effects.Algebra().FromAtoms(atoms)
}

func (rule *HotRule) check(derivation engine.RuleDerivation[effectfactor.Value, hotOperand]) (engine.RuleEvidence, bool) {
	if rule == nil || rule.fragment == nil || derivation.Rule() != rule.fragment.semantic || derivation.InputCount() != 1 || derivation.ReadCount() != 1 || derivation.DispositionCount() == 0 {
		return engine.RuleEvidence{}, false
	}
	value, operandOK := derivation.Operand()
	input, inputOK := derivation.InputAt(0)
	if !operandOK || !rule.accepts(value) || !derivation.OperandContentMatches(value.receipt.id) || !inputOK || input.Guard().Empty() ||
		!callowner.ReadMatches(rule.calls, derivation, rule.read, value.receipt.key) {
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
		cells, cellsOK := engine.DerivationDispositionReadValue(derivation, disposition, rule.read)
		if !cellsOK || cells.Count() != 1 {
			return engine.RuleEvidence{}, false
		}
		fact, present, available := cells.At(0)
		if !available {
			return engine.RuleEvidence{}, false
		}
		if !present {
			if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
				return engine.RuleEvidence{}, false
			}
			continue
		}
		expected, expectedOK := rule.reduce(value.receipt, fact)
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

func (rule *HotRule) Implementation() (*effectowner.RuleImplementation[hotOperand], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := effectowner.ResolveRuleImplementationFor(rule.effects, rule.implementation)
	return rule.implementation, ok
}

// SealProgramRule is this typed rule's schema registration. It is the only
// place the private owner issuer is converted to the engine primitive.
func SealProgramRule(rule *HotRule) (engine.ProgramRule, bool) {
	if rule == nil {
		return engine.ProgramRule{}, false
	}
	implementation, ok := effectowner.ResolveRuleImplementationFor(rule.effects, rule.implementation)
	if !ok {
		return engine.ProgramRule{}, false
	}
	return engine.SealProgramRule(implementation)
}
