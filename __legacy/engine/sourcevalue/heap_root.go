package sourcevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/domain/type/subtype"
)

// PreferExactHeapRoot returns the more precise root carried by the same exact
// table identity. Symbol slots retain the runtime identity while structural
// mutations refine the identity-owned heap root.
func PreferExactHeapRoot(reg *axis.Registry, types *typevalue.Cache, in state.State, current product.Value) product.Value {
	if reg == nil {
		return current
	}
	id, exact := identityvalue.ExactID(reg, current)
	if !exact {
		return current
	}
	root := in.ReadHeapTableObject(reg, id).Root()
	rootID, rootExact := identityvalue.ExactID(reg, root)
	if !rootExact || rootID != id || product.Equal(reg, root, product.Bottom(reg)) {
		return current
	}
	if product.LessOrEq(reg, root, current) {
		return root
	}
	typeOf := typevalue.TypeOf
	if types != nil {
		typeOf = types.TypeOf
	}
	currentType, currentOK := typeOf(reg, current)
	rootType, rootOK := typeOf(reg, root)
	if currentOK && rootOK && subtype.IsSubtype(rootType, currentType) && !subtype.IsSubtype(currentType, rootType) {
		return root
	}
	return current
}
