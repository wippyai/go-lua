package dispatch

import (
	"github.com/wippyai/go-lua/analysis/identity"
	calldomain "github.com/wippyai/go-lua/domain/call"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// dispatchRow is Dispatch's bind-time projection for one canonical mounted
// call. Call's sealed occurrence projection already carries the coordinate,
// the application key and the row's owner-issued identity, so this table
// holds only the Value coordinate and Pack root join Dispatch adds on top of
// it. No identity is minted here.
type dispatchRow struct {
	key        calldomain.Key
	coordinate valuedomain.Coordinate
	contentID  identity.ContentID
}

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
		projected, projectedOK := algebra.CallCoordinateAt(index)
		_, callID, moduleID, valueID, _, identityOK := projected.Identity()
		key, keyOK := projected.Key()
		contentID, contentOK := projected.ContentID()
		coordinate, coordinateOK := values.CoordinateForID(valueID)
		root, rootOK := rule.packs.CallRootForMountedSemantic(moduleID, callID)
		if !projectedOK || !identityOK || !keyOK || !contentOK || !coordinateOK || !rootOK {
			return nil, false
		}
		if _, rootIDOK := rule.packs.RootID(root); !rootIDOK {
			return nil, false
		}
		rows[index] = dispatchRow{key: key, coordinate: coordinate, contentID: contentID}
	}
	return rows, true
}
