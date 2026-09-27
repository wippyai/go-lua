package typ

import (
	"sync"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
)

// MarkDeclaredShared marks records reachable from several roots as declared in
// one pass. A source record reachable from more than one root maps to a single
// marked object, so nominal comparisons across the roots keep pointer identity.
// Only an unmarked record is rebuilt; an already-declared record is reused, so
// repeating the call over an unchanged set is stable.
func MarkDeclaredShared(roots ...Type) []Type {
	memo := make(map[Type]Type)
	out := make([]Type, len(roots))
	for i, root := range roots {
		if root == nil {
			continue
		}
		out[i] = markDeclaredDepth(root, memo, 0)
	}
	return out
}

func markedRecord(r *Record, memo map[Type]Type) *Record {
	if marked, ok := memo[r]; ok {
		return marked.(*Record)
	}
	marked := r.WithDeclared(true)
	memo[r] = marked
	return marked
}

func markDeclaredDepth(t Type, memo map[Type]Type, depth int) Type {
	if t == nil || DepthExceeded(depth) {
		return t
	}
	if prior, ok := memo[t]; ok {
		return prior
	}
	// Visit dispatches through transparent wrappers, so handle an annotated
	// node here to keep its runtime annotations around the marked inner type.
	if ann, ok := t.(*Annotated); ok && ann.Inner != nil && ann.Inner != t {
		inner := markDeclaredDepth(ann.Inner, memo, depth+1)
		return replaceOrKeep(t, inner == ann.Inner, func() Type { return NewAnnotated(inner, ann.Annotations) })
	}
	return VisitWithGuard(t, NewGuard(), t, func(next internal.RecursionGuard) Visitor[Type] {
		return Visitor[Type]{
			Optional: func(o *Optional) Type {
				if o.Inner == nil {
					return t
				}
				inner := markDeclaredDepth(o.Inner, memo, depth+1)
				return replaceOrKeep(t, inner == o.Inner, func() Type { return NewOptional(inner) })
			},
			Union: func(u *Union) Type {
				members := markDeclaredMembers(u.Members, memo, depth)
				if members == nil {
					return t
				}
				return NewUnion(members...)
			},
			Intersection: func(i *Intersection) Type {
				members := markDeclaredMembers(i.Members, memo, depth)
				if members == nil {
					return t
				}
				return NewIntersection(members...)
			},
			Array: func(a *Array) Type {
				elem := markDeclaredDepth(a.Element, memo, depth+1)
				return replaceOrKeep(t, elem == a.Element, func() Type { return NewArray(elem) })
			},
			Map: func(m *Map) Type {
				keyType := markDeclaredDepth(m.Key, memo, depth+1)
				valueType := markDeclaredDepth(m.Value, memo, depth+1)
				return replaceOrKeep(t, keyType == m.Key && valueType == m.Value, func() Type {
					return NewMap(keyType, valueType)
				})
			},
			Tuple: func(tup *Tuple) Type {
				elems := make([]Type, len(tup.Elements))
				changed := false
				for i, e := range tup.Elements {
					elems[i] = markDeclaredDepth(e, memo, depth+1)
					changed = changed || elems[i] != e
				}
				return replaceOrKeep(t, !changed, func() Type { return NewTuple(elems...) })
			},
			Function: func(fn *Function) Type {
				return markDeclaredFunction(fn, t, memo, depth)
			},
			Record: func(r *Record) Type {
				marked := markedRecord(r, memo)
				memo[t] = marked
				// Descend into the marked record's children so nested records
				// are declared too, keeping the interned field types.
				fields := make([]Field, len(marked.Fields))
				changed := false
				for i, f := range marked.Fields {
					fields[i] = f
					fields[i].Type = markDeclaredDepth(f.Type, memo, depth+1)
					changed = changed || fields[i].Type != f.Type
				}
				metatable := marked.Metatable
				if metatable != nil {
					next := markDeclaredDepth(metatable, memo, depth+1)
					changed = changed || next != metatable
					metatable = next
				}
				mapKey, mapValue := marked.MapKey, marked.MapValue
				if marked.HasMapComponent() {
					key := markDeclaredDepth(marked.MapKey, memo, depth+1)
					value := markDeclaredDepth(marked.MapValue, memo, depth+1)
					changed = changed || key != mapKey || value != mapValue
					mapKey, mapValue = key, value
				}
				if !changed {
					return marked
				}
				return buildRecordTypeWithFlags(fields, metatable, mapKey, mapValue,
					marked.Open, true, true, marked.MapInferredPresence, marked.MapExplicitNilWrite, marked.Complete)
			},
			Alias: func(a *Alias) Type {
				target := markDeclaredDepth(a.Target, memo, depth+1)
				return replaceOrKeep(t, target == a.Target, func() Type { return NewAlias(a.Name, target) })
			},
			Meta: func(m *Meta) Type {
				of := markDeclaredDepth(m.Of, memo, depth+1)
				return replaceOrKeep(t, of == m.Of, func() Type { return NewMeta(of) })
			},
			Instantiated: func(inst *Instantiated) Type {
				args := make([]Type, len(inst.TypeArgs))
				changed := false
				for i, a := range inst.TypeArgs {
					args[i] = markDeclaredDepth(a, memo, depth+1)
					changed = changed || args[i] != a
				}
				if !changed {
					return t
				}
				return Instantiate(inst.Generic, args...)
			},
			Interface: func(iface *Interface) Type {
				methods := make([]Method, len(iface.Methods))
				changed := false
				for i, m := range iface.Methods {
					methods[i] = m
					if m.Type == nil {
						continue
					}
					next := markDeclaredDepth(m.Type, memo, depth+1)
					if nextFn, ok := next.(*Function); ok && nextFn != m.Type {
						methods[i].Type = nextFn
						changed = true
					}
				}
				if !changed {
					return t
				}
				return NewInterface(iface.Name, methods)
			},
			Recursive: func(rec *Recursive) Type {
				if rec.Body == nil || rec.Body == rec {
					return t
				}
				body := markDeclaredDepth(rec.Body, memo, depth+1)
				if body == rec.Body {
					return t
				}
				return NewRecursiveWithBody(rec.Name, body)
			},
			Generic: func(g *Generic) Type {
				if g.Body == nil {
					return t
				}
				body := markDeclaredDepth(g.Body, memo, depth+1)
				return replaceOrKeep(t, body == g.Body, func() Type {
					return NewGeneric(g.Name, g.TypeParams, body)
				})
			},
			Default: func(Type) Type { return t },
		}
	})
}

