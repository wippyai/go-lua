package call

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
)

// One mounted call occurrence determines exactly one Call carrier coordinate,
// exactly one application key, and one fixed set of detached identities. The
// Algebra is the seal that owns the mounted-call declaration, so it issues
// that whole projection once, content-addressed, and every consumer addresses
// it by dense ordinal instead of rebuilding the occurrence -> identity -> key
// -> coordinate walk and hashing a local operand digest on top of it.
type callCoordinateRow struct {
	applicationID identity.ContentID
	callID        identity.ContentID
	moduleID      identity.ContentID
	calleeValueID identity.ContentID
	loaderSeedID  identity.ContentID
	content       identity.ContentID
	// key is the dense application-key slot; the Call carrier coordinate is
	// key-1, the same coordinate KeyIndex projects for the same Key.
	key uint32
}

// CallCoordinate is the owner-fenced handle for one row of Call's sealed
// occurrence projection. The dense slot is meaningful only to the issuing
// Algebra, so a row from a separately sealed Link cannot be rebound here.
type CallCoordinate struct {
	owner *Algebra
	slot  uint32
}

const callCoordinateVersion uint64 = 1

// callCoordinateDomain separates this projection's preimage from every other
// Call identity derived under the same Link.
var callCoordinateDomain = [...]byte{'c', 'a', 'l', 'l', '-', 'c', 'o', 'o', 'r', 'd', 'i', 'n', 'a', 't', 'e', '!'}

// sealCallCoordinates issues the projection once, immediately after the
// mounted-call directory is complete. It is a function of the sealed Link
// alone: two Algebras over one program produce identical rows and one
// identical table identity.
func (algebra *Algebra) sealCallCoordinates() bool {
	if algebra == nil || !algebra.Valid() || algebra.callCoordinates != nil {
		return false
	}
	linkID := algebra.linkOwner.ContentID()
	if !linkID.Available() {
		return false
	}
	rows := make([]callCoordinateRow, len(algebra.mountedCalls))
	table := sha256.New()
	var header [16]byte
	binary.BigEndian.PutUint64(header[0:8], uint64(len(algebra.mountedCalls)))
	binary.BigEndian.PutUint64(header[8:16], callCoordinateVersion)
	_, _ = table.Write(linkID[:])
	_, _ = table.Write(callCoordinateDomain[:])
	_, _ = table.Write(header[:])
	for index := range rows {
		mounted := MountedCall{owner: algebra, slot: uint32(index + 1)}
		mountedRow, mountedOK := algebra.mountedCallRow(mounted)
		if !mountedOK {
			return false
		}
		mount, mountOK := algebra.mountRow(mountedRow.mount)
		key, keyOK := algebra.applicationKey(mountedRow)
		applicationID, applicationOK := key.ApplicationID()
		keyID, keyIDOK := key.ContentID()
		if !mountOK || !keyOK || !applicationOK || !keyIDOK {
			return false
		}
		content := callCoordinateContentID(linkID, applicationID, mountedRow.callID, mount.moduleID, mountedRow.calleeValueID, keyID)
		if !content.Available() {
			return false
		}
		rows[index] = callCoordinateRow{
			applicationID: applicationID,
			callID:        mountedRow.callID,
			moduleID:      mount.moduleID,
			calleeValueID: mountedRow.calleeValueID,
			loaderSeedID:  mount.loaderSeedID,
			content:       content,
			key:           mountedRow.applicationKey,
		}
		_, _ = table.Write(content[:])
	}
	var tableID identity.ContentID
	copy(tableID[:], table.Sum(nil))
	if !tableID.Available() {
		return false
	}
	// A program with no mounted call still seals an empty projection: absence
	// of rows is a sealed fact, not an unsealed table.
	if rows == nil {
		rows = []callCoordinateRow{}
	}
	algebra.callCoordinates, algebra.callCoordinateTable = rows, tableID
	return true
}

