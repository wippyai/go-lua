package factor

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/effect/internal/valuecore"
)

// Valid reports whether this detached mounted-call receipt was issued by its
// exact Effect algebra.
func (mounted MountedCall) Valid() bool {
	return mounted.owner != nil && mounted.slot != 0 && uint64(mounted.slot) <= uint64(len(mounted.owner.mountedCalls)) && mounted.owner.Valid()
}

func (a *Algebra) MountedCallCount() int {
	if !a.Valid() {
		return 0
	}
	return len(a.mountedCalls)
}

// MountedCallAt returns the one cold-issued detached receipt. It carries only
// the opaque owner/slot; no Project Application, shard, or Program occurrence
// can escape the Factor.
func (a *Algebra) MountedCallAt(index int) (MountedCall, bool) {
	if !a.Valid() || index < 0 || index >= len(a.mountedCalls) {
		return MountedCall{}, false
	}
	return MountedCall{owner: a, slot: uint32(index + 1)}, true
}

func (a *Algebra) mountedCallRow(mounted MountedCall) (mountedCallRow, bool) {
	if !a.Valid() || !mounted.Valid() || mounted.owner != a {
		return mountedCallRow{}, false
	}
	row := a.mountedCalls[mounted.slot-1]
	return row, row.applicationID.Available() && row.moduleID.Available() && row.contextID.Available() && row.root != 0 && uint64(row.root) <= uint64(len(a.roots))
}

// MountedCallOrdinalForOccurrence is the sole exported dense inverse from a
// mounted Program occurrence to this algebra's canonical mounted-call order.
// Owners key their sealed row tables by that ordinal instead of rebuilding a
// module-scoped occurrence directory of their own.
func (a *Algebra) MountedCallOrdinalForOccurrence(moduleID, occurrenceID identity.ContentID) (int, bool) {
	if !a.Valid() || !moduleID.Available() || !occurrenceID.Available() {
		return 0, false
	}
	slot := a.mountedCallIndex[mountedCallRef{moduleID: moduleID, contextID: occurrenceID}]
	mounted := MountedCall{owner: a, slot: slot}
	row, ok := a.mountedCallRow(mounted)
	if !ok || row.moduleID != moduleID || row.contextID != occurrenceID {
		return 0, false
	}
	return int(slot) - 1, true
}

func (a *Algebra) RootForMountedCall(mounted MountedCall) (Root, bool) {
	row, ok := a.mountedCallRow(mounted)
	if !ok {
		return Root{}, false
	}
	return Root{owner: a, slot: row.root}, true
}

func (a *Algebra) selectedMountedCall(root Root, mounted MountedCall, operation vocabulary.Operation) bool {
	row, ok := a.mountedCallRow(mounted)
	if !ok || !a.ownsRoot(root) || row.root != root.slot {
		return false
	}
	if _, ok := a.applicationOperation(row.applicationID, operation); !ok {
		return false
	}
	packRoot, ok := a.packs.CallRootForMountedSemantic(row.moduleID, row.contextID)
	if !ok {
		return false
	}
	_, ok = a.packs.RootID(packRoot)
	return ok
}

func (a *Algebra) SelectedMountedCallOpaque(root Root, mounted MountedCall, operation vocabulary.Operation) (Value, bool) {
	if !a.selectedMountedCall(root, mounted, operation) {
		return Value{}, false
	}
	tail, _, ok := a.contract.Operations.EffectTail(operation)
	if !ok || tail == vocabulary.RowVariable {
		return Value{}, false
	}
	known := tail == vocabulary.RowUnknownOpen
	for index := 0; index < a.contract.Operations.CallbackCount(operation); index++ {
		callback, ok := a.contract.Operations.CallbackAt(operation, index)
		if !ok {
			return Value{}, false
		}
		tail, _, ok := a.contract.Operations.CallbackEffectTail(callback)
		if !ok || tail == vocabulary.RowVariable {
			return Value{}, false
		}
		known = known || tail == vocabulary.RowUnknownOpen
	}
	if !known {
		return a.Bottom(), true
	}
	return a.Singleton(valuecore.NewAtom(a, root.slot, a.unknownID))
}

func (a *Algebra) MountedCallOpaqueUnknown(root Root, calls *call.Algebra, mounted MountedCall, value call.Value) (Atom, bool) {
	if !a.ownsRoot(root) || calls == nil || !calls.Valid() || !calls.LinkOwner().Matches(a.linkOwner) || !value.HasOpaqueAlternative() {
		return Atom{}, false
	}
	row, ok := a.mountedCallRow(mounted)
	if !ok {
		return Atom{}, false
	}
	key, ok := calls.KeyForApplicationID(row.applicationID)
	if !ok || !calls.Admits(key, value) || !a.ownsRoot(root) || row.root != root.slot {
		return Atom{}, false
	}
	return valuecore.NewAtom(a, root.slot, a.unknownID), true
}

// MountedCallIdentity exposes cold scalar evidence for domain-owned formal
// substitution.  It remains fenced by the opaque mounted receipt.
func (a *Algebra) MountedCallIdentity(mounted MountedCall) (applicationID, moduleID, occurrenceID identity.ContentID, ok bool) {
	row, ok := a.mountedCallRow(mounted)
	return row.applicationID, row.moduleID, row.contextID, ok
}

// MountedCallForOccurrence is the axis member candidate resolver: the sole
// path from a mounted Program occurrence to Effect's canonical mounted-call
// receipt. It composes the dense inverse and the cold accessor rather than
// adding a second occurrence directory.
func (a *Algebra) MountedCallForOccurrence(moduleID, occurrenceID identity.ContentID) (MountedCall, bool) {
	ordinal, ok := a.MountedCallOrdinalForOccurrence(moduleID, occurrenceID)
	if !ok {
		return MountedCall{}, false
	}
	return a.MountedCallAt(ordinal)
}

// MountedCallOrdinal is the candidate directory's own inverse: the dense
// position MountedCallAt would return this exact receipt from. It is the
// owner-receiver form the axis member candidate directory addresses, in the
// uint32 width the generated relation owner returns it in.
func (a *Algebra) MountedCallOrdinal(mounted MountedCall) (uint32, bool) {
	if !a.Valid() || !mounted.Valid() || mounted.owner != a {
		return 0, false
	}
	return mounted.slot - 1, true
}

// Root is the axis member destination projection: the body coordinate this
// mounted call's Effect contribution is transported through. It is declared
// on the candidate value itself, exactly as CallCoordinate carries its own
// Key, so the generated projection needs no algebra-mediated indirection.
func (mounted MountedCall) Root() (Root, bool) {
	if mounted.owner == nil {
		return Root{}, false
	}
	return mounted.owner.RootForMountedCall(mounted)
}
