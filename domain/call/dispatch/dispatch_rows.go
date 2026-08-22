package dispatch

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/link"
	calldomain "github.com/wippyai/go-lua/domain/call"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// dispatchRow is Dispatch's bind-time projection for one canonical mounted call.
// It contains only the exact handles used by the hot rule and their already
// issued operand identity; source identities and Pack roots are not mirrored.
type dispatchRow struct {
	key        calldomain.Key
	coordinate valuedomain.Coordinate
	contentID  identity.ContentID
}

const dispatchRowVersion uint64 = 1

func sealDispatchRows(rule *HotRule) ([]dispatchRow, bool) {
	if rule == nil || rule.calls == nil || rule.values == nil || rule.packs == nil {
		return nil, false
	}
	algebra := rule.calls.Algebra()
	values := rule.values.Schema()
	if algebra == nil || values == nil {
		return nil, false
	}
	rows := make([]dispatchRow, algebra.MountedCallCount())
	for index := range rows {
		mounted, mountedOK := algebra.MountedCallAtHandle(index)
		applicationID, callID, moduleID, valueID, _, identityOK := algebra.MountedCallIdentity(mounted)
		key, keyOK := algebra.KeyForMountedCall(mounted)
		keyApplication, applicationOK := key.ApplicationID()
		keyID, keyIDOK := key.ContentID()
		coordinate, coordinateOK := values.CoordinateForID(valueID)
		root, rootOK := rule.packs.CallRootForMountedSemantic(moduleID, callID)
		rootID, rootIDOK := rule.packs.RootID(root)
		contentID := dispatchRowID(algebra.LinkOwner(), keyID, valueID, rootID)
		if !mountedOK || !identityOK || !keyOK || !applicationOK || keyApplication != applicationID || !keyIDOK || !coordinateOK || !rootOK || !rootIDOK || !contentID.Available() {
			return nil, false
		}
		rows[index] = dispatchRow{key: key, coordinate: coordinate, contentID: contentID}
	}
	return rows, true
}

func dispatchRowID(owner link.OwnerCapability, keyID, valueID, rootID identity.ContentID) identity.ContentID {
	if !owner.Available() || !keyID.Available() || !valueID.Available() || !rootID.Available() {
		return identity.ContentID{}
	}
	linkID := owner.ContentID()
	if !linkID.Available() {
		return identity.ContentID{}
	}
	var image [32*4 + 16]byte
	copy(image[:32], linkID[:])
	copy(image[32:64], keyID[:])
	copy(image[64:96], valueID[:])
	copy(image[96:128], rootID[:])
	binary.BigEndian.PutUint64(image[128:136], 0x63616c6c2d646973) // call-dis
	binary.BigEndian.PutUint64(image[136:144], dispatchRowVersion)
	return sha256.Sum256(image[:])
}
