package core

import (
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/union"
)

// CompatibleFunctionFromUnion finds a compatible function signature from union members.
//
// When a union type contains multiple function signatures, this selects or merges
// them based on the expected parameter count. The selection strategy:
//
//  1. If only one function exists, return it
//  2. Keep only functions whose arity can accept paramCount
//     (respecting required params, optional params, and variadics)
//  3. If multiple candidates remain, merge them into a combined signature
//
// This is used during function call type checking when the callee has a union
// type containing multiple function overloads.
//
// Returns nil if no function types exist in the union or no function is arity-compatible.
func CompatibleFunctionFromUnion(paramCount int, expected typ.Type) *typ.Function {
	fns := union.FunctionTypes(expected)
	if len(fns) == 0 {
		return nil
	}

	if len(fns) == 1 {
		if acceptsArity(fns[0], paramCount) {
			return fns[0]
		}
		return nil
	}

	var matching []*typ.Function
	for _, f := range fns {
		if acceptsArity(f, paramCount) {
			matching = append(matching, f)
		}
	}

	if len(matching) == 0 {
		return nil
	}

	if len(matching) == 1 {
		return matching[0]
	}

	return union.MergeFunctions(matching)
}

func acceptsArity(fn *typ.Function, argCount int) bool {
	if fn == nil {
		return false
	}
	if argCount < typ.MinRequiredArgs(fn) {
		return false
	}
	if fn.Variadic != nil {
		return true
	}
	return argCount <= len(fn.Params)
}

// joinProjections joins the types projected from the alternative shapes a
// single value may take: the members of a union, or the record fields a key
// domain may select.
//
// typ.NewUnion treats Unknown as the identity of inference joins, where it
// stands for a result that is not resolved yet. A projected Unknown is
// different: the alternative exists and its projected value is opaque, so the
// value set of the projection is unbounded and Unknown absorbs the join.
// Any still dominates, and a nil-bearing alternative keeps the result optional.
func joinProjections(types ...typ.Type) typ.Type {
	joined := typ.NewUnion(types...)
	if typ.IsAny(joined) {
		return joined
	}
	opaque := false
	for _, t := range types {
		if projectsUnknown(t) {
			opaque = true
			break
		}
	}
	if !opaque {
		return joined
	}
	if containsNilOrOptional(joined) || joined.Kind() == kind.Nil {
		return typ.NewOptional(typ.Unknown)
	}
	return typ.Unknown
}

// projectsUnknown reports whether a projected type is Unknown or Unknown?.
func projectsUnknown(t typ.Type) bool {
	if t == nil {
		return false
	}
	t = typ.UnwrapAnnotated(t)
	if typ.IsUnknown(t) {
		return true
	}
	if opt, ok := t.(*typ.Optional); ok {
		return typ.IsUnknown(typ.UnwrapAnnotated(opt.Inner))
	}
	return false
}
