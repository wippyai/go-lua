package effectlowering

import (
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

// CallableIteratorSignature reports whether the first result of a sealed call
// signature is itself the Lua iterator function. This is the function-valued
// half of generic-for protocol classification; collection iterators remain
// represented by iteration.Iterator effects. Keeping the two forms distinct
// prevents function factories such as string.gmatch from being mis-modelled as
// iteration over one of their arguments.
func CallableIteratorSignature(sig signature.Function) (*typ.Function, bool) {
	if sig.Type == nil || len(sig.Type.Returns) == 0 {
		return nil, false
	}
	iterator, ok := sig.Type.Returns[0].(*typ.Function)
	return iterator, ok && iterator != nil
}
