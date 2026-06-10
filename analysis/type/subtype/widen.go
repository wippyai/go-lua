package subtype

import (
	luatable "github.com/wippyai/go-lua/analysis/lua/table"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Widen converts literal types to their primitive base type.
//
// This is a shallow operation: it widens literal members in unions and optional
// inners, but it does not descend into records, containers, or functions.
func Widen(t typ.Type) typ.Type {
	return widenDepth(t, 0)
}

func stopWidenDepth(t typ.Type, depth int) bool {
	return t == nil || typ.DepthExceeded(depth)
}

func widenDepth(t typ.Type, depth int) typ.Type {
	if stopWidenDepth(t, depth) {
		return t
	}

	return typ.Visit(t, typ.Visitor[typ.Type]{
		Literal: func(lit *typ.Literal) typ.Type {
			switch lit.Base {
			case kind.Boolean:
				return typ.Boolean
			case kind.Integer:
				return typ.Integer
			case kind.Number:
				return typ.Number
			case kind.String:
				return typ.String
			default:
				return t
			}
		},
		Union: func(u *typ.Union) typ.Type {
			members := make([]typ.Type, len(u.Members))
			changed := false

			for i, m := range u.Members {
				members[i] = widenDepth(m, depth+1)
				if !typ.SameNode(members[i], m) {
					changed = true
				}
			}

			if !changed {
				return t
			}

			return typ.NewUnion(members...)
		},
		Optional: func(o *typ.Optional) typ.Type {
			inner := widenDepth(o.Inner, depth+1)
			if typ.SameNode(inner, o.Inner) {
				return t
			}

			return typ.NewOptional(inner)
		},
		Default: func(t typ.Type) typ.Type {
			return t
		},
	})
}

// WidenForInference performs deep literal widening across structural types.
func WidenForInference(t typ.Type) typ.Type {
	return widenForInferenceDepth(t, 0, false)
}

// WidenReturnTowerOnly performs deep widening while preserving function
// parameter types. It widens return types, variadics, and nested structural
// growth without flattening the contravariant parameter side.
func WidenReturnTowerOnly(t typ.Type) typ.Type {
	return widenForInferenceDepth(t, 0, true)
}

func widenForInferenceDepth(t typ.Type, depth int, preserveParams bool) typ.Type {
	if stopWidenDepth(t, depth) {
		return t
	}

	t = widenDepth(t, depth)

	return typ.Visit(t, typ.Visitor[typ.Type]{
		Tuple: func(tup *typ.Tuple) typ.Type {
			elems := make([]typ.Type, len(tup.Elements))
			changed := false
			for i, e := range tup.Elements {
				elems[i] = widenForInferenceDepth(e, depth+1, preserveParams)
				if !typ.SameNode(elems[i], e) {
					changed = true
				}
			}
			if !changed {
				return t
			}
			return typ.NewTuple(elems...)
		},
		Array: func(a *typ.Array) typ.Type {
			elem := widenForInferenceDepth(a.Element, depth+1, preserveParams)
			if typ.SameNode(elem, a.Element) {
				return t
			}
			return typ.NewArray(elem)
		},
		Map: func(m *typ.Map) typ.Type {
			key := widenForInferenceDepth(m.Key, depth+1, preserveParams)
			value := widenForInferenceDepth(m.Value, depth+1, preserveParams)
			if typ.SameNode(key, m.Key) && typ.SameNode(value, m.Value) {
				return t
			}
			return luatable.NewMap(key, value)
		},
		ReadonlyMap: func(m *typ.ReadonlyMap) typ.Type {
			key := widenForInferenceDepth(m.Key, depth+1, preserveParams)
			value := widenForInferenceDepth(m.Value, depth+1, preserveParams)
			if typ.SameNode(key, m.Key) && typ.SameNode(value, m.Value) {
				return t
			}
			return luatable.NewReadonlyMap(key, value)
		},
		Record: func(r *typ.Record) typ.Type {
			if len(r.Fields) > typ.DefaultRecursionDepth {
				fieldTypes := make([]typ.Type, 0, len(r.Fields))
				for _, f := range r.Fields {
					fieldTypes = append(fieldTypes, widenForInferenceDepth(f.Type, depth+1, preserveParams))
				}

				elem := typ.Unknown
				if len(fieldTypes) > 0 {
					elem = typ.NewUnion(fieldTypes...)
				}

				return luatable.NewMap(typ.String, elem)
			}

			builder := luatable.NewRecord()
			if r.Open {
				builder.SetOpen(true)
			}

			changed := false
			for _, f := range r.Fields {
				fieldType := widenForInferenceDepth(f.Type, depth+1, preserveParams)
				if !typ.SameNode(fieldType, f.Type) {
					changed = true
				}
				switch {
				case f.Optional && f.Readonly:
					builder.OptReadonlyField(f.Name, fieldType)
				case f.Optional:
					builder.OptField(f.Name, fieldType)
				case f.Readonly:
					builder.ReadonlyField(f.Name, fieldType)
				default:
					builder.Field(f.Name, fieldType)
				}
			}

			for _, m := range r.StaticMembers {
				member := m
				member.Type = widenForInferenceDepth(m.Type, depth+1, preserveParams)
				if !typ.SameNode(member.Type, m.Type) {
					changed = true
				}
				builder.AddStaticMember(member)
			}

			if r.Metatable != nil {
				metatable := widenForInferenceDepth(r.Metatable, depth+1, preserveParams)
				if !typ.SameNode(metatable, r.Metatable) {
					changed = true
				}
				builder.Metatable(metatable)
			}

			if r.HasMapComponent() {
				key := widenForInferenceDepth(r.MapKey, depth+1, preserveParams)
				value := widenForInferenceDepth(r.MapValue, depth+1, preserveParams)
				if !typ.SameNode(key, r.MapKey) || !typ.SameNode(value, r.MapValue) {
					changed = true
				}
				builder.MapComponent(key, value)
			}

			if !changed {
				return t
			}
			return builder.Build()
		},
		Function: func(fn *typ.Function) typ.Type {
			if len(fn.TypeParams) > 0 {
				return t
			}

			params := make([]typ.Param, len(fn.Params))
			changed := false
			for i, p := range fn.Params {
				paramType := p.Type
				if !preserveParams {
					paramType = widenForInferenceDepth(p.Type, depth+1, preserveParams)
				}
				params[i] = typ.Param{Name: p.Name, Type: paramType, Optional: p.Optional}
				if !typ.SameNode(paramType, p.Type) {
					changed = true
				}
			}

			var variadic typ.Type
			if fn.Variadic != nil {
				variadic = widenForInferenceDepth(fn.Variadic, depth+1, preserveParams)
				if !typ.SameNode(variadic, fn.Variadic) {
					changed = true
				}
			}

			returns := make([]typ.Type, len(fn.Returns))
			for i, ret := range fn.Returns {
				returns[i] = widenForInferenceDepth(ret, depth+1, preserveParams)
				if !typ.SameNode(returns[i], ret) {
					changed = true
				}
			}

			if !changed {
				return t
			}

			builder := typ.Func().Effects(fn.Effects)
			for _, p := range params {
				if p.Optional {
					builder.OptParam(p.Name, p.Type)
				} else {
					builder.Param(p.Name, p.Type)
				}
			}
			if variadic != nil {
				builder.Variadic(variadic)
			}
			return builder.Returns(returns...).Build()
		},
		Interface: func(in *typ.Interface) typ.Type {
			methods := make([]typ.Method, len(in.Methods))
			changed := false
			for i, m := range in.Methods {
				methodType := widenForInferenceDepth(m.Type, depth+1, preserveParams)
				methodFn, ok := methodType.(*typ.Function)
				if !ok {
					methodFn = m.Type
				}
				methods[i] = typ.Method{Name: m.Name, Type: methodFn}
				if !typ.SameNode(methodFn, m.Type) {
					changed = true
				}
			}
			if !changed {
				return t
			}
			return typ.NewInterface(in.Name, methods)
		},
		Default: func(t typ.Type) typ.Type {
			return t
		},
	})
}
