package call

import "github.com/wippyai/go-lua/analysis/identity"

// MountedCall is Call's opaque, owner-issued handle for one ordinary-call
// placement.  The dense slot is meaningful only to the exact Algebra which
// issued it; callers can transport the handle but cannot manufacture or
// reinterpret its row.
type MountedCall struct {
	owner *Algebra
	slot  uint32
}

// Valid reports whether this handle still names one row in its issuing
// Algebra.  The owner pointer is part of the authority fence, so an equal
// row from a separately sealed Link cannot be rebound here.
func (mounted MountedCall) Valid() bool {
	return mounted.owner != nil && mounted.owner.Valid() && mounted.slot != 0 && uint64(mounted.slot) <= uint64(len(mounted.owner.mountedCalls))
}

// MountedCallAtHandle returns one owner-fenced dense mounted-call handle in
// Call's canonical application/call-occurrence order.
func (algebra *Algebra) MountedCallAtHandle(index int) (MountedCall, bool) {
	if !algebra.Valid() || index < 0 || index >= len(algebra.mountedCalls) {
		return MountedCall{}, false
	}
	mounted := MountedCall{owner: algebra, slot: uint32(index + 1)}
	return mounted, mounted.Valid()
}

// MountedCallForApplication performs the sole O(1) application inverse for
// mounted-call rows. The returned handle remains fenced to this Algebra.
func (algebra *Algebra) MountedCallForApplication(applicationID identity.ContentID) (MountedCall, bool) {
	if !algebra.Valid() || !applicationID.Available() {
		return MountedCall{}, false
	}
	slot := algebra.mountedCallIndex[applicationID]
	mounted := MountedCall{owner: algebra, slot: slot}
	row, ok := algebra.mountedCallRow(mounted)
	key, keyOK := algebra.applicationKey(row)
	issued, issuedOK := key.ApplicationID()
	return mounted, ok && keyOK && issuedOK && issued == applicationID
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
	mount, mountOK := algebra.mountRow(row.mount)
	return mounted, ok && mountOK && mount.moduleID == moduleID && row.callID == callID
}

// MountedCallKeyForOccurrence resolves one authenticated mounted occurrence
// and its canonical application key. The mounted row already stores the exact
// private key coordinate, so no detached identity round-trip is required.
func (algebra *Algebra) MountedCallKeyForOccurrence(moduleID, callID identity.ContentID) (MountedCall, Key, bool) {
	if algebra == nil || !algebra.Valid() || !moduleID.Available() || !callID.Available() {
		return MountedCall{}, Key{}, false
	}
	mounted, mountedOK := algebra.MountedCallForOccurrence(moduleID, callID)
	key, keyOK := algebra.KeyForMountedCall(mounted)
	if !mountedOK || !keyOK {
		return MountedCall{}, Key{}, false
	}
	return mounted, key, true
}

// MountedCallOrdinalForOccurrence is the dense address of one module-scoped
// artifact call in this Algebra's canonical mounted-call order.  It is the
// direct form of MountedCallForOccurrence for owners that key their own
// sealed row tables by ordinal rather than by handle.
func (algebra *Algebra) MountedCallOrdinalForOccurrence(moduleID, callID identity.ContentID) (int, bool) {
	mounted, ok := algebra.MountedCallForOccurrence(moduleID, callID)
	if !ok {
		return 0, false
	}
	return algebra.MountedCallOrdinal(mounted)
}

// MountedCallOrdinal authenticates an owner-issued handle and projects its
// private dense coordinate for sealed child tables. No occurrence inverse or
// detached identity replay is involved.
func (algebra *Algebra) MountedCallOrdinal(mounted MountedCall) (int, bool) {
	if _, ok := algebra.mountedCallRow(mounted); !ok {
		return 0, false
	}
	return int(mounted.slot) - 1, true
}

// MountedCallIdentity projects the canonical detached row behind an exact
// mounted handle. calleeValueID is the Link-owned Boundary Value identity;
// callID is the reusable artifact call identity qualified by moduleID.
// No Project, Boundary, Shard, or Program authority is retained or reopened.
func (algebra *Algebra) MountedCallIdentity(mounted MountedCall) (applicationID, callID, moduleID, calleeValueID, loaderSeedID identity.ContentID, ok bool) {
	row, ok := algebra.mountedCallRow(mounted)
	if !ok {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	mount, mountOK := algebra.mountRow(row.mount)
	key, keyOK := algebra.applicationKey(row)
	applicationID, applicationOK := key.ApplicationID()
	return applicationID, row.callID, mount.moduleID, row.calleeValueID, mount.loaderSeedID, mountOK && keyOK && applicationOK
}

// KeyForMountedCall resolves the mounted application arm through the exact
// Algebra that issued the handle. It is the sole bridge from mounted-call
// identity to Call's closed source-sum key; callers cannot synthesize a key
// from detached application bytes.
func (algebra *Algebra) KeyForMountedCall(mounted MountedCall) (Key, bool) {
	row, ok := algebra.mountedCallRow(mounted)
	if !ok {
		return Key{}, false
	}
	return algebra.applicationKey(row)
}

// OwnsMountedModule authenticates a Link mount identity against this exact
// Call Algebra.  It lets module-scoped issuers reject foreign mounts even
// when a mount contains no ordinary-call occurrence.
func (algebra *Algebra) OwnsMountedModule(moduleID identity.ContentID) bool {
	if !algebra.Valid() || !moduleID.Available() {
		return false
	}
	slot := algebra.mountModuleIndex[moduleID]
	row, ok := algebra.mountRow(slot)
	return ok && row.moduleID == moduleID
}

func (algebra *Algebra) mountedCallRow(mounted MountedCall) (mountedCallRow, bool) {
	if !algebra.Valid() || mounted.owner != algebra || !mounted.Valid() {
		return mountedCallRow{}, false
	}
	row := algebra.mountedCalls[mounted.slot-1]
	_, mountOK := algebra.mountRow(row.mount)
	_, keyOK := algebra.applicationKey(row)
	return row, row.callID.Available() && row.calleeValueID.Available() && mountOK && keyOK
}

func (algebra *Algebra) applicationKey(row mountedCallRow) (Key, bool) {
	if !algebra.Valid() || row.applicationKey == 0 || uint64(row.applicationKey) > uint64(len(algebra.keys)) {
		return Key{}, false
	}
	key := Key{owner: algebra, slot: row.applicationKey}
	return key, key.IsApplication()
}

func (algebra *Algebra) mountRow(slot uint32) (mountRow, bool) {
	if !algebra.Valid() || slot == 0 || uint64(slot) > uint64(len(algebra.mounts)) {
		return mountRow{}, false
	}
	row := algebra.mounts[slot-1]
	return row, row.moduleID.Available() && row.programID.Available() && row.loaderSeedID.Available()
}
