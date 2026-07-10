package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// HasIntegerType reports whether value's current witness is known to be an
// integer subtype. It is a value-domain predicate, not an engine fact.
func HasIntegerType(reg *axis.Registry, value product.Value) bool {
	t, ok := TypeOf(reg, value)
	return ok && subtype.IsSubtype(t, typ.Integer)
}

// HasOnlyNilType reports whether value's current witness can only be nil.
func HasOnlyNilType(reg *axis.Registry, value product.Value) bool {
	t, ok := TypeOf(reg, value)
	return ok && subtype.IsSubtype(t, typ.Nil)
}
