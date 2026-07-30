package typeoperator

import (
	"math"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func isNumericStringLiteral(t typ.Type) bool {
	t = operatorSurface(t)
	lit, ok := t.(*typ.Literal)
	if !ok || lit.Base != kind.String {
		return false
	}
	_, ok = numericStringLiteral(lit)
	return ok
}

func numericStringLiteral(lit *typ.Literal) (float64, bool) {
	if lit == nil || lit.Base != kind.String {
		return 0, false
	}
	value, ok := lit.Value.(string)
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, false
	}
	return n, true
}

func isIntegralFloat(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v == math.Trunc(v)
}
