package access

import (
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func indexInMap(keyDomain typ.Type, value typ.Type, key typ.Type, depth int, mode indexMode) fieldResult {
	return indexByKeyVariants(key, depth, mode, true, func(key typ.Type) fieldResult {
		if !typetable.MapComponentKeyAdmitsType(keyDomain, key) {
			return fieldResult{}
		}
		if value == nil {
			value = typ.Nil
		}
		return fieldResult{t: value, ok: true, nilable: mode != indexWrite}
	})
}
