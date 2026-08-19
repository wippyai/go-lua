package call

import "github.com/wippyai/go-lua/analysis/identity"

// MountedCall is Call's opaque, owner-issued receipt for one ordinary-call
// placement.  The dense slot is meaningful only to the exact Algebra which
// issued it; callers can transport the receipt but cannot manufacture or
// reinterpret its row.
type MountedCall struct {
	owner *Algebra
	slot  uint32
}

// Valid reports whether this receipt still names one row in its issuing
// Algebra.  The owner pointer is part of the authority fence, so an equal
// row from a separately sealed Link cannot be rebound here.
func (mounted MountedCall) Valid() bool {
	return mounted.owner != nil && mounted.owner.Valid() && mounted.slot != 0 && uint64(mounted.slot) <= uint64(len(mounted.owner.mountedCalls))
}

// MountedCallAtHandle returns one owner-fenced dense mounted-call receipt in
// Call's canonical application/call-occurrence order.
func (algebra *Algebra) MountedCallAtHandle(index int) (MountedCall, bool) {
	if !algebra.Valid() || index < 0 || index >= len(algebra.mountedCalls) {
		return MountedCall{}, false
	}
	mounted := MountedCall{owner: algebra, slot: uint32(index + 1)}
	return mounted, mounted.Valid()
}

// MountedCallForApplication performs the sole O(1) application inverse for
// mounted-call rows.  The returned receipt remains fenced to this Algebra.
func (algebra *Algebra) MountedCallForApplication(applicationID identity.ContentID) (MountedCall, bool) {
	if !algebra.Valid() || !applicationID.Available() {
		return MountedCall{}, false
	}
	slot := algebra.mountedCallIndex[applicationID]
	mounted := MountedCall{owner: algebra, slot: slot}
	row, ok := algebra.mountedCallRow(mounted)
	return mounted, ok && row.applicationID == applicationID
}

// MountedCallForOccurrence performs the sole O(1) module-scoped artifact-call
// inverse. Call IDs are reusable across mounts, so ModuleKey is an intentional
// part of the lookup key and exactness fence.
func (algebra *Algebra) MountedCallForOccurrence(moduleID, callID identity.ContentID) (MountedCall, bool) {
	if !algebra.Valid() || !moduleID.Available() || !callID.Available() {
		return MountedCall{}, false
	}
	slot := algebra.mountedCallOccurrenceIndex[mountedCallOccurrenceRef{moduleID: moduleID, callID: callID}]
	mounted := MountedCall{owner: algebra, slot: slot}
	row, ok := algebra.mountedCallRow(mounted)
	return mounted, ok && row.moduleID == moduleID && row.callID == callID
}

// MountedCallOrdinalForOccurrence is the dense address of one module-scoped
// artifact call in this Algebra's canonical mounted-call order.  It is the
// direct form of MountedCallForOccurrence for owners that key their own
// sealed row tables by ordinal rather than by receipt.
func (algebra *Algebra) MountedCallOrdinalForOccurrence(moduleID, callID identity.ContentID) (int, bool) {
	mounted, ok := algebra.MountedCallForOccurrence(moduleID, callID)
	if !ok {
		return 0, false
	}
	return int(mounted.slot) - 1, true
}

// MountedCallIdentity projects the canonical detached row behind an exact
// mounted receipt. calleeValueID is the Link-owned Boundary Value identity;
// callID is the reusable artifact call identity qualified by moduleID.
// No Project, Boundary, Shard, or Program authority is retained or reopened.
func (algebra *Algebra) MountedCallIdentity(mounted MountedCall) (applicationID, callID, moduleID, calleeValueID, loaderSeedID identity.ContentID, ok bool) {
	row, ok := algebra.mountedCallRow(mounted)
	return row.applicationID, row.callID, row.moduleID, row.calleeValueID, row.loaderSeedID, ok
}

// OwnsMountedModule authenticates a Link mount identity against this exact
// Call Algebra.  It lets module-scoped issuers reject foreign mounts even
// when a mount contains no ordinary-call occurrence.
func (algebra *Algebra) OwnsMountedModule(moduleID identity.ContentID) bool {
	if !algebra.Valid() || !moduleID.Available() {
		return false
	}
	slot := algebra.mountModuleIndex[moduleID]
	return slot != 0 && uint64(slot) <= uint64(len(algebra.mountModules)) && algebra.mountModules[slot-1] == moduleID
}

func (algebra *Algebra) mountedCallRow(mounted MountedCall) (mountedCallRow, bool) {
	if !algebra.Valid() || mounted.owner != algebra || !mounted.Valid() {
		return mountedCallRow{}, false
	}
	row := algebra.mountedCalls[mounted.slot-1]
	return row, row.applicationID.Available() && row.callID.Available() && row.moduleID.Available() && row.calleeValueID.Available() && row.loaderSeedID.Available()
}