func callCoordinateContentID(linkID, applicationID, callID, moduleID, calleeValueID, keyID identity.ContentID) identity.ContentID {
	if !linkID.Available() || !applicationID.Available() || !callID.Available() || !moduleID.Available() || !calleeValueID.Available() || !keyID.Available() {
		return identity.ContentID{}
	}
	var image [32*6 + 16]byte
	copy(image[0:32], linkID[:])
	copy(image[32:64], applicationID[:])
	copy(image[64:96], callID[:])
	copy(image[96:128], moduleID[:])
	copy(image[128:160], calleeValueID[:])
	copy(image[160:192], keyID[:])
	copy(image[192:208], callCoordinateDomain[:])
	return sha256.Sum256(image[:])
}

// callCoordinatesSealed is the completeness fence for the projection. It is
// deliberately not folded into Valid: the Algebra is already valid while the
// mounted-call directory it projects is still being built.
func (algebra *Algebra) callCoordinatesSealed() bool {
	return algebra.Valid() && algebra.callCoordinates != nil &&
		len(algebra.callCoordinates) == len(algebra.mountedCalls) && algebra.callCoordinateTable.Available()
}

// CallCoordinateTableID is the content identity of the whole projection. It
// is the identity a Rule Program join pins when it references this table.
func (algebra *Algebra) CallCoordinateTableID() (identity.ContentID, bool) {
	if !algebra.callCoordinatesSealed() {
		return identity.ContentID{}, false
	}
	return algebra.callCoordinateTable, true
}

// CallCoordinateCount is the projection's dense extent, which is the mounted
// call count by construction.
func (algebra *Algebra) CallCoordinateCount() int {
	if !algebra.callCoordinatesSealed() {
		return 0
	}
	return len(algebra.callCoordinates)
}

// CallCoordinateAt addresses the projection by the dense ordinal a Rule
// Program join carries.
func (algebra *Algebra) CallCoordinateAt(ordinal int) (CallCoordinate, bool) {
	if !algebra.callCoordinatesSealed() || ordinal < 0 || ordinal >= len(algebra.callCoordinates) {
		return CallCoordinate{}, false
	}
	row := CallCoordinate{owner: algebra, slot: uint32(ordinal + 1)}
	return row, row.Valid()
}

// CallCoordinateForOccurrence is the sole module-scoped inverse. Call IDs are
// reusable across mounts, so the module is part of the key and of the fence.
func (algebra *Algebra) CallCoordinateForOccurrence(moduleID, callID identity.ContentID) (CallCoordinate, bool) {
	if !algebra.callCoordinatesSealed() {
		return CallCoordinate{}, false
	}
	mounted, mountedOK := algebra.MountedCallForOccurrence(moduleID, callID)
	if !mountedOK {
		return CallCoordinate{}, false
	}
	return algebra.CallCoordinateForMountedCall(mounted)
}

// CallCoordinateForApplication is the application inverse for consumers whose
// operand names the Project application rather than the module occurrence.
func (algebra *Algebra) CallCoordinateForApplication(applicationID identity.ContentID) (CallCoordinate, bool) {
	if !algebra.callCoordinatesSealed() {
		return CallCoordinate{}, false
	}
	mounted, mountedOK := algebra.MountedCallForApplication(applicationID)
	if !mountedOK {
		return CallCoordinate{}, false
	}
	return algebra.CallCoordinateForMountedCall(mounted)
}

// CallCoordinateForMountedCall projects an already authenticated handle.
func (algebra *Algebra) CallCoordinateForMountedCall(mounted MountedCall) (CallCoordinate, bool) {
	ordinal, ordinalOK := algebra.MountedCallOrdinal(mounted)
	if !ordinalOK {
		return CallCoordinate{}, false
	}
	return algebra.CallCoordinateAt(ordinal)
}

