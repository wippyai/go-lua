package access

import (
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func (q *query) indexInMap(keyDomain typ.Type, value typ.Type, key typ.Type, depth int, mode indexMode) fieldResult {
	return q.indexByKeyVariants(key, depth, mode, true, fieldResult{}, func(key typ.Type) fieldResult {
		ok := typetable.MapComponentKeyAdmitsType(keyDomain, key)
		if !ok && mode == indexRuntime {
			ok = typetable.MapComponentKeyMayOverlapType(keyDomain, key)
		}
		if !ok {
			return fieldResult{}
		}
		if value == nil {
			value = typ.Nil
		}
		return fieldResult{t: value, ok: true, nilable: mode != indexWrite}
	})
}
