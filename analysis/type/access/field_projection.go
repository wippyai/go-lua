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
	return distributeUnion(u.Members, depth, func(member typ.Type, depth int) fieldResult {
		return fieldDepth(member, name, depth)
	})
}

func fieldInIntersection(in *typ.Intersection, name string, depth int) fieldResult {
	if in == nil {
		return fieldResult{}
	}
	return distributeIntersection(in.Members, depth, func(member typ.Type, depth int) (typ.Type, bool) {
		return fieldAtDepth(member, name, depth)
	})
}
