package transform

import (
	"sync"

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
	if !rewriteCanDescend(t) {
		return rewriteDepth(t, fn, nil)
	}
	memo := getRewriteMemo()
	defer putRewriteMemo(memo)
	return rewriteDepth(t, fn, memo)
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
	t typ.Type
}

func rewriteDepth(t typ.Type, fn func(typ.Type) (typ.Type, bool), memo map[rewriteKey]typ.Type) typ.Type {
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
		key = rewriteKey{t: t}
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
	// Publish the current immutable node before descending. Recursive handlers
	// replace this provisional entry with a μ placeholder, while ordinary DAG
	// sharing terminates immediately on a repeated node.
	if memo != nil {
		memo[key] = t
	}

	var out typ.Type
	switch tt := typ.UnwrapTransparentWrappers(t).(type) {
	case *typ.Optional:
		if tt.Inner == nil {
			out = t
			break
		}
		inner := rewriteDepth(tt.Inner, fn, memo)
		if inner == tt.Inner {
			out = t
			break
		}
		out = typeexpr.Optional(inner)
	case *typ.Union:
		members, changed := typ.MapMembers(tt.Members, func(m typ.Type) typ.Type {
			return rewriteDepth(m, fn, memo)
		})
		if !changed {
			out = t
			break
		}
		out = typeexpr.Union(members...)
	case *typ.Intersection:
		members, changed := typ.MapMembers(tt.Members, func(m typ.Type) typ.Type {
			return rewriteDepth(m, fn, memo)
		})
		if !changed {
			out = t
			break
		}
		out = typeexpr.Intersection(members...)
	case *typ.Array:
		elem := rewriteDepth(tt.Element, fn, memo)
		if elem == tt.Element {
			out = t
			break
		}
		out = typ.NewArray(elem)
	case *typ.Map:
		keyType := rewriteDepth(tt.Key, fn, memo)
		valueType := rewriteDepth(tt.Value, fn, memo)
		if keyType == tt.Key && valueType == tt.Value {
			out = t
			break
		}
		out = typetable.NewMap(keyType, valueType)
	case *typ.ReadonlyMap:
		keyType := rewriteDepth(tt.Key, fn, memo)
		valueType := rewriteDepth(tt.Value, fn, memo)
		if keyType == tt.Key && valueType == tt.Value {
			out = t
			break
		}
		out = typetable.NewReadonlyMap(keyType, valueType)
	case *typ.Tuple:
		elems, changed := typ.MapMembers(tt.Elements, func(e typ.Type) typ.Type {
			return rewriteDepth(e, fn, memo)
		})
		if !changed {
			out = t
			break
		}
		out = typ.NewTuple(elems...)
	case *typ.Function:
		out = rewriteFunction(tt, t, fn, memo)
	case *typ.Record:
		out = rewriteRecord(tt, t, fn, memo)
	case *typ.Meta:
		out = rewriteMeta(tt, t, fn, memo)
	case *typ.TypeParam:
		out = rewriteTypeParam(tt, t, fn, memo)
	case *typ.Generic:
		out = rewriteGeneric(tt, t, fn, memo)
	case *typ.Recursive:
		out = rewriteRecursive(tt, t, fn, memo)
	case *typ.Alias:
		target := rewriteDepth(tt.Target, fn, memo)
		if target == tt.Target {
			out = t
			break
		}
		out = typ.NewAlias(tt.Name, target)
	case *typ.Instantiated:
		args, changed := typ.MapMembers(tt.TypeArgs, func(a typ.Type) typ.Type {
			return rewriteDepth(a, fn, memo)
		})
		generic := tt.Generic
		if replacement, ok := fn(generic); ok {
			if resolved, ok := replacement.(*typ.Generic); ok && resolved != nil {
				generic = resolved
				changed = true
			}
		}
		if !changed {
			out = t
			break
		}
		out = typ.Instantiate(generic, args...)
	case *typ.Interface:
		var methods []typ.Method
		for idx, m := range tt.Methods {
			newType := rewriteDepth(m.Type, fn, memo)
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
