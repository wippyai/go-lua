package sourcevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
)

// LuaTypeNameValue is the canonical value semantics of Lua's type(value).
// A singleton runtime kind produces its exact literal tag. Ambiguous runtime
// kinds produce string, matching the concrete body checker's signature
// fallback without discarding caller dependence before specialization.
func LuaTypeNameValue(reg *axis.Registry, typeValues *typevalue.Cache, value product.Value) (product.Value, bool) {
	if reg == nil || product.Equal(reg, value, product.Bottom(reg)) {
		return product.Value{}, false
	}
	kinds := product.Get(reg, value, runtimekind.Key)
	if kinds.IsTop() || kinds.IsBottom() {
		if t, ok := typeValues.TypeOf(reg, value); ok {
			kinds, _ = typevalue.RuntimeKindFromType(t)
		}
	}
	tags := kinds.Tags()
	if len(tags) == 1 {
		return typevalue.LiteralString(reg, tags[0].String()), true
	}
	return typevalue.String(reg), true
}
