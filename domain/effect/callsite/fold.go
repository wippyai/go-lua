package callsite

import (
	"github.com/wippyai/go-lua/analysis/engine/operand"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	calldomain "github.com/wippyai/go-lua/domain/call"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
)

// Judgment is the sealed semantic state of the two exact Call-to-Effect rules:
// the two cold algebras their answer rests on, and which of the two readings
// of a call target this state was sealed for.
//
// It is the family's state, not a rule payload. Both algebras are cold and
// immutable for the life of the binding they were issued by, so the state is
// sealed once when the family is installed and every invocation reads it.
// Neither algebra is ever a parameter of the fold: the fold takes the mounted
// call it is indexed by and the one Call fact it read, and nothing else.
type Judgment struct {
	effects *effectfactor.Algebra
	calls   *calldomain.Algebra
	// opaque selects the reading. A selected site resolves each seed target to
	// the exact effect bindings its operation declares; an opaque site resolves
	// the same operation to the one unknown part the Effect algebra publishes
	// for it, and admits the call value's opaque alternative beside them.
	opaque bool
}

// DeriveSelected seals the exact reading. The two algebras must belong to one
// Link: a mounted call joins a Call row to an Effect root, and two owners of
// different Links have no such row in common.
func DeriveSelected(effects *effectfactor.Algebra, calls *calldomain.Algebra) (Judgment, bool) {
	return derive(effects, calls, false)
}

// DeriveBody seals the interprocedural reading against the same two algebras.
// It is a third reading of one call site rather than a second state type: what
// differs is which effects the site publishes, not which world answers them.
func DeriveBody(effects *effectfactor.Algebra, calls *calldomain.Algebra) (Judgment, bool) {
	return derive(effects, calls, false)
}

// DeriveOpaque seals the opaque reading against the same two algebras.
func DeriveOpaque(effects *effectfactor.Algebra, calls *calldomain.Algebra) (Judgment, bool) {
	return derive(effects, calls, true)
}

func derive(effects *effectfactor.Algebra, calls *calldomain.Algebra, opaque bool) (Judgment, bool) {
	if effects == nil || !effects.Valid() || calls == nil || !calls.Valid() || !calls.LinkOwner().Matches(effects.LinkOwner()) {
		return Judgment{}, false
	}
	return Judgment{effects: effects, calls: calls, opaque: opaque}, true
}

// Valid reports whether this state was sealed by DeriveSelected or DeriveOpaque.
func (judgment Judgment) Valid() bool {
	return judgment.effects != nil && judgment.effects.Valid() &&
		judgment.calls != nil && judgment.calls.Valid() &&
		judgment.calls.LinkOwner().Matches(judgment.effects.LinkOwner())
}

// site resolves the two coordinates one mounted call row joins: the Call key
// its fact is read at, and the Effect root its answer is published under. The
// mounted receipt is the Effect algebra's own, and the Call row is resolved
// through the occurrence both directories are addressed by, so nothing here
// compares two dense ordinals or carries a directory of its own.
func (judgment Judgment) site(mounted effectfactor.MountedCall) (calldomain.Key, effectfactor.Root, bool) {
	if !judgment.Valid() {
		return calldomain.Key{}, effectfactor.Root{}, false
	}
	application, module, occurrence, identityOK := judgment.effects.MountedCallIdentity(mounted)
	_, key, callOK := judgment.calls.MountedCallKeyForOccurrence(module, occurrence)
	keyApplication, keyApplicationOK := key.ApplicationID()
	root, rootOK := judgment.effects.RootForMountedCall(mounted)
	rootID, rootIDOK := judgment.effects.RootID(root)
	if !identityOK || !callOK || !keyApplicationOK || keyApplication != application ||
		!rootOK || !rootIDOK || !rootID.Available() || !judgment.effects.Admit(root, judgment.effects.Bottom()) {
		return calldomain.Key{}, effectfactor.Root{}, false
	}
	return key, root, true
}

// Effect is the one irreducible judgment of the two exact call-site rules: the
// Effect fact one mounted call site publishes, given the Call targets that
// site dispatches to.
//
// A Top call value carries every target there is, so the site's effect is Top.
// Otherwise each declared target is authenticated against the Call algebra's
// canonical row for its role, body roles are left to the interprocedural rule
// that owns them, and every seed target contributes the atoms its operation
// declares. An answer that reduces to Bottom is no candidate rather than a
// published empty fact.
func (judgment Judgment) Effect(mounted effectfactor.MountedCall, value calldomain.Value) (effectfactor.Value, structure.ReductionOutcome) {
	key, root, siteOK := judgment.site(mounted)
	if !siteOK || !judgment.calls.Admits(key, value) {
		return effectfactor.Value{}, structure.Refuse
	}
	if value.IsTop() {
		return judgment.effects.Top(), structure.Concrete
	}
	atoms, atomsOK := judgment.atoms(mounted, root, value)
	if !atomsOK {
		return effectfactor.Value{}, structure.Refuse
	}
	result, resultOK := judgment.effects.FromAtoms(atoms)
	if !resultOK {
		return effectfactor.Value{}, structure.Refuse
	}
	if judgment.effects.Equal(result, judgment.effects.Bottom()) {
		return effectfactor.Value{}, structure.NoCandidate
	}
	return result, structure.Concrete
}

