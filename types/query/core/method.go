package core

import (
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// Method resolves a method type on a container without using an engine.
//
// This is the pure, non-memoized version of method lookup. Methods in Lua
// are accessed via colon syntax (obj:method()) which passes obj as the first
// argument. This function resolves the method's type signature.
//
// Search order:
//  1. Meta type's built-in :is method
//  2. Record fields that are function types
//  3. Interface method signatures
//  4. Metatable fields for records with metatables
//  5. __index chain for inherited methods
//
// For unions, the method must exist with compatible signatures in all non-nil
// members. Returns (nil, false) if the method does not exist.
func Method(t typ.Type, name string) (typ.Type, bool) {
	res, ok := methodDepth(t, name, 0)
	if zzMethodDbg && (name == "type" || name == "all" || name == "contexts") {
		println("ZZMETHOD name=", name, " recv=", zzMethodDump(t), " ok=", ok, " res=", typ.FormatShort(res))
	}
	return res, ok
}

// methodViaIndex walks the __index chain to find inherited methods.
//
// Lua's prototype inheritance uses __index to delegate lookups to parent tables.
// This function follows the chain:
//  1. Get __index field from current metatable
//  2. If __index is a table, look up the method there and recurse
//  3. If __index is a function, use its return type as the method type
func methodViaIndex(meta typ.Type, name string, depth int, owners ...typ.Type) (typ.Type, bool) {
	if stopDepth(meta, depth) {
		return nil, false
	}

	// A sealed class family appears as a recursive metatable (mu X). Unfold it to
	// its body so prototype/__index lookup proceeds on the class record.
	if mu, ok := meta.(*typ.Recursive); ok {
		if mu.Body == nil || mu.Body == mu {
			return nil, false
		}
		return methodViaIndex(mu.Body, name, depth+1, append(owners, mu)...)
	}

	rec, ok := meta.(*typ.Record)
	if !ok {
		return nil, false
	}

	// Get __index from this metatable
	indexField := rec.GetField("__index")
	if indexField == nil {
		return nil, false
	}

	res := typ.Visit(indexField.Type, typ.Visitor[fieldResult]{
		Recursive: func(rec *typ.Recursive) fieldResult {
			if rec == nil || rec.Body == nil || rec.Body == rec {
				return fieldResult{}
			}
			if ft, ok := fieldDepth(rec.Body, name, depth+1); ok {
				return fieldResult{t: normalizeMethodReceiverSelf(ft, append(owners, rec, rec.Body)...), ok: true}
			}
			ft, ok := methodViaIndex(rec.Body, name, depth+1, append(owners, rec)...)
			return fieldResult{t: ft, ok: ok}
		},
		Alias: func(a *typ.Alias) fieldResult {
			ft, ok := methodViaIndex(a.Target, name, depth+1, owners...)
			return fieldResult{t: ft, ok: ok}
		},
		Optional: func(o *typ.Optional) fieldResult {
			ft, ok := methodViaIndex(o.Inner, name, depth+1, owners...)
			return fieldResult{t: ft, ok: ok}
		},
		Record: func(r *typ.Record) fieldResult {
			// __index is a table - look for method there
			if ft, ok := fieldDepth(r, name, depth+1); ok {
				return fieldResult{t: normalizeMethodReceiverSelf(ft, append(owners, r)...), ok: true}
			}
			// Continue walking the chain
			if r.Metatable != nil {
				ft, ok := methodViaIndex(r.Metatable, name, depth+1, append(owners, r)...)
				return fieldResult{t: ft, ok: ok}
			}

			return fieldResult{}
		},
		Function: func(fn *typ.Function) fieldResult {
			// __index is a function - returns any
			if len(fn.Returns) > 0 {
				return fieldResult{t: fn.Returns[0], ok: true}
			}

			return fieldResult{t: typ.Unknown, ok: true}
		},
		Default: func(t typ.Type) fieldResult {
			return fieldResult{}
		},
	})
	return res.t, res.ok
}

// methodDepth recursively resolves method lookup with depth limiting.
// Handles various type constructors and propagates through wrappers.
func methodDepth(t typ.Type, name string, depth int) (typ.Type, bool) {
	return methodDepthWithOwner(t, name, depth, nil)
}

func methodDepthWithOwner(t typ.Type, name string, depth int, owner typ.Type) (typ.Type, bool) {
	if stopDepth(t, depth) {
		return nil, false
	}
	if top, ok := specialAccessType(t); ok {
		return top, true
	}

	res := typ.Visit(t, typ.Visitor[fieldResult]{
		TypeParam: func(tp *typ.TypeParam) fieldResult {
			if tp.Constraint != nil {
				mt, ok := methodDepthWithOwner(tp.Constraint, name, depth+1, owner)
				return fieldResult{t: mt, ok: ok}
			}
			return fieldResult{}
		},
		Meta: func(m *typ.Meta) fieldResult {
			// Meta types have a built-in :is method for type guards
			if name == "is" {
				return fieldResult{t: metaIsMethod(m.Of), ok: true}
			}
			return fieldResult{}
		},
		Record: func(r *typ.Record) fieldResult {
			methodOwner := owner
			if methodOwner == nil {
				methodOwner = r
			}
			// Check direct fields first. Do not use fieldDepth here: field access
			// follows __index and open-record fallbacks, while method lookup must
			// normalize colon-call receiver contracts at the method boundary.
			if ft, ok := directRecordField(r, name); ok {
				if mt, ok := methodFieldView(ft, methodOwner, r); ok {
					return fieldResult{t: mt, ok: true}
				}
				return fieldResult{}
			}

			if r.Metatable == nil || typ.IsMetatableUnconstrained(r.Metatable) {
				if r.Open {
					// Open record: allow unknown methods to be called.
					return fieldResult{t: typ.Func().Variadic(typ.Any).Returns(typ.Any).Build(), ok: true}
				}
				return fieldResult{}
			}
			// Look for method in metatable's fields
			if ft, ok := fieldDepth(r.Metatable, name, depth+1); ok {
				if mt, ok := methodFieldView(ft, methodOwner, r.Metatable); ok {
					return fieldResult{t: mt, ok: true}
				}
			}
			// Check __index chain for inherited methods
			if mt, ok := methodViaIndex(r.Metatable, name, depth+1, methodOwner, r.Metatable); ok {
				return fieldResult{t: mt, ok: true}
			}
			if r.Open {
				// Open record: allow unknown methods even when metatable has no method.
				return fieldResult{t: typ.Func().Variadic(typ.Any).Returns(typ.Any).Build(), ok: true}
			}
			return fieldResult{}
		},
		Interface: func(i *typ.Interface) fieldResult {
			for _, m := range i.Methods {
				if m.Name == name {
					return fieldResult{t: m.Type, ok: true}
				}
			}

			return fieldResult{}
		},
		Function: func(fn *typ.Function) fieldResult {
			return fieldResult{}
		},
		Union: func(u *typ.Union) fieldResult {
			methods := make([]typ.Type, 0, len(u.Members))

			for _, m := range u.Members {
				// Skip nil in union - nil | T should allow method calls on T
				if m.Kind() == kind.Nil {
					continue
				}

				mt, ok := methodDepthWithOwner(m, name, depth+1, nil)
				if !ok {
					return fieldResult{}
				}

				methods = append(methods, mt)
			}
			// If union was empty or only nil members, no method found.
			if len(methods) == 0 {
				return fieldResult{}
			}

			result := typ.NewUnion(methods...)
			return fieldResult{t: result, ok: result != nil}
		},
		Intersection: func(in *typ.Intersection) fieldResult {
			for _, m := range in.Members {
				if mt, ok := methodDepthWithOwner(m, name, depth+1, owner); ok {
					return fieldResult{t: mt, ok: true}
				}
			}

			return fieldResult{}
		},
		Optional: func(o *typ.Optional) fieldResult {
			mt, ok := methodDepthWithOwner(o.Inner, name, depth+1, owner)
			return fieldResult{t: mt, ok: ok}
		},
		Recursive: func(rec *typ.Recursive) fieldResult {
			if rec.Body == nil || rec.Body == rec {
				return fieldResult{}
			}
			methodOwner := owner
			if methodOwner == nil {
				methodOwner = rec
			}
			mt, ok := methodDepthWithOwner(rec.Body, name, depth+1, methodOwner)
			return fieldResult{t: mt, ok: ok}
		},
		Alias: func(a *typ.Alias) fieldResult {
			mt, ok := methodDepthWithOwner(a.Target, name, depth+1, owner)
			return fieldResult{t: mt, ok: ok}
		},
		Instantiated: func(inst *typ.Instantiated) fieldResult {
			resolved, err := ResolveInstantiated(inst)
			if err != nil {
				return fieldResult{}
			}

			mt, ok := methodDepthWithOwner(resolved, name, depth+1, owner)
			return fieldResult{t: mt, ok: ok}
		},
		Ref: func(r *typ.Ref) fieldResult {
			return fieldResult{}
		},
		Default: func(t typ.Type) fieldResult {
			return fieldResult{}
		},
	})
	return res.t, res.ok
}

func directRecordField(r *typ.Record, name string) (typ.Type, bool) {
	if r == nil {
		return nil, false
	}
	f := r.GetField(name)
	if f == nil {
		return nil, false
	}
	if f.Optional {
		return typ.NewOptional(f.Type), true
	}
	return f.Type, true
}

func methodFieldView(t typ.Type, owners ...typ.Type) (typ.Type, bool) {
	if t == nil {
		return nil, false
	}
	if top, ok := specialAccessType(t); ok {
		return top, true
	}
	switch v := unwrap.Alias(t).(type) {
	case *typ.Optional:
		inner, ok := methodFieldView(v.Inner, owners...)
		if !ok {
			return nil, false
		}
		return typ.NewOptional(inner), true
	case *typ.Union:
		// A field whose type is a union of callables (e.g. close provided both as
		// a direct field and via __index, merged across the return summary) is
		// callable as a method. View each member and rejoin; the call pipeline
		// resolves the union callee.
		views := make([]typ.Type, 0, len(v.Members))
		for _, m := range v.Members {
			mv, ok := methodFieldView(m, owners...)
			if !ok {
				return nil, false
			}
			views = append(views, mv)
		}
		return typ.NewUnion(views...), true
	default:
		if unwrap.Function(t) == nil {
			return nil, false
		}
		return normalizeMethodReceiverSelf(t, owners...), true
	}
}

func normalizeMethodReceiverSelf(t typ.Type, owners ...typ.Type) typ.Type {
	fn := unwrap.Function(t)
	if fn == nil || len(fn.Params) == 0 {
		return t
	}
	first := fn.Params[0]
	if first.Name != "self" && first.Name != "Self" && !methodReceiverParamMatchesOwner(first.Type, owners...) {
		return t
	}
	builder := typ.Func().ReserveParams(len(fn.Params))
	for _, tp := range fn.TypeParams {
		builder = builder.TypeParam(tp.Name, tp.Constraint)
	}
	for i, p := range fn.Params {
		paramType := p.Type
		if i == 0 {
			paramType = typ.Self
		}
		if p.Optional {
			builder = builder.OptParam(p.Name, paramType)
		} else {
			builder = builder.Param(p.Name, paramType)
		}
	}
	if fn.Variadic != nil {
		builder = builder.Variadic(fn.Variadic)
	}
	if len(fn.Returns) > 0 {
		builder = builder.Returns(fn.Returns...)
	}
	if fn.Effects != nil {
		builder = builder.Effects(fn.Effects)
	}
	if fn.Spec != nil {
		builder = builder.Spec(fn.Spec)
	}
	if fn.Refinement != nil {
		builder = builder.WithRefinement(fn.Refinement)
	}
	return builder.Build()
}

func methodReceiverParamMatchesOwner(param typ.Type, owners ...typ.Type) bool {
	param = receiverOwnerComparableType(param)
	if param == nil {
		return false
	}
	for _, owner := range owners {
		owner = receiverOwnerComparableType(owner)
		if owner == nil {
			continue
		}
		if typ.SameNodeOrAcyclicEqual(param, owner) || typ.SameProductFamily(param, owner) {
			return true
		}
	}
	return false
}

func receiverOwnerComparableType(t typ.Type) typ.Type {
	t = unwrap.Alias(t)
	if opt, ok := t.(*typ.Optional); ok {
		return receiverOwnerComparableType(opt.Inner)
	}
	return t
}

// HasMethod returns true if a type has a method with the given name.
//
// This is a convenience wrapper around Method that discards the type result.
func HasMethod(t typ.Type, name string) bool {
	_, ok := methodDepth(t, name, 0)
	return ok
}

// FieldOrMethod looks up a name as either field or method.
//
// This combines field and method lookup, preferring fields. It handles the
// ambiguity in Lua where t.name could access a field or a method (methods
// are just fields containing functions).
//
// Returns (nil, false) if neither field nor method exists.
func FieldOrMethod(t typ.Type, name string) (typ.Type, bool) {
	if ft, ok := fieldDepth(t, name, 0); ok {
		return ft, true
	}

	return methodDepth(t, name, 0)
}

// Callable returns the function type if t is callable.
//
// A type is callable if it can be invoked as a function. This includes:
//   - Function types directly
//   - Records with __call metamethod
//   - Unions where all members are callable
//   - Intersections where any member is callable
//   - Generic types whose body is callable
//
// Returns (nil, false) if the type cannot be called.
func Callable(t typ.Type) (*typ.Function, bool) {
	return callableDepth(t, 0)
}

// callableDepth recursively checks callability with depth limiting.
func callableDepth(t typ.Type, depth int) (*typ.Function, bool) {
	if stopDepth(t, depth) {
		return nil, false
	}

	res := typ.Visit(t, typ.Visitor[callableResult]{
		Function: func(fn *typ.Function) callableResult {
			return callableResult{fn: fn, ok: true}
		},
		Optional: func(o *typ.Optional) callableResult {
			fn, ok := callableDepth(o.Inner, depth+1)
			return callableResult{fn: fn, ok: ok}
		},
		Recursive: func(rec *typ.Recursive) callableResult {
			if rec.Body == nil || rec.Body == rec {
				return callableResult{}
			}
			fn, ok := callableDepth(rec.Body, depth+1)
			return callableResult{fn: fn, ok: ok}
		},
		Alias: func(a *typ.Alias) callableResult {
			fn, ok := callableDepth(a.Target, depth+1)
			return callableResult{fn: fn, ok: ok}
		},
		Instantiated: func(inst *typ.Instantiated) callableResult {
			resolved, err := ResolveInstantiated(inst)
			if err != nil {
				return callableResult{}
			}

			fn, ok := callableDepth(resolved, depth+1)
			return callableResult{fn: fn, ok: ok}
		},
		Generic: func(g *typ.Generic) callableResult {
			fn, ok := callableDepth(g.Body, depth+1)
			return callableResult{fn: fn, ok: ok}
		},
		Union: func(u *typ.Union) callableResult {
			var result *typ.Function

			for _, m := range u.Members {
				f, ok := callableDepth(m, depth+1)
				if !ok {
					return callableResult{}
				}

				if result == nil {
					result = f
				}
			}

			return callableResult{fn: result, ok: result != nil}
		},
		Intersection: func(in *typ.Intersection) callableResult {
			for _, m := range in.Members {
				if f, ok := callableDepth(m, depth+1); ok {
					return callableResult{fn: f, ok: true}
				}
			}

			return callableResult{}
		},
		Record: func(r *typ.Record) callableResult {
			// Check for __call metamethod
			if r.Metatable == nil || typ.IsMetatableUnconstrained(r.Metatable) {
				return callableResult{}
			}

			callMeta, ok := fieldDepth(r.Metatable, "__call", depth+1)
			if !ok {
				return callableResult{}
			}

			if fn, ok := callMeta.(*typ.Function); ok {
				return callableResult{fn: fn, ok: true}
			}

			return callableResult{}
		},
		Default: func(t typ.Type) callableResult {
			return callableResult{}
		},
	})
	return res.fn, res.ok
}

// GetMetamethod returns the type of a metamethod if present.
//
// Metamethods are special fields in a type's metatable that customize behavior:
//   - __index: field access fallback
//   - __newindex: field assignment fallback
//   - __call: function call operator
//   - __add, __sub, __mul, etc.: arithmetic operators
//   - __eq, __lt, __le: comparison operators
//   - __tostring: string conversion
//   - __len: length operator
//
// Returns (nil, false) if the type has no metatable or lacks the metamethod.
func GetMetamethod(t typ.Type, name string) (typ.Type, bool) {
	return getMetamethodDepth(t, name, 0)
}

// getMetamethodDepth recursively looks up metamethods with depth limiting.
func getMetamethodDepth(t typ.Type, name string, depth int) (typ.Type, bool) {
	if stopDepth(t, depth) {
		return nil, false
	}

	res := typ.Visit(t, typ.Visitor[fieldResult]{
		Record: func(r *typ.Record) fieldResult {
			if r.Metatable == nil || typ.IsMetatableUnconstrained(r.Metatable) {
				return fieldResult{}
			}

			mt, ok := fieldDepth(r.Metatable, name, depth+1)
			return fieldResult{t: mt, ok: ok}
		},
		Optional: func(o *typ.Optional) fieldResult {
			mt, ok := getMetamethodDepth(o.Inner, name, depth+1)
			return fieldResult{t: mt, ok: ok}
		},
		Recursive: func(rec *typ.Recursive) fieldResult {
			if rec.Body == nil || rec.Body == rec {
				return fieldResult{}
			}
			mt, ok := getMetamethodDepth(rec.Body, name, depth+1)
			return fieldResult{t: mt, ok: ok}
		},
		Alias: func(a *typ.Alias) fieldResult {
			mt, ok := getMetamethodDepth(a.Target, name, depth+1)
			return fieldResult{t: mt, ok: ok}
		},
		Instantiated: func(inst *typ.Instantiated) fieldResult {
			resolved, err := ResolveInstantiated(inst)
			if err != nil {
				return fieldResult{}
			}

			mt, ok := getMetamethodDepth(resolved, name, depth+1)
			return fieldResult{t: mt, ok: ok}
		},
		Default: func(t typ.Type) fieldResult {
			return fieldResult{}
		},
	})
	return res.t, res.ok
}

// HasMetamethod returns true if the type has the specified metamethod.
//
// This is a convenience wrapper around GetMetamethod that discards the type.
func HasMetamethod(t typ.Type, name string) bool {
	_, ok := getMetamethodDepth(t, name, 0)
	return ok
}

// metaIsMethod returns the built-in :is method signature for Meta types.
//
// Meta types (reified type values) have a built-in :is method for runtime
// type guards: value:is(Type) returns (value|nil, err?).
// Signature: (value: any) -> (T?, LuaError?)
func metaIsMethod(of typ.Type) *typ.Function {
	if of == nil {
		of = typ.Any
	}
	return typ.Func().
		Param("value", typ.Any).
		Returns(typ.NewOptional(of), typ.NewOptional(typ.LuaError)).
		Effects(effect.WithTypeValueMethod()).
		Build()
}
