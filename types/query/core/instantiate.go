package core

import (
	"errors"

	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
)

// Errors returned by generic instantiation functions.
var (
	// ErrNotGeneric indicates an attempt to instantiate a non-generic type.
	ErrNotGeneric = errors.New("type is not generic")

	// ErrTypeArgCount indicates a mismatch between type parameters and arguments.
	ErrTypeArgCount = errors.New("wrong number of type arguments")

	// ErrConstraintViolation indicates a type argument that doesn't satisfy its constraint.
	ErrConstraintViolation = errors.New("type argument violates constraint")
)

// InstantiateGeneric substitutes type arguments into a generic type body.
//
// This is the core generic instantiation function. Given a generic type
// definition and concrete type arguments, it:
//  1. Validates the argument count matches the parameter count
//  2. Validates each argument satisfies its parameter's constraint
//  3. Substitutes parameters by their declaration identity throughout the body
//
// Example:
//
//	// Given: type List<T> = {items: T[]}
//	// Call:  InstantiateGeneric(List, [number])
//	// Result: {items: number[]}
//
// Returns an error if the generic is nil, argument count is wrong, or any
// argument violates its constraint.
func InstantiateGeneric(g *typ.Generic, typeArgs []typ.Type) (typ.Type, error) {
	if g == nil {
		return nil, ErrNotGeneric
	}

	if len(typeArgs) != len(g.TypeParams) {
		return nil, ErrTypeArgCount
	}

	// Validate constraints
	for i, arg := range typeArgs {
		constraint := g.TypeParams[i].Constraint
		if constraint != nil && !subtype.IsSubtype(arg, constraint) {
			return nil, ErrConstraintViolation
		}
	}

	return subst.Params(g.Body, g.TypeParams, typeArgs), nil
}

// ResolveInstantiated fully resolves an Instantiated type to its body.
//
// An Instantiated type represents a generic type with type arguments already
// bound (e.g., List<number>). This function expands it to the concrete type
// by substituting the arguments into the generic body.
//
// This is a convenience wrapper around InstantiateGeneric that extracts the
// generic and type arguments from the Instantiated node.
func ResolveInstantiated(inst *typ.Instantiated) (typ.Type, error) {
	return InstantiateGeneric(inst.Generic, inst.TypeArgs)
}

// CollectTypeParams returns all type parameters found in a type.
//
// This traverses the type structure and collects all TypeParam nodes,
// which is useful for:
//   - Determining if a type is fully concrete or still parameterized
//   - Identifying which parameters need inference during type checking
//   - Validating that all parameters are bound before instantiation
//
// Returns an empty slice if the type contains no type parameters.
func CollectTypeParams(t typ.Type) []*typ.TypeParam {
	var params []*typ.TypeParam

	collectTypeParamsVisited(t, &params, make(map[typ.Type]bool))

	return params
}

// collectTypeParamsVisited recursively collects type parameters with cycle detection.
func collectTypeParamsVisited(t typ.Type, params *[]*typ.TypeParam, visited map[typ.Type]bool) {
	if t == nil || visited[t] {
		return
	}

	visited[t] = true

	typ.Visit(t, typ.Visitor[struct{}]{
		TypeParam: func(tp *typ.TypeParam) struct{} {
			*params = append(*params, tp)
			return struct{}{}
		},
		Function: func(fn *typ.Function) struct{} {
			for _, p := range fn.Params {
				collectTypeParamsVisited(p.Type, params, visited)
			}

			collectTypeParamsVisited(fn.Variadic, params, visited)

			for _, r := range fn.Returns {
				collectTypeParamsVisited(r, params, visited)
			}
			return struct{}{}
		},
		Record: func(r *typ.Record) struct{} {
			for _, f := range r.Fields {
				collectTypeParamsVisited(f.Type, params, visited)
			}
			return struct{}{}
		},
		Array: func(a *typ.Array) struct{} {
			collectTypeParamsVisited(a.Element, params, visited)
			return struct{}{}
		},
		Map: func(m *typ.Map) struct{} {
			collectTypeParamsVisited(m.Key, params, visited)
			collectTypeParamsVisited(m.Value, params, visited)
			return struct{}{}
		},
		Tuple: func(tup *typ.Tuple) struct{} {
			for _, e := range tup.Elements {
				collectTypeParamsVisited(e, params, visited)
			}
			return struct{}{}
		},
		Optional: func(o *typ.Optional) struct{} {
			collectTypeParamsVisited(o.Inner, params, visited)
			return struct{}{}
		},
		Union: func(u *typ.Union) struct{} {
			for _, m := range u.Members {
				collectTypeParamsVisited(m, params, visited)
			}
			return struct{}{}
		},
		Intersection: func(in *typ.Intersection) struct{} {
			for _, m := range in.Members {
				collectTypeParamsVisited(m, params, visited)
			}
			return struct{}{}
		},
		Ref: func(r *typ.Ref) struct{} {
			// Refs are unresolved name references - no type params inside
			return struct{}{}
		},
		Alias: func(a *typ.Alias) struct{} {
			collectTypeParamsVisited(a.Target, params, visited)
			return struct{}{}
		},
		Instantiated: func(inst *typ.Instantiated) struct{} {
			for _, a := range inst.TypeArgs {
				collectTypeParamsVisited(a, params, visited)
			}
			return struct{}{}
		},
		Default: func(t typ.Type) struct{} {
			return struct{}{}
		},
	})
}

// HasTypeParams returns true if the type contains any type parameters.
//
// A type with parameters is not fully concrete and requires instantiation
// before it can be used for runtime values. This is a quick check that
// avoids building the full parameter list when only presence is needed.
func HasTypeParams(t typ.Type) bool {
	return len(CollectTypeParams(t)) > 0
}
