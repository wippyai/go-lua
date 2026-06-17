package exportmanifest

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func unionType(types []typ.Type) (typ.Type, bool) {
	switch len(types) {
	case 0:
		return nil, false
	case 1:
		if types[0] == nil {
			return nil, false
		}
		return types[0], true
	default:
		return normalize.UnionForEvidence(types...), true
	}
}

func valueType(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	return typevalue.TypeOf(reg, value)
}
