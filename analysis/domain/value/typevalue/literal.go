package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func IntegerLiteralValue(reg *axis.Registry, value product.Value) (int64, bool) {
	t, ok := TypeOf(reg, value)
	if !ok {
		return 0, false
	}
	lit, ok := t.(*typ.Literal)
	if !ok || lit.Base != kind.Integer {
		return 0, false
	}
	v, ok := lit.Value.(int64)
	return v, ok
}
