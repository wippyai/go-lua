package engine

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// nativeExactResultType is the public summary contract's fail-closed shape
// classifier. The summary projection itself is now published by front.
func nativeExactResultType(result typ.Type) bool {
	result = unwrap.Alias(result)
	if result == nil || typ.AbsentOrTopLike(result) || typ.ContainsTypeParam(result) {
		return false
	}
	switch result.Kind() {
	case kind.Nil, kind.Boolean, kind.Number, kind.Integer, kind.String, kind.Literal,
		kind.Record, kind.Array, kind.Map, kind.ReadonlyMap, kind.Tuple, kind.Function:
		return true
	default:
		return false
	}
}
