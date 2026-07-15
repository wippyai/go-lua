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
	return stringKeyInRecord(r, name)
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

func (q *query) fieldInUnion(u *typ.Union, name string, depth int) fieldResult {
	if u == nil || len(u.Members) == 0 {
		return fieldResult{}
	}
	return q.distributeUnion(u.Members, depth, func(member typ.Type, depth int) fieldResult {
		// Union field access is a must query. A recursive backedge is the
		// conjunction identity; it contributes no type by itself, while every
		// concrete arm must still succeed.
		return q.field(member, name, depth, fieldResult{ok: true})
	})
}

func (q *query) fieldInIntersection(in *typ.Intersection, name string, depth int) fieldResult {
	if in == nil {
		return fieldResult{}
	}
	return distributeIntersection(in.Members, depth, func(member typ.Type, depth int) (typ.Type, bool) {
		// Intersection field access is a may query, so a backedge is the
		// disjunction identity (no result).
		return q.field(member, name, depth, fieldResult{}).materialize()
	})
}
