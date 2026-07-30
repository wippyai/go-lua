package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	typeliteral "github.com/wippyai/go-lua/analysis/type/literal"
)

func IntegerLiteralValue(reg *axis.Registry, value product.Value) (int64, bool) {
	t, ok := TypeOf(reg, value)
	if !ok {
		return 0, false
	}
	return typeliteral.IntegerValue(t)
}
