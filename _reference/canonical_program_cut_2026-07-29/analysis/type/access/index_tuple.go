package access

import (
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func (q *query) indexInTuple(tup *typ.Tuple, key typ.Type, depth int, mode indexMode) fieldResult {
	if tup == nil {
		return fieldResult{}
	}
	return q.indexByKeyVariants(key, depth, mode, true, fieldResult{}, func(key typ.Type) fieldResult {
		index, ok := literalIntKey(key)
		if !ok {
			switch mode {
			case indexRuntime:
				if !q.arrayRuntimeKeyMayBeInteger(key, depth+1) {
					return fieldResult{}
				}
			default:
				if !subtype.IsSubtype(key, typ.Integer) {
					return fieldResult{}
				}
			}
			out := make([]typ.Type, 0, len(tup.Elements))
			for _, elem := range tup.Elements {
				if elem == nil {
					elem = typ.Unknown
				}
				out = append(out, elem)
			}
			if len(out) == 0 {
				return fieldResult{}
			}
			return fieldResult{t: normalize.UnionForEvidence(out...), ok: true, nilable: mode != indexWrite}
		}
		if index < 1 || index > int64(len(tup.Elements)) {
			return fieldResult{}
		}
		elem := tup.Elements[index-1]
		if elem == nil {
			elem = typ.Unknown
		}
		return fieldResult{t: elem, ok: true}
	})
}
