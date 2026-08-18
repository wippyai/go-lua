package factor

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/call"
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
	return row, row.applicationID.Available() && row.moduleID.Available() && row.contextID.Available()
}

func (a *Algebra) mountedCallForApplication(applicationID identity.ContentID) (MountedCall, bool) {
	if !a.Valid() || !applicationID.Available() {
		return MountedCall{}, false
	}
	row, ok := a.callRows[applicationID]
	if !ok {
		return MountedCall{}, false
	}
	slot := a.mountedCallIndex[mountedCallRef{moduleID: row.moduleID, contextID: row.context}]
	mounted := MountedCall{owner: a, slot: slot}
	issued, issuedOK := a.mountedCallRow(mounted)
	return mounted, issuedOK && issued.applicationID == applicationID
}

func (a *Algebra) RootForMountedCall(mounted MountedCall) (Root, bool) {
	row, ok := a.mountedCallRow(mounted)
	if !ok {
		return Root{}, false
	}
	root, ok := a.RootForCallID(row.applicationID)
	return root, ok && a.callInRootID(root, row.applicationID)
}

func (a *Algebra) selectedMountedCall(root Root, mounted MountedCall, operation vocabulary.Operation) bool {
	row, ok := a.mountedCallRow(mounted)
	if !ok || !a.callInRootID(root, row.applicationID) {
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
	tail, _, ok := a.contract.EffectTail(operation)
	if !ok || tail == vocabulary.RowVariable {
		return Value{}, false
	}
	known := tail == vocabulary.RowUnknownOpen
	for index := 0; index < a.contract.CallbackCount(operation); index++ {
		callback, ok := a.contract.CallbackAt(operation, index)
		if !ok {
			return Value{}, false
		}
		tail, _, ok := a.contract.CallbackEffectTail(callback)
		if !ok || tail == vocabulary.RowVariable {
			return Value{}, false
		}
		known = known || tail == vocabulary.RowUnknownOpen
	}
	if !known {
		return a.Bottom(), true
	}
	return a.Singleton(Atom{owner: a, root: root.slot, id: a.unknownID})
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
	if !ok || !calls.Admits(key, value) || !a.callInRootID(root, row.applicationID) {
		return Atom{}, false
	}
	return Atom{owner: a, root: root.slot, id: a.unknownID}, true
}

// MountedCallIdentity exposes cold scalar evidence for domain-owned formal
// substitution.  It remains fenced by the opaque mounted receipt.
func (a *Algebra) MountedCallIdentity(mounted MountedCall) (applicationID, moduleID, occurrenceID identity.ContentID, ok bool) {
	row, ok := a.mountedCallRow(mounted)
	return row.applicationID, row.moduleID, row.contextID, ok
}