func (row CallCoordinate) Valid() bool {
	if row.owner == nil || !row.owner.callCoordinatesSealed() || row.slot == 0 || uint64(row.slot) > uint64(len(row.owner.callCoordinates)) {
		return false
	}
	stored := row.owner.callCoordinates[row.slot-1]
	return stored.applicationID.Available() && stored.callID.Available() && stored.moduleID.Available() &&
		stored.calleeValueID.Available() && stored.loaderSeedID.Available() && stored.content.Available() &&
		stored.key != 0 && uint64(stored.key) <= uint64(len(row.owner.keys))
}

// OwnsCallCoordinate authenticates a row against this exact Algebra.
func (algebra *Algebra) OwnsCallCoordinate(row CallCoordinate) bool {
	return algebra != nil && row.owner == algebra && row.Valid()
}

func (row CallCoordinate) stored() (callCoordinateRow, bool) {
	if !row.Valid() {
		return callCoordinateRow{}, false
	}
	return row.owner.callCoordinates[row.slot-1], true
}

// Ordinal is this row's dense address in the projection.
func (row CallCoordinate) Ordinal() (int, bool) {
	if !row.Valid() {
		return 0, false
	}
	return int(row.slot) - 1, true
}

// CoordinateIndex is the dense Call carrier coordinate of this occurrence: the
// value every consumer previously recovered by resolving the mounted call to
// its application key and asking KeyIndex for the key's dense slot.
func (row CallCoordinate) CoordinateIndex() (uint64, bool) {
	stored, ok := row.stored()
	if !ok {
		return 0, false
	}
	return uint64(stored.key - 1), true
}

// Key is the closed source-sum arm this occurrence dispatches through. It is
// an application arm for every row in the projection.
func (row CallCoordinate) Key() (Key, bool) {
	stored, ok := row.stored()
	if !ok {
		return Key{}, false
	}
	key := Key{owner: row.owner, slot: stored.key}
	return key, key.IsApplication()
}

// ContentID is the owner-issued identity of this occurrence row. It is the
// operand digest a consuming rule uses; consumers do not hash one themselves.
func (row CallCoordinate) ContentID() (identity.ContentID, bool) {
	stored, ok := row.stored()
	if !ok {
		return identity.ContentID{}, false
	}
	return stored.content, true
}

// Identity projects the detached row behind this occurrence. It is the same
// tuple MountedCallIdentity returns, read from the sealed projection.
func (row CallCoordinate) Identity() (applicationID, callID, moduleID, calleeValueID, loaderSeedID identity.ContentID, ok bool) {
	stored, storedOK := row.stored()
	if !storedOK {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	return stored.applicationID, stored.callID, stored.moduleID, stored.calleeValueID, stored.loaderSeedID, true
}

func (row CallCoordinate) ApplicationID() (identity.ContentID, bool) {
	stored, ok := row.stored()
	if !ok {
		return identity.ContentID{}, false
	}
	return stored.applicationID, true
}

func (row CallCoordinate) CallID() (identity.ContentID, bool) {
	stored, ok := row.stored()
	if !ok {
		return identity.ContentID{}, false
	}
	return stored.callID, true
}

func (row CallCoordinate) ModuleID() (identity.ContentID, bool) {
	stored, ok := row.stored()
	if !ok {
		return identity.ContentID{}, false
	}
	return stored.moduleID, true
}

func (row CallCoordinate) CalleeValueID() (identity.ContentID, bool) {
	stored, ok := row.stored()
	if !ok {
		return identity.ContentID{}, false
	}
	return stored.calleeValueID, true
}

func (row CallCoordinate) LoaderSeedID() (identity.ContentID, bool) {
	stored, ok := row.stored()
	if !ok {
		return identity.ContentID{}, false
	}
	return stored.loaderSeedID, true
}

// MountedCall re-derives the mounted handle for callers that still need the
// Call-owned row identity beside the coordinate.
func (row CallCoordinate) MountedCall() (MountedCall, bool) {
	if !row.Valid() {
		return MountedCall{}, false
	}
	return row.owner.MountedCallAtHandle(int(row.slot) - 1)
}
