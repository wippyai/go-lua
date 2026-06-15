package access

import (
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func indexInRecord(r *typ.Record, key typ.Type, depth int, mode indexMode) fieldResult {
	if r == nil {
		return fieldResult{}
	}
	return indexByKeyVariants(key, depth, mode, true, func(key typ.Type) fieldResult {
		if name, ok := literalStringKey(key); ok {
			return indexStringInRecord(r, name)
		}
		if index, ok := literalIntKey(key); ok {
			return indexIntInRecord(r, index)
		}
		if r.HasMapComponent() && typetable.MapComponentKeyAdmitsType(r.MapKey, key) {
			return fieldResult{t: r.MapValue, ok: true, nilable: true}
		}
		if r.Open {
			return fieldResult{t: typ.Unknown, ok: true}
		}
		return fieldResult{}
	})
}

func indexStringInRecord(r *typ.Record, name string) fieldResult {
	if f := r.GetField(name); f != nil {
		return fieldResult{t: f.Type, ok: true, nilable: f.Optional}
	}
	if member := r.GetStaticStringIndex(name); member != nil {
		return fieldResult{t: member.Type, ok: true, nilable: member.Optional}
	}
	if r.HasMapComponent() && typetable.MapComponentKeyMayContainString(r.MapKey, name) {
		return fieldResult{t: r.MapValue, ok: true, nilable: true}
	}
	if r.Open {
		return fieldResult{t: typ.Unknown, ok: true}
	}
	return fieldResult{}
}

func indexIntInRecord(r *typ.Record, index int64) fieldResult {
	if member := r.GetStaticIntIndex(index); member != nil {
		return fieldResult{t: member.Type, ok: true, nilable: member.Optional}
	}
	if r.HasMapComponent() && typetable.MapComponentKeyMayContainInt(r.MapKey, index) {
		return fieldResult{t: r.MapValue, ok: true, nilable: true}
	}
	if r.Open {
		return fieldResult{t: typ.Unknown, ok: true}
	}
	return fieldResult{}
}
