package transform

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/internal/recursion"
	"github.com/wippyai/go-lua/analysis/type/kind"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

// Rewrite traverses a type tree and applies fn at each node.
//
// The function fn is called before child traversal. If fn returns
// (replacement, true), the replacement is used and that node's children are not
// visited. If fn returns (_, false), children are recursively rewritten and the
// parent is rebuilt only when a child changes.
//
// Returns the original pointer when nothing changed (structural sharing).
// This is the foundation for type substitution, expansion, and other transforms.
func Rewrite(t typ.Type, fn func(typ.Type) (typ.Type, bool)) typ.Type {
	return rewriteWithDepth(t, fn, typ.DefaultRecursionDepth)
}

func rewriteWithDepth(t typ.Type, fn func(typ.Type) (typ.Type, bool), maxDepth int) typ.Type {
	guard := typ.GuardForDepth(maxDepth)
	if !rewriteCanDescend(t) {
		return rewriteDepth(t, fn, guard, 0, nil)
	}
	memo := getRewriteMemo()
	defer putRewriteMemo(memo)
	return rewriteDepth(t, fn, guard, 0, memo)
}

const rewriteMemoMaxEntries = 4096

var rewriteMemoPool = sync.Pool{
	New: func() any {
		return make(map[rewriteKey]typ.Type, 64)
	},
}

func getRewriteMemo() map[rewriteKey]typ.Type {
	return rewriteMemoPool.Get().(map[rewriteKey]typ.Type)
}

func putRewriteMemo(m map[rewriteKey]typ.Type) {
	if len(m) > rewriteMemoMaxEntries {
		rewriteMemoPool.Put(make(map[rewriteKey]typ.Type, 64))
		return
	}
	clear(m)
	rewriteMemoPool.Put(m)
}

type rewriteKey struct {
	t     typ.Type
	depth int
}

func rewriteDepth(t typ.Type, fn func(typ.Type) (typ.Type, bool), guard recursion.Guard, depth int, memo map[rewriteKey]typ.Type) typ.Type {
	if t == nil {
		return t
	}
	if !rewriteCanDescend(t) {
		if replacement, ok := fn(t); ok {
			return replacement
		}
		return t
	}

	var key rewriteKey
	if memo != nil {
		key = rewriteKey{t: t, depth: depth}
		if cached, ok := memo[key]; ok {
			return cached
		}
	}

	if replacement, ok := fn(t); ok {
		if memo != nil {
			memo[key] = replacement
		}
		return replacement
	}

	next, ok := guard.Enter()
	if !ok {
		return t
	}
	childDepth := depth + 1

	var out typ.Type
	switch tt := typ.UnwrapTransparentWrappers(t).(type) {
	case *typ.Optional:
		if tt.Inner == nil {
			out = t
			break
		}
		inner := rewriteDepth(tt.Inner, fn, next, childDepth, memo)
		if inner == tt.Inner {
			out = t
			break
		}
		out = typeexpr.Optional(inner)
	case *typ.Union:
		members, changed := typ.MapMembers(tt.Members, func(m typ.Type) typ.Type {
			return rewriteDepth(m, fn, next, childDepth, memo)
		})
		if !changed {
			out = t
			break
		}
		out = typeexpr.Union(members...)
	case *typ.Intersection:
		members, changed := typ.MapMembers(tt.Members, func(m typ.Type) typ.Type {
			return rewriteDepth(m, fn, next, childDepth, memo)
		})
		if !changed {
			out = t
			break
		}
		out = typeexpr.Intersection(members...)
	case *typ.Array:
		elem := rewriteDepth(tt.Element, fn, next, childDepth, memo)
		if elem == tt.Element {
			out = t
			break
		}
		out = typ.NewArray(elem)
	case *typ.Map:
		keyType := rewriteDepth(tt.Key, fn, next, childDepth, memo)
		valueType := rewriteDepth(tt.Value, fn, next, childDepth, memo)
		if keyType == tt.Key && valueType == tt.Value {
			out = t
			break
		}
		out = typetable.NewMap(keyType, valueType)
	case *typ.ReadonlyMap:
		keyType := rewriteDepth(tt.Key, fn, next, childDepth, memo)
		valueType := rewriteDepth(tt.Value, fn, next, childDepth, memo)
		if keyType == tt.Key && valueType == tt.Value {
			out = t
			break
		}
		out = typetable.NewReadonlyMap(keyType, valueType)
	case *typ.Tuple:
		elems, changed := typ.MapMembers(tt.Elements, func(e typ.Type) typ.Type {
			return rewriteDepth(e, fn, next, childDepth, memo)
		})
		if !changed {
			out = t
			break
		}
		out = typ.NewTuple(elems...)
	case *typ.Function:
		out = rewriteFunction(tt, t, fn, next, childDepth, memo)
	case *typ.Record:
		out = rewriteRecord(tt, t, fn, next, childDepth, memo)
	case *typ.Meta:
		out = rewriteMeta(tt, t, fn, next, childDepth, memo)
	case *typ.TypeParam:
		out = rewriteTypeParam(tt, t, fn, next, childDepth, memo)
	case *typ.Generic:
		out = rewriteGeneric(tt, t, fn, next, childDepth, memo)
	case *typ.Recursive:
		out = rewriteRecursive(tt, t, fn, next, childDepth, memo)
	case *typ.Alias:
		target := rewriteDepth(tt.Target, fn, next, childDepth, memo)
		if target == tt.Target {
			out = t
			break
		}
		out = typ.NewAlias(tt.Name, target)
	case *typ.Instantiated:
		args, changed := typ.MapMembers(tt.TypeArgs, func(a typ.Type) typ.Type {
			return rewriteDepth(a, fn, next, childDepth, memo)
		})
		if !changed {
			out = t
			break
		}
		out = typ.Instantiate(tt.Generic, args...)
	case *typ.Interface:
		var methods []typ.Method
		for idx, m := range tt.Methods {
			newType := rewriteDepth(m.Type, fn, next, childDepth, memo)
			if newType != m.Type {
				if methods == nil {
					methods = make([]typ.Method, len(tt.Methods))
					copy(methods, tt.Methods)
				}
				if fnType, ok := newType.(*typ.Function); ok {
					methods[idx] = typ.Method{Name: m.Name, Type: fnType}
				} else {
					methods[idx] = m
				}
			} else if methods != nil {
				methods[idx] = m
			}
		}
		if methods == nil {
			out = t
			break
		}
		out = typ.NewInterface(tt.Name, methods)
	default:
		out = t
	}

	if memo != nil {
		memo[key] = out
	}
	return out
}

func rewriteCanDescend(t typ.Type) bool {
	if t == nil {
		return false
	}
	switch t.Kind() {
	case kind.Optional,
		kind.Union,
		kind.Intersection,
		kind.Array,
		kind.Map,
		kind.ReadonlyMap,
		kind.Tuple,
		kind.Function,
		kind.Record,
		kind.Meta,
		kind.TypeParam,
		kind.Generic,
		kind.Recursive,
		kind.Alias,
		kind.Instantiated,
		kind.Interface:
		return true
	default:
		return false
	}
}
