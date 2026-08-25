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

// callsiteOwnersValid is the common owner fence for the Call-to-Effect rules
// still bound through the schema protocol, and for the free publication
// accessors below. A mounted row is useful here only after both Factors belong
// to the exact sealed binding and Link.
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
	callMounted, key, callMountedOK := callAlgebra.MountedCallKeyForOccurrence(module, occurrence)
	keyApplication, keyApplicationOK := key.ApplicationID()
	root, rootOK := effectAlgebra.RootForMountedCall(mounted)
	rootID, rootIDOK := effectAlgebra.RootID(root)
	if !effectIdentityOK || !ordinalOK || ordinal < 0 || !canonicalEffectOK || canonicalEffect != mounted ||
		!callMountedOK || !keyApplicationOK || keyApplication != application ||
		!rootOK || !rootIDOK || !rootID.Available() || !effectAlgebra.Admit(root, effectAlgebra.Bottom()) {
		return calldomain.MountedCall{}, calldomain.Key{}, effectfactor.Root{}, false
	}
	return callMounted, key, root, true
}

// mountedOperandID content-addresses one mounted call site under a rule's own
// operand domain, from the Call key and Effect root the site joins.
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

// mountedForOccurrence is the owner-fenced lookup shared by the bind-time fold
// and the free publication accessors. It takes the two owners directly rather
// than a rule instance so a caller holding an authenticated capability and the
// two sealed owners - not a private rule instance - can resolve the same
// mounted row.
func mountedForOccurrence(binding *engine.SchemaBinding, calls *callowner.HotOwner, effects *effectowner.HotOwner, mount, occurrence identity.ContentID) (effectfactor.MountedCall, bool) {
	if !callsiteOwnersValid(binding, calls, effects) || !mount.Available() || !occurrence.Available() {
		return effectfactor.MountedCall{}, false
	}
	ordinal, ordinalOK := effects.Algebra().MountedCallOrdinalForOccurrence(mount, occurrence)
	mounted, mountedOK := effects.Algebra().MountedCallAt(ordinal)
	_, _, _, rowsOK := mountedCallRows(binding, calls, effects, mounted)
	application, module, callOccurrence, identityOK := effects.Algebra().MountedCallIdentity(mounted)
	return mounted, ordinalOK && ordinal >= 0 && mountedOK && rowsOK && identityOK && application.Available() && module == mount && callOccurrence == occurrence
}