// atoms collects the effect parts every declared target of this call value
// contributes at this site.
func (judgment Judgment) atoms(mounted effectfactor.MountedCall, root effectfactor.Root, value calldomain.Value) ([]effectfactor.Atom, bool) {
	atoms := make([]effectfactor.Atom, 0)
	for index := 0; index < value.KnownTargetCount(); index++ {
		target, targetOK := value.KnownTargetAt(index)
		role, roleOK := target.RoleID()
		canonicalTarget, canonicalTargetOK := judgment.calls.TargetForRole(role)
		if !targetOK || !roleOK || !canonicalTargetOK || !canonicalTarget.Same(target) {
			return nil, false
		}
		if role.Kind() == calldomain.TargetRoleBody {
			continue
		}
		if role.Kind() != calldomain.TargetRoleSeed {
			return nil, false
		}
		operation, applicable := canonicalTarget.Operation()
		if !applicable {
			continue
		}
		part, partOK := judgment.operationAtoms(mounted, root, operation)
		if !partOK {
			return nil, false
		}
		atoms = append(atoms, part...)
	}
	if !judgment.opaque || !value.HasOpaqueAlternative() {
		return atoms, true
	}
	alternative, alternativeOK := judgment.effects.MountedCallOpaqueUnknown(root, judgment.calls, mounted, value)
	if !alternativeOK {
		return nil, false
	}
	return append(atoms, alternative), true
}

// operationAtoms answers the effect parts one seed operation contributes under
// the reading this state was sealed for. The opaque reading publishes exactly
// one unknown part per operation; more than one would be an opaque site
// claiming structure it does not have.
func (judgment Judgment) operationAtoms(mounted effectfactor.MountedCall, root effectfactor.Root, operation vocabulary.Operation) ([]effectfactor.Atom, bool) {
	if judgment.opaque {
		part, partOK := judgment.effects.SelectedMountedCallOpaque(root, mounted, operation)
		if !partOK {
			return nil, false
		}
		unknown, known := judgment.effects.AtomAt(part, 0)
		if _, extra := judgment.effects.AtomAt(part, 1); extra {
			return nil, false
		}
		if !known {
			return nil, true
		}
		return []effectfactor.Atom{unknown}, true
	}
	bindings, bindingsOK := judgment.effects.SelectedCallEffectBindings(root, mounted, operation)
	if !bindingsOK {
		return nil, false
	}
	atoms := make([]effectfactor.Atom, 0, len(bindings))
	for _, binding := range bindings {
		atom, atomOK := binding.Atom()
		if !atomOK {
			return nil, false
		}
		atoms = append(atoms, atom)
	}
	return atoms, true
}

// BodyEffect is the interprocedural judgment: the Effect fact one mounted call
// site publishes, given the effect of every executable body it dispatches to.
//
// The members were named by the declared route relation and observed at their
// own roots, so this fold reads what it was handed and nothing else - it does
// not re-derive which bodies the call reaches. Each present part is
// transported to THIS site's root, because a callee's effect is stated under
// the callee's root and the site publishes under its own. A part that is
// already Top makes the whole site Top, and an answer that reduces to Bottom
// is no candidate rather than a published empty fact.
func (judgment Judgment) BodyEffect(mounted effectfactor.MountedCall, cells []operand.SelectedCell[effectfactor.Value]) (effectfactor.Value, structure.ReductionOutcome) {
	_, root, siteOK := judgment.site(mounted)
	if !siteOK {
		return effectfactor.Value{}, structure.Refuse
	}
	atoms := make([]effectfactor.Atom, 0)
	for _, cell := range cells {
		if !cell.Present {
			continue
		}
		if judgment.effects.Equal(cell.Value, judgment.effects.Top()) {
			return judgment.effects.Top(), structure.Concrete
		}
		transported, transportOK := judgment.effects.Transport(cell.Value, root)
		if !transportOK {
			return effectfactor.Value{}, structure.Refuse
		}
		for index := 0; ; index++ {
			atom, exists := judgment.effects.AtomAt(transported, index)
			if !exists {
				break
			}
			atoms = append(atoms, atom)
		}
	}
	result, resultOK := judgment.effects.FromAtoms(atoms)
	if !resultOK {
		return effectfactor.Value{}, structure.Refuse
	}
	if judgment.effects.Equal(result, judgment.effects.Bottom()) {
		return effectfactor.Value{}, structure.NoCandidate
	}
	return result, structure.Concrete
}
