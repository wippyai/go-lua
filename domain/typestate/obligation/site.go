package obligation

import (
	calldomain "github.com/wippyai/go-lua/domain/call"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// DeriveCallSite is the operation that publishes the Call coordinate one
// mounted call actual is read at.
//
// The actual carries the module and the Call identity of the site it belongs
// to, and Call already numbers its own occurrences, so this resolves an
// existing coordinate rather than minting one. Several actuals of one call
// resolve to the same coordinate, which is why the row hangs off the actual's
// own candidate directory instead of a correspondence: the two directories
// number different subjects.
func (judgment Judgment) DeriveCallSite(candidate valuedomain.MountedCallArgument) (calldomain.CallCoordinate, bool) {
	if !judgment.Valid() || !judgment.values.OwnsMountedCallArgument(candidate) {
		return calldomain.CallCoordinate{}, false
	}
	module, moduleOK := candidate.Module()
	call, callOK := candidate.CallID()
	if !moduleOK || !callOK {
		return calldomain.CallCoordinate{}, false
	}
	return judgment.calls.CallCoordinateForOccurrence(module, call)
}
