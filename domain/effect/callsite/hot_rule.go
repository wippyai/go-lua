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

const selectedOperandDomain = "wippy.analysis.effect.callsite.v2\x00"

// callsiteOwnersValid is the common owner fence for the three Call-to-Effect
// rules. A mounted row is useful here only after both Factors belong to the
// exact sealed binding and Link.
func callsiteOwnersValid(binding *engine.SchemaBinding, calls *callowner.HotOwner, effects *effectowner.HotOwner) bool {
	return binding != nil && binding.Sealed() && calls != nil && calls.MatchesBinding(binding) && calls.Algebra() != nil && calls.Algebra().Valid() &&
		effects != nil && effects.MatchesBinding(binding) && effects.Algebra() != nil && effects.Algebra().Valid() &&
		calls.Algebra().LinkOwner().Matches(effects.Algebra().LinkOwner())
}

// mountedCallRows joins Effect's canonical mounted operand to Call's exact
// mounted row, application key, and Effect root. No joined row is retained:
// every caller receives only the existing owner-issued values.
func mountedCallRows(binding *engine.SchemaBinding, calls *callowner.HotOwner, effects *effectowner.HotOwner, mounted effectfactor.MountedCall) (calldomain.MountedCall, calldomain.Key, effectfactor.Root, bool) {
	if !callsiteOwnersValid(binding, calls, effects) {
		return calldomain.MountedCall{}, calldomain.Key{}, effectfactor.Root{}, false
	}
	callAlgebra, effectAlgebra := calls.Algebra(), effects.Algebra()
	application, module, occurrence, effectIdentityOK := effectAlgebra.MountedCallIdentity(mounted)
	ordinal, ordinalOK := effectAlgebra.MountedCallOrdinalForOccurrence(module, occurrence)
	canonicalEffect, canonicalEffectOK := effectAlgebra.MountedCallAt(ordinal)
	callMounted, callMountedOK := callAlgebra.MountedCallForOccurrence(module, occurrence)
	canonicalCall, canonicalCallOK := callAlgebra.MountedCallForApplication(application)
	callApplication, callOccurrence, callModule, _, _, callIdentityOK := callAlgebra.MountedCallIdentity(callMounted)
	key, keyOK := callAlgebra.KeyForApplicationID(application)
	keyApplication, keyApplicationOK := key.ApplicationID()
	root, rootOK := effectAlgebra.RootForMountedCall(mounted)
	rootID, rootIDOK := effectAlgebra.RootID(root)
	if !effectIdentityOK || !ordinalOK || ordinal < 0 || !canonicalEffectOK || canonicalEffect != mounted ||
		!callMountedOK || !canonicalCallOK || canonicalCall != callMounted || !callIdentityOK ||
		callApplication != application || callModule != module || callOccurrence != occurrence || !callAlgebra.OwnsMountedModule(module) ||
		!keyOK || !keyApplicationOK || keyApplication != application || !callAlgebra.OwnsKey(key) ||
		!rootOK || !rootIDOK || !rootID.Available() || !effectAlgebra.Admit(root, effectAlgebra.Bottom()) {
		return calldomain.MountedCall{}, calldomain.Key{}, effectfactor.Root{}, false
	}
	return callMounted, key, root, true
}

func mountedOperandID(domain string, calls *calldomain.Algebra, effects *effectfactor.Algebra, key calldomain.Key, root effectfactor.Root) (identity.ContentID, bool) {
	if domain == "" || calls == nil || effects == nil || !calls.Valid() || !effects.Valid() || !calls.LinkOwner().Matches(effects.LinkOwner()) || !calls.OwnsKey(key) || !effects.Admit(root, effects.Bottom()) {
		return identity.ContentID{}, false
	}
	callID, callOK := key.ContentID()
	rootID, rootOK := effects.RootID(root)
	if !callOK || !rootOK || !callID.Available() || !rootID.Available() {
		return identity.ContentID{}, false
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(domain))
	_, _ = hash.Write(callID[:])
	_, _ = hash.Write(rootID[:])
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id, id.Available()
}

