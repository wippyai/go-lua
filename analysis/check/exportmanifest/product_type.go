package exportmanifest

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func valueType(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	return typevalue.TypeOf(reg, value)
}
