package typeaccess

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

type indexMode uint8

const (
	indexStatic indexMode = iota
	indexRuntime
)

// Index resolves a bracket-index projection against a type using static type
// facts only.
func Index(container typ.Type, key typ.Type) (typ.Type, bool) {
	return indexDepth(container, key, 0, indexStatic).materialize()
}

// RuntimeIndex resolves a bracket-index projection with Lua read semantics:
// missing table slots can produce nil, but non-table indexing still fails.
func RuntimeIndex(container typ.Type, key typ.Type) (typ.Type, bool) {
	return indexDepth(container, key, 0, indexRuntime).materialize()
}

func indexDepth(container typ.Type, key typ.Type, depth int, mode indexMode) fieldResult {
	if stopDepth(container, depth) {
		return fieldResult{}
	}
	if top, ok := specialAccessType(container); ok {
		return fieldResult{t: top, ok: true}
	}

	res := indexDepthCore(container, key, depth, mode)
	if res.ok || mode != indexRuntime {
		return res
	}
	if missingFieldReadsNilDepth(container, depth+1) {
		return fieldResult{t: typ.Nil, ok: true}
	}
	return fieldResult{}
}

func indexDepthCore(container typ.Type, key typ.Type, depth int, mode indexMode) fieldResult {
	switch v := unwrap.Annotated(container).(type) {
	case *typ.Record:
		return indexInRecord(v, key, depth+1, mode)
	case *typ.Map:
		return indexInMap(v.Key, v.Value, key, depth+1, mode)
	case *typ.ReadonlyMap:
		return indexInMap(v.Key, v.Value, key, depth+1, mode)
	case *typ.Array:
		return indexInArray(v, key, depth+1, mode)
	case *typ.Tuple:
		return indexInTuple(v, key, depth+1, mode)
	case *typ.Union:
		return indexInUnion(v, key, depth+1, mode)
	case *typ.Intersection:
		return indexInIntersection(v, key, depth+1, mode)
	case *typ.Optional:
		res := indexDepth(v.Inner, key, depth+1, mode)
		if res.ok {
			res.nilable = true
		}
		return res
	case *typ.Alias:
		return indexDepth(v.UnaliasedTarget(), key, depth+1, mode)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		if expanded == nil || expanded == container {
			return fieldResult{}
		}
		return indexDepth(expanded, key, depth+1, mode)
	default:
		return fieldResult{}
	}
}

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

func indexInMap(keyDomain typ.Type, value typ.Type, key typ.Type, depth int, mode indexMode) fieldResult {
	return indexByKeyVariants(key, depth, mode, true, func(key typ.Type) fieldResult {
		if !typetable.MapComponentKeyAdmitsType(keyDomain, key) {
			return fieldResult{}
		}
		if value == nil {
			value = typ.Nil
		}
		return fieldResult{t: value, ok: true, nilable: true}
	})
}

func indexInArray(a *typ.Array, key typ.Type, depth int, mode indexMode) fieldResult {
	if a == nil {
		return fieldResult{}
	}
	return indexByKeyVariants(key, depth, mode, true, func(key typ.Type) fieldResult {
		if !subtype.IsSubtype(key, typ.Integer) {
			return fieldResult{}
		}
		elem := a.Element
		if elem == nil {
			elem = typ.Unknown
		}
		return fieldResult{t: elem, ok: true, nilable: true}
	})
}

func indexInTuple(tup *typ.Tuple, key typ.Type, depth int, mode indexMode) fieldResult {
	if tup == nil {
		return fieldResult{}
	}
	return indexByKeyVariants(key, depth, mode, true, func(key typ.Type) fieldResult {
		index, ok := literalIntKey(key)
		if !ok || index < 1 || index > int64(len(tup.Elements)) {
			return fieldResult{}
		}
		elem := tup.Elements[index-1]
		if elem == nil {
			elem = typ.Unknown
		}
		return fieldResult{t: elem, ok: true}
	})
}

func indexInUnion(u *typ.Union, key typ.Type, depth int, mode indexMode) fieldResult {
	if u == nil || len(u.Members) == 0 {
		return fieldResult{}
	}
	out := make([]typ.Type, 0, len(u.Members))
	nilable := false

	for _, member := range u.Members {
		res := indexDepth(member, key, depth+1, mode)
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

func indexInIntersection(in *typ.Intersection, key typ.Type, depth int, mode indexMode) fieldResult {
	if in == nil {
		return fieldResult{}
	}
	out := make([]typ.Type, 0, len(in.Members))
	for _, member := range in.Members {
		if it, ok := indexAtDepth(member, key, depth+1, mode); ok {
			out = append(out, it)
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

func indexAtDepth(container typ.Type, key typ.Type, depth int, mode indexMode) (typ.Type, bool) {
	return indexDepth(container, key, depth, mode).materialize()
}

func indexByKeyVariants(key typ.Type, depth int, mode indexMode, missingNil bool, project func(typ.Type) fieldResult) fieldResult {
	if stopDepth(key, depth) {
		return fieldResult{}
	}
	switch v := unwrap.Annotated(key).(type) {
	case *typ.Union:
		return indexKeyUnion(v, depth+1, mode, missingNil, project)
	case *typ.Optional:
		res := indexByKeyVariants(v.Inner, depth+1, mode, missingNil, project)
		if res.ok {
			res.nilable = true
		}
		return res
	case *typ.Alias:
		return indexByKeyVariants(v.UnaliasedTarget(), depth+1, mode, missingNil, project)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		if expanded == nil || expanded == key {
			return fieldResult{}
		}
		return indexByKeyVariants(expanded, depth+1, mode, missingNil, project)
	default:
		res := project(key)
		if !res.ok && mode == indexRuntime && missingNil {
			return fieldResult{t: typ.Nil, ok: true}
		}
		return res
	}
}

func indexKeyUnion(u *typ.Union, depth int, mode indexMode, missingNil bool, project func(typ.Type) fieldResult) fieldResult {
	if u == nil || len(u.Members) == 0 {
		return fieldResult{}
	}
	out := make([]typ.Type, 0, len(u.Members))
	nilable := false
	for _, member := range u.Members {
		res := indexByKeyVariants(member, depth+1, mode, missingNil, project)
		if !res.ok {
			if mode == indexRuntime && missingNil {
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

func literalStringKey(t typ.Type) (string, bool) {
	lit, ok := t.(*typ.Literal)
	if !ok || lit.Base != kind.String {
		return "", false
	}
	name, ok := lit.Value.(string)
	return name, ok
}

func literalIntKey(t typ.Type) (int64, bool) {
	lit, ok := t.(*typ.Literal)
	if !ok || lit.Base != kind.Integer {
		return 0, false
	}
	index, ok := lit.Value.(int64)
	return index, ok
}