// replaceOrKeep returns the original t when nothing changed, else the rebuilt
// value, so pointer identity is preserved for untouched subtrees.
func replaceOrKeep(t Type, unchanged bool, rebuild func() Type) Type {
	if unchanged {
		return t
	}
	return rebuild()
}

func markDeclaredMembers(members []Type, memo map[Type]Type, depth int) []Type {
	out := make([]Type, len(members))
	changed := false
	for i, m := range members {
		out[i] = markDeclaredDepth(m, memo, depth+1)
		changed = changed || out[i] != m
	}
	if !changed {
		return nil
	}
	return out
}

func markDeclaredFunction(fn *Function, orig Type, memo map[Type]Type, depth int) Type {
	params := make([]Param, len(fn.Params))
	changed := false
	for i, p := range fn.Params {
		params[i] = p
		next := markDeclaredDepth(p.Type, memo, depth+1)
		if next != p.Type {
			params[i].Type = next
			changed = true
		}
	}
	returns := make([]Type, len(fn.Returns))
	for i, r := range fn.Returns {
		returns[i] = markDeclaredDepth(r, memo, depth+1)
		changed = changed || returns[i] != r
	}
	var variadic Type
	if fn.Variadic != nil {
		variadic = markDeclaredDepth(fn.Variadic, memo, depth+1)
		changed = changed || variadic != fn.Variadic
	}
	if !changed {
		return orig
	}
	return buildFunctionType(fn.TypeParams, params, variadic, returns, fn.Effects, fn.Spec, fn.Refinement)
}

// Rewrite traverses a type tree and applies fn at each node (bottom-up transformation).
//
// The function fn is called on each type node before recursing into children.
// If fn returns (replacement, true), the replacement is used and children are
// not visited (early termination). If fn returns (_, false), children are
// recursively rewritten first, then the result is reassembled.
//
// Returns the original pointer when nothing changed (structural sharing).
// This is the foundation for type substitution, expansion, and other transforms.
func Rewrite(t Type, fn func(Type) (Type, bool)) Type {
	return rewriteWithDepth(t, fn, DefaultRecursionDepth)
}

