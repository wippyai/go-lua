package body

import (
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func (r *Result) ObjectLiteralEntryType(value product.Value) (typ.Type, bool) {
	if r == nil {
		return nil, false
	}
	return luasourcevalue.ObjectLiteralEntryType(r.Registry(), r.typeValues, value)
}

func (r *Result) ObjectLiteralEntryHasUntrustedTopOrigin(value product.Value) bool {
	if r == nil {
		return false
	}
	return luasourcevalue.ObjectLiteralEntryHasUntrustedTopOrigin(r.Registry(), value)
}
