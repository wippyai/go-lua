package access

import (
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func indexInRecord(r *typ.Record, key typ.Type, depth int, mode indexMode) fieldResult {
	if r == nil {
		return fieldResult{}
	}
	return indexByKeyVariants(key, depth, mode, true, func(key typ.Type) fieldResult {
		if name, ok := literalStringKey(key); ok {
			return stringKeyInRecord(r, name)
		}
		if index, ok := literalIntKey(key); ok {
			return indexIntInRecord(r, index)
		}
		if res := indexDynamicIntMembersInRecord(r, key, depth+1, mode); res.ok {
			if mapRes := indexRecordMapComponent(r, key, depth+1, mode); mapRes.ok {
				return unionFieldResults(res, mapRes)
			}
			return res
		}
		if mapRes := indexRecordMapComponent(r, key, depth+1, mode); mapRes.ok {
			return mapRes
		}
		if r.Open {
			return fieldResult{t: typ.Unknown, ok: true}
		}
		return fieldResult{}
	})
}

func indexDynamicIntMembersInRecord(r *typ.Record, key typ.Type, depth int, mode indexMode) fieldResult {
	if r == nil || len(r.StaticMembers) == 0 {
		return fieldResult{}
	}
	switch mode {
	case indexRuntime:
		if !arrayRuntimeKeyMayBeInteger(key, depth+1) {
			return fieldResult{}
		}
	default:
		if !subtype.IsSubtype(key, typ.Integer) {
			return fieldResult{}
		}
	}
	out := make([]typ.Type, 0, len(r.StaticMembers))
	for _, member := range r.StaticMembers {
		if member.Kind != typ.StaticMemberIntIndex {
			continue
		}
		if member.Type == nil {
			out = append(out, typ.Unknown)
			continue
		}
		out = append(out, member.Type)
	}
	if len(out) == 0 {
		return fieldResult{}
	}
	return fieldResult{t: normalize.UnionForEvidence(out...), ok: true, nilable: true}
}

func indexRecordMapComponent(r *typ.Record, key typ.Type, depth int, mode indexMode) fieldResult {
	if r == nil || !r.HasMapComponent() {
		return fieldResult{}
	}
	ok := typetable.MapComponentKeyAdmitsType(r.MapKey, key)
	if !ok && mode == indexRuntime {
		ok = typetable.MapComponentKeyMayOverlapType(r.MapKey, key)
	}
	if !ok {
		return fieldResult{}
	}
	return fieldResult{t: r.MapValue, ok: true, nilable: mode != indexWrite}
}

func unionFieldResults(a fieldResult, b fieldResult) fieldResult {
	if !a.ok {
		return b
	}
	if !b.ok {
		return a
	}
	out := make([]typ.Type, 0, 2)
	if a.t != nil {
		out = append(out, a.t)
	}
	if b.t != nil {
		out = append(out, b.t)
	}
	if len(out) == 0 {
		return fieldResult{t: typ.Unknown, ok: true, nilable: a.nilable || b.nilable}
	}
	return fieldResult{t: normalize.UnionForEvidence(out...), ok: true, nilable: a.nilable || b.nilable}
}

func stringKeyInRecord(r *typ.Record, name string) fieldResult {
	if r == nil {
		return fieldResult{}
	}
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
