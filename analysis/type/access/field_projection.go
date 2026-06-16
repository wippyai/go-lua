package access

import (
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subst"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type fieldResult struct {
	t       typ.Type
	ok      bool
	nilable bool
}

func (r fieldResult) materialize() (typ.Type, bool) {
	if !r.ok {
		return nil, false
	}
	if r.t == nil {
		r.t = typ.Unknown
	}
	if r.nilable {
		return normalize.Optional(r.t), true
	}
	return r.t, true
}

func fieldInInterface(i *typ.Interface, receiver typ.Type, name string) fieldResult {
	if i == nil {
		return fieldResult{}
	}
	for _, method := range i.Methods {
		if method.Name == name {
			return fieldResult{t: subst.Self(method.Type, receiver), ok: true}
		}
	}
	return fieldResult{}
}

func fieldInRecord(r *typ.Record, name string) fieldResult {
	if r == nil {
		return fieldResult{}
	}
	if f := r.GetField(name); f != nil {
		return fieldResult{t: f.Type, ok: true, nilable: f.Optional}
	}
	if member := r.GetStaticStringIndex(name); member != nil {
		return fieldResult{t: member.Type, ok: true, nilable: member.Optional}
	}
	if r.HasMapComponent() {
		if typetable.MapComponentKeyMayContainString(r.MapKey, name) {
			return fieldResult{t: r.MapValue, ok: true, nilable: true}
		}
	}
	if r.Open {
		return fieldResult{t: typ.Unknown, ok: true}
	}
	return fieldResult{}
}

func fieldInMap(key typ.Type, value typ.Type, name string) fieldResult {
	if !typetable.MapComponentKeyMayContainString(key, name) {
		return fieldResult{}
	}
	if value == nil {
		return fieldResult{t: typ.Nil, ok: true}
	}
	return fieldResult{t: value, ok: true, nilable: true}
}

func fieldInUnion(u *typ.Union, name string, depth int) fieldResult {
	if u == nil || len(u.Members) == 0 {
		return fieldResult{}
	}
	out := make([]typ.Type, 0, len(u.Members))
	nilable := false

	for _, member := range u.Members {
		res := fieldDepth(member, name, depth+1)
		if !res.ok {
			if missingFieldReadsNilDepth(member, depth+1) {
				nilable = true
				continue
			}
			return fieldResult{}
		}
		if res.nilable {
			nilable = true
		}
		if res.t != nil {
			out = append(out, res.t)
		}
	}

	if len(out) == 0 {
		if nilable {
			return fieldResult{t: typ.Nil, ok: true}
		}
		return fieldResult{}
	}
	return fieldResult{t: normalize.UnionForEvidence(out...), ok: true, nilable: nilable}
}

func fieldInIntersection(in *typ.Intersection, name string, depth int) fieldResult {
	if in == nil {
		return fieldResult{}
	}
	out := make([]typ.Type, 0, len(in.Members))
	for _, member := range in.Members {
		if ft, ok := fieldAtDepth(member, name, depth+1); ok {
			out = append(out, ft)
		}
	}
	if len(out) == 0 {
		return fieldResult{}
	}
	if len(out) == 1 {
		return fieldResult{t: out[0], ok: true}
	}
	return fieldResult{t: normalize.IntersectionForMeet(out...), ok: true}
}
