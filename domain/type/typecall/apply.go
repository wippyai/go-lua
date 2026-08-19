package typecall

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/type/channelselect"
	"github.com/wippyai/go-lua/domain/type/typ"
)

// ApplyCall specializes a callable against argument types. channel.select
// admits CaseSet rows from the parent-issued site; other callables use
// InstantiateGenericCall and CallableReturn. Facts are empty unless the
// callee is channel.select.
func ApplyCall(callee typ.Type, site identity.ContentID, args []typ.Type) (typ.Type, channelselect.CaseSet, bool) {
	fn, ok := Callable(callee)
	if !ok || fn == nil {
		return nil, channelselect.CaseSet{}, false
	}
	if channelselect.IsSelectFunction(fn) {
		return channelselect.SpecializeSelect(site, args)
	}
	if len(fn.TypeParams) > 0 {
		instantiated, _ := InstantiateGenericCall(fn, args)
		fn = instantiated
	}
	result, ok := CallableReturn(fn)
	if !ok {
		return nil, channelselect.CaseSet{}, false
	}
	return result, channelselect.CaseSet{}, true
}
