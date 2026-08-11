package access

import (
	"github.com/wippyai/go-lua/analysis/type/normalize"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

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
	if field := r.GetField(name); field != nil {
		return fieldResult{t: field.Type, ok: true, nilable: field.Optional}
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
	if r == nil {
		return fieldResult{}
	}
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