func rewriteWithDepth(t Type, fn func(Type) (Type, bool), maxDepth int) Type {
	guard := GuardForDepth(maxDepth)
	if !rewriteCanDescend(t) {
		return rewriteDepth(t, fn, guard, nil)
	}
	memo := getRewriteMemo()
	defer putRewriteMemo(memo)
	return rewriteDepth(t, fn, guard, memo)
}

const rewriteMemoMaxEntries = 4096

var rewriteMemoPool = sync.Pool{
	New: func() any {
		return make(map[rewriteKey]Type, 64)
	},
}

func getRewriteMemo() map[rewriteKey]Type {
	return rewriteMemoPool.Get().(map[rewriteKey]Type)
}

func putRewriteMemo(m map[rewriteKey]Type) {
	if len(m) > rewriteMemoMaxEntries {
		rewriteMemoPool.Put(make(map[rewriteKey]Type, 64))
		return
	}
	clear(m)
	rewriteMemoPool.Put(m)
}

type rewriteKey struct {
	t     Type
	depth int
}

func rewriteDepth(t Type, fn func(Type) (Type, bool), guard internal.RecursionGuard, memo map[rewriteKey]Type) Type {
	if t == nil {
		return t
	}
	if !rewriteCanDescend(t) {
		if replacement, ok := fn(t); ok {
			return replacement
		}
		return t
	}

	depth := guard.Depth()
	var key rewriteKey
	if memo != nil {
		key = rewriteKey{t: t, depth: depth}
		if cached, ok := memo[key]; ok {
			return cached
		}
	}

	next, ok := guard.Enter(t)
	if !ok {
		return t
	}

	if replacement, ok := fn(t); ok {
		if memo != nil {
			memo[key] = replacement
		}
		return replacement
	}

	var out Type
	switch tt := unwrapTransparentWrappers(t).(type) {
	case *Optional:
		if tt.Inner == nil {
			out = t
			break
		}
		inner := rewriteDepth(tt.Inner, fn, next, memo)
		if inner == tt.Inner {
			out = t
			break
		}
		out = NewOptional(inner)
	case *Union:
		var members []Type
		for i, m := range tt.Members {
			newMember := rewriteDepth(m, fn, next, memo)
			if newMember != m {
				if members == nil {
					members = make([]Type, len(tt.Members))
					copy(members, tt.Members)
				}
				members[i] = newMember
			} else if members != nil {
				members[i] = m
			}
		}
		if members == nil {
			out = t
			break
		}
		out = NewUnion(members...)
	case *Intersection:
		var members []Type
		for i, m := range tt.Members {
			newMember := rewriteDepth(m, fn, next, memo)
			if newMember != m {
				if members == nil {
					members = make([]Type, len(tt.Members))
					copy(members, tt.Members)
				}
				members[i] = newMember
			} else if members != nil {
				members[i] = m
			}
		}
		if members == nil {
			out = t
			break
		}
		out = NewIntersection(members...)
	case *Array:
		elem := rewriteDepth(tt.Element, fn, next, memo)
		if elem == tt.Element {
			out = t
			break
		}
		out = NewArray(elem)
	case *Map:
		keyType := rewriteDepth(tt.Key, fn, next, memo)
		valueType := rewriteDepth(tt.Value, fn, next, memo)
		if keyType == tt.Key && valueType == tt.Value {
			out = t
			break
		}
		out = NewMap(keyType, valueType)
	case *Tuple:
		var elems []Type
		for i, e := range tt.Elements {
			newElem := rewriteDepth(e, fn, next, memo)
			if newElem != e {
				if elems == nil {
					elems = make([]Type, len(tt.Elements))
					copy(elems, tt.Elements)
				}
				elems[i] = newElem
			} else if elems != nil {
				elems[i] = e
			}
		}
		if elems == nil {
			out = t
			break
		}
		out = NewTuple(elems...)
	case *Function:
		out = rewriteFunction(tt, t, fn, next, memo)
	case *Record:
		out = rewriteRecord(tt, t, fn, next, memo)
	case *Alias:
		target := rewriteDepth(tt.Target, fn, next, memo)
		if target == tt.Target {
			out = t
			break
		}
		out = NewAlias(tt.Name, target)
	case *Meta:
		of := rewriteDepth(tt.Of, fn, next, memo)
		if of == tt.Of {
			out = t
			break
		}
		out = NewMeta(of)
	case *Instantiated:
		var args []Type
		for idx, a := range tt.TypeArgs {
			newArg := rewriteDepth(a, fn, next, memo)
			if newArg != a {
				if args == nil {
					args = make([]Type, len(tt.TypeArgs))
					copy(args, tt.TypeArgs)
				}
				args[idx] = newArg
			} else if args != nil {
				args[idx] = a
			}
		}
		if args == nil {
			out = t
			break
		}
		out = Instantiate(tt.Generic, args...)
	case *Interface:
		var methods []Method
		for idx, m := range tt.Methods {
			newType := rewriteDepth(m.Type, fn, next, memo)
			if newType != m.Type {
				if methods == nil {
					methods = make([]Method, len(tt.Methods))
					copy(methods, tt.Methods)
				}
				if fnType, ok := newType.(*Function); ok {
					methods[idx] = Method{Name: m.Name, Type: fnType}
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
		out = NewInterface(tt.Name, methods)
	default:
		out = t
	}

	if memo != nil {
		memo[key] = out
	}
	return out
}

func rewriteCanDescend(t Type) bool {
	if t == nil {
		return false
	}
	switch t.Kind() {
	case kind.Optional,
		kind.Union,
		kind.Intersection,
		kind.Array,
		kind.Map,
		kind.Tuple,
		kind.Function,
		kind.Record,
		kind.Alias,
		kind.Meta,
		kind.Instantiated,
		kind.Interface:
		return true
	default:
		return false
	}
}

func rewriteFunction(v *Function, orig Type, fn func(Type) (Type, bool), guard internal.RecursionGuard, memo map[rewriteKey]Type) Type {
	changed := false

	var params []Param
	for i, p := range v.Params {
		newType := rewriteDepth(p.Type, fn, guard, memo)
		if newType != p.Type {
			if params == nil {
				params = make([]Param, len(v.Params))
				copy(params, v.Params)
			}
			changed = true
			params[i] = Param{Name: p.Name, Type: newType, Optional: p.Optional}
		} else if params != nil {
			params[i] = p
		}
	}

	var returns []Type
	for i, r := range v.Returns {
		newRet := rewriteDepth(r, fn, guard, memo)
		if newRet != r {
			if returns == nil {
				returns = make([]Type, len(v.Returns))
				copy(returns, v.Returns)
			}
			changed = true
			returns[i] = newRet
		} else if returns != nil {
			returns[i] = r
		}
	}

	var variadic Type
	if v.Variadic != nil {
		variadic = rewriteDepth(v.Variadic, fn, guard, memo)
		if variadic != v.Variadic {
			changed = true
		}
	}

	if !changed {
		return orig
	}

	paramSrc := v.Params
	if params != nil {
		paramSrc = params
	}
	returnsSrc := v.Returns
	if returns != nil {
		returnsSrc = returns
	}
	return buildFunctionType(
		v.TypeParams,
		paramSrc,
		variadic,
		returnsSrc,
		v.Effects,
		v.Spec,
		v.Refinement,
	)
}

func rewriteRecord(v *Record, orig Type, fn func(Type) (Type, bool), guard internal.RecursionGuard, memo map[rewriteKey]Type) Type {
	changed := false

	var fields []Field
	for i, f := range v.Fields {
		newType := rewriteDepth(f.Type, fn, guard, memo)
		if newType != f.Type {
			if fields == nil {
				fields = make([]Field, len(v.Fields))
				copy(fields, v.Fields)
			}
			changed = true
			fields[i] = f
			fields[i].Type = newType
		} else if fields != nil {
			fields[i] = f
		}
	}

	var metatable Type
	if v.Metatable != nil {
		metatable = rewriteDepth(v.Metatable, fn, guard, memo)
		if metatable != v.Metatable {
			changed = true
		}
	}

	mapKey := v.MapKey
	mapValue := v.MapValue
	if v.HasMapComponent() {
		mapKey = rewriteDepth(v.MapKey, fn, guard, memo)
		if mapKey != v.MapKey {
			changed = true
		}
		mapValue = rewriteDepth(v.MapValue, fn, guard, memo)
		if mapValue != v.MapValue {
			changed = true
		}
	}

	if !changed {
		return orig
	}

	fieldsSrc := v.Fields
	if fields != nil {
		fieldsSrc = fields
	}
	return buildRecordType(fieldsSrc, metatable, mapKey, mapValue, v.Open, v.Complete, true)
}