// HotRule is the Selected or Opaque Call-to-Effect Rule binder. Its operand
// is Effect's canonical mounted row. Reduction joins that row to Call's
// TargetRole rows and Effect's formal/publication rows without a second
// mounted-call directory or target map.
type HotRule struct {
	implementation *effectowner.RuleImplementation[effectfactor.MountedCall]
	binding        *engine.SchemaBinding
	fragment       *schemaFragment
	calls          *callowner.HotOwner
	effects        *effectowner.HotOwner
	read           engine.Read[engine.OrderedCells[calldomain.Value]]
	opaque         bool
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

func bindHot(binding *engine.SchemaBinding, fragment *schemaFragment, calls *callowner.HotOwner, effects *effectowner.HotOwner, opaque bool) (*HotRule, bool) {
	if binding == nil || fragment == nil || fragment.slot == nil || calls == nil || !calls.MatchesBinding(binding) || calls.Algebra() == nil || effects == nil || !effects.MatchesBinding(binding) || effects.Algebra() == nil ||
		!calls.Algebra().LinkOwner().Matches(effects.Algebra().LinkOwner()) || !fragment.semantic.Available() {
		return nil, false
	}
	hot := &HotRule{binding: binding, fragment: fragment, calls: calls, effects: effects, opaque: opaque}
	var runtimeRead engine.Read[engine.OrderedCells[calldomain.Value]]
	implementation, read, ok := effectowner.BindExactReadRule(effects, fragment.slot, fragment.callRead, calls.FactorRef(), fragment.write, engine.HotRuleSpec[effectfactor.Value, effectfactor.MountedCall]{
		OperandContent: hot.operandContent,
		Fold: func(frame engine.Frame[effectfactor.Value, effectfactor.MountedCall]) engine.RuleResult[effectfactor.Value] {
			mounted, operandOK := engine.Operand(frame)
			_, key, root, siteOK := mountedCallRows(hot.binding, hot.calls, hot.effects, mounted)
			if !operandOK || !siteOK {
				return engine.RuleResult[effectfactor.Value]{}
			}
			cells, readOK := engine.ReadValue(frame, runtimeRead)
			if !readOK || cells.Count() != 1 {
				return engine.RuleResult[effectfactor.Value]{}
			}
			fact, present, available := cells.At(0)
			if !available {
				return engine.RuleResult[effectfactor.Value]{}
			}
			if !present {
				return engine.NoCandidate(frame)
			}
			result, resultOK := hot.reduce(mounted, key, root, fact)
			if !resultOK {
				return engine.RuleResult[effectfactor.Value]{}
			}
			if hot.effects.Algebra().Equal(result, hot.effects.Algebra().Bottom()) {
				return engine.NoCandidate(frame)
			}
			return engine.Staged(frame, result)
		},
	}, func(mounted effectfactor.MountedCall) (uint64, bool) {
		_, key, _, ok := mountedCallRows(binding, calls, effects, mounted)
		index, indexOK := calls.Algebra().KeyIndex(key)
		return uint64(index), ok && indexOK && index >= 0
	}, func(mounted effectfactor.MountedCall) (uint64, bool) {
		_, _, root, ok := mountedCallRows(binding, calls, effects, mounted)
		index, indexOK := effects.Algebra().RootIndex(root)
		return uint64(index), ok && indexOK && index >= 0
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

func (rule *HotRule) valid() bool {
	return rule != nil && rule.fragment != nil && rule.fragment.slot != nil && rule.implementation != nil &&
		callsiteOwnersValid(rule.binding, rule.calls, rule.effects)
}

func (rule *HotRule) mountedForOccurrence(mount, occurrence identity.ContentID) (effectfactor.MountedCall, bool) {
	if !rule.valid() || !mount.Available() || !occurrence.Available() {
		return effectfactor.MountedCall{}, false
	}
	ordinal, ordinalOK := rule.effects.Algebra().MountedCallOrdinalForOccurrence(mount, occurrence)
	mounted, mountedOK := rule.effects.Algebra().MountedCallAt(ordinal)
	_, _, _, rowsOK := mountedCallRows(rule.binding, rule.calls, rule.effects, mounted)
	application, module, callOccurrence, identityOK := rule.effects.Algebra().MountedCallIdentity(mounted)
	return mounted, ordinalOK && ordinal >= 0 && mountedOK && rowsOK && identityOK && application.Available() && module == mount && callOccurrence == occurrence
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (effectfactor.MountedCall, bool) {
	return rule.mountedForOccurrence(coords.Mount, coords.Occurrence)
}

func (rule *HotRule) operandContent(mounted effectfactor.MountedCall) (effectfactor.MountedCall, [32]byte, bool) {
	if rule == nil {
		return effectfactor.MountedCall{}, [32]byte{}, false
	}
	_, key, root, rowsOK := mountedCallRows(rule.binding, rule.calls, rule.effects, mounted)
	if !rowsOK {
		return effectfactor.MountedCall{}, [32]byte{}, false
	}
	if rule.opaque {
		if _, ok := rule.effects.Algebra().MountedCallOpaqueUnknown(root, rule.calls.Algebra(), mounted, rule.calls.Algebra().Top()); !ok {
			return effectfactor.MountedCall{}, [32]byte{}, false
		}
	}
	id, idOK := mountedOperandID(selectedOperandDomain, rule.calls.Algebra(), rule.effects.Algebra(), key, root)
	if !idOK {
		return effectfactor.MountedCall{}, [32]byte{}, false
	}
	return mounted, [32]byte(id), true
}

func (rule *HotRule) reduce(mounted effectfactor.MountedCall, key calldomain.Key, root effectfactor.Root, value calldomain.Value) (effectfactor.Value, bool) {
	if !rule.valid() || !rule.calls.Algebra().Admits(key, value) {
		return effectfactor.Value{}, false
	}
	_, canonicalKey, canonicalRoot, siteOK := mountedCallRows(rule.binding, rule.calls, rule.effects, mounted)
	if !siteOK || canonicalKey != key || canonicalRoot != root {
		return effectfactor.Value{}, false
	}
	if value.IsTop() {
		return rule.effects.Algebra().Top(), true
	}
	atoms := make([]effectfactor.Atom, 0)
	for index := 0; index < value.KnownTargetCount(); index++ {
		target, targetOK := value.KnownTargetAt(index)
		role, roleOK := target.RoleID()
		canonicalRole, canonicalRoleOK := rule.calls.Algebra().TargetForRole(role)
		canonicalTarget, canonicalTargetOK := canonicalRole.Target()
		if !targetOK || !roleOK || !canonicalRoleOK || !canonicalTargetOK || !canonicalTarget.Same(target) {
			return effectfactor.Value{}, false
		}
		if role.Kind() == calldomain.TargetRoleBody {
			continue
		}
		if role.Kind() != calldomain.TargetRoleSeed {
			return effectfactor.Value{}, false
		}
		operation, applicable := canonicalTarget.Operation()
		if !applicable {
			continue
		}
		if rule.opaque {
			part, partOK := rule.effects.Algebra().SelectedMountedCallOpaque(root, mounted, operation)
			if !partOK {
				return effectfactor.Value{}, false
			}
			unknown, known := rule.effects.Algebra().AtomAt(part, 0)
			if _, extra := rule.effects.Algebra().AtomAt(part, 1); extra {
				return effectfactor.Value{}, false
			}
			if known {
				atoms = append(atoms, unknown)
			}
			continue
		}
		bindings, bindingsOK := rule.effects.Algebra().SelectedCallEffectBindings(root, mounted, operation)
		if !bindingsOK {
			return effectfactor.Value{}, false
		}
		for _, binding := range bindings {
			atom, atomOK := binding.Atom()
			if !atomOK {
				return effectfactor.Value{}, false
			}
			atoms = append(atoms, atom)
		}
	}
	if rule.opaque && value.HasOpaqueAlternative() {
		alternative, alternativeOK := rule.effects.Algebra().MountedCallOpaqueUnknown(root, rule.calls.Algebra(), mounted, value)
		if !alternativeOK {
			return effectfactor.Value{}, false
		}
		atoms = append(atoms, alternative)
	}
	return rule.effects.Algebra().FromAtoms(atoms)
}

func (rule *HotRule) Implementation() (*effectowner.RuleImplementation[effectfactor.MountedCall], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := effectowner.ResolveRuleImplementationFor(rule.effects, rule.implementation)
	return rule.implementation, ok
}

// SealProgramRule is this typed rule's schema registration. It is the only
// place the private owner issuer is converted to the engine primitive.
func SealProgramRule(rule *HotRule) (engine.ProgramRule, bool) {
	if rule == nil || !rule.valid() {
		return engine.ProgramRule{}, false
	}
	implementation, ok := effectowner.ResolveRuleImplementationFor(rule.effects, rule.implementation)
	if !ok {
		return engine.ProgramRule{}, false
	}
	return engine.SealProgramRule(implementation)
}
