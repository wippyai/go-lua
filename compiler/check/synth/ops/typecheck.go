package ops

import (
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

type typePredicate struct {
	allBranches         bool
	optionalInner       bool
	recursiveBody       bool
	typeParamConstraint bool
	prepare             func(typ.Type) typ.Type
	beforeGuard         func(typ.Type) bool
	afterGuard          func(typ.Type) bool
	leaf                func(typ.Type) bool
}

func (p typePredicate) matches(t typ.Type, guard internal.RecursionGuard) bool {
	if t == nil {
		return false
	}
	if p.prepare != nil {
		t = p.prepare(t)
		if t == nil {
			return false
		}
	}
	if p.beforeGuard != nil && p.beforeGuard(t) {
		return true
	}
	next, ok := guard.Enter(t)
	if !ok {
		return false
	}
	if p.afterGuard != nil && p.afterGuard(t) {
		return true
	}

	var branches []typ.Type
	isBranches := false
	switch v := t.(type) {
	case *typ.Union:
		branches, isBranches = v.Members, true
	case *typ.Intersection:
		branches, isBranches = v.Members, true
	case *typ.Alias:
		return v.Target != nil && p.matches(v.Target, next)
	case *typ.Optional:
		return p.optionalInner && v.Inner != nil && p.matches(v.Inner, next)
	case *typ.Recursive:
		if p.recursiveBody {
			return v.Body != nil && p.matches(v.Body, next)
		}
	case *typ.TypeParam:
		if p.typeParamConstraint {
			return v.Constraint != nil && p.matches(v.Constraint, next)
		}
	}
	if isBranches {
		if p.allBranches {
			if len(branches) == 0 {
				return false
			}
			for _, branch := range branches {
				if !p.matches(branch, next) {
					return false
				}
			}
			return true
		}
		for _, branch := range branches {
			if p.matches(branch, next) {
				return true
			}
		}
		return false
	}
	return p.leaf != nil && p.leaf(t)
}

// IsNumeric checks if a type supports arithmetic operations (+, -, *, /, %).
//
// A type is numeric if:
//   - It has kind Number or Integer
//   - It's a literal number (int64 or float64)
//   - It's a union/intersection where all members are numeric
//   - It's an alias to a numeric type
//   - It's a type parameter with a numeric constraint
//
// Optional types are NOT numeric - they must be narrowed first.
// Placeholder types (any, unknown) are considered numeric for flexibility.
func IsNumeric(t typ.Type) bool {
	return (typePredicate{
		allBranches:         true,
		recursiveBody:       true,
		typeParamConstraint: true,
		leaf:                numericPredicateLeaf,
	}).matches(t, typ.NewGuard())
}

func numericPredicateLeaf(t typ.Type) bool {
	if lit, ok := t.(*typ.Literal); ok {
		switch lit.Value.(type) {
		case float64, int64:
			return true
		default:
			return false
		}
	}
	k := t.Kind()
	return k == kind.Number || k == kind.Integer || k.IsPlaceholder()
}

// IsOrderable checks if a type supports comparison operators (<, <=, >, >=).
//
// A type is orderable if:
//   - It has kind Number, Integer, or String
//   - It's a literal number or string
//   - It's a union/intersection where all members are orderable
//   - It's an alias to an orderable type
//
// Notably, booleans and tables are NOT orderable in Lua.
func IsOrderable(t typ.Type) bool {
	return (typePredicate{
		allBranches:         true,
		typeParamConstraint: true,
		leaf:                orderablePredicateLeaf,
	}).matches(t, typ.NewGuard())
}

func orderablePredicateLeaf(t typ.Type) bool {
	if lit, ok := t.(*typ.Literal); ok {
		switch lit.Value.(type) {
		case float64, int64, string:
			return true
		default:
			return false
		}
	}
	k := t.Kind()
	return k == kind.Number || k == kind.Integer || k == kind.String || k.IsPlaceholder()
}

// MayBeOrderable checks whether a type could support ordering operators
// (<, <=, >, >=). Unlike IsOrderable (all branches), this is conservative:
// true if any feasible branch is orderable.
func MayBeOrderable(t typ.Type) bool {
	return (typePredicate{
		optionalInner:       true,
		typeParamConstraint: true,
		leaf:                orderablePredicateLeaf,
	}).matches(t, typ.NewGuard())
}

// IsStringable checks if a type can be used with string concatenation (..).
//
// In Lua, both strings and numbers can be concatenated (numbers are coerced).
// Types implementing Error interface or with __tostring metamethod also work.
//
// A type is stringable if:
//   - It has kind String, Number, or Integer
//   - It's the Error interface type
//   - It's a subtype of string (has __tostring)
//   - It's a literal string or number
//   - It's a union/intersection where all members are stringable
func IsStringable(t typ.Type) bool {
	return (typePredicate{
		allBranches:         true,
		typeParamConstraint: true,
		prepare:             ExtractFirstValue,
		afterGuard:          isLuaErrorType,
		leaf:                stringablePredicateLeaf,
	}).matches(t, typ.NewGuard())
}

func isLuaErrorType(t typ.Type) bool {
	return t.Equals(typ.LuaError)
}

func stringablePredicateLeaf(t typ.Type) bool {
	if lit, ok := t.(*typ.Literal); ok {
		switch lit.Value.(type) {
		case string, float64, int64:
			return true
		default:
			return false
		}
	}
	if iface, ok := t.(*typ.Interface); ok {
		// Error values convert through their __tostring and __concat metamethods.
		return typ.TypeEquals(iface, typ.LuaError)
	}
	k := t.Kind()
	if k.IsPlaceholder() || k == kind.String || k == kind.Number || k == kind.Integer {
		return true
	}
	// Types with __tostring metamethod are subtypes of string.
	return subtype.IsSubtype(t, typ.String)
}

// MayBeStringable checks whether a type could participate in string
// concatenation. Unlike IsStringable (which requires all branches to be
// stringable), this returns true if any feasible branch is stringable.
func MayBeStringable(t typ.Type) bool {
	return (typePredicate{
		optionalInner:       true,
		typeParamConstraint: true,
		leaf:                stringablePredicateLeaf,
	}).matches(t, typ.NewGuard())
}

// HasLength checks if a type supports the length operator (#).
//
// In Lua, the following types have length:
//   - Strings (byte count)
//   - Arrays (element count)
//   - Tables/records (via __len metamethod or table length)
//   - Tuples (element count)
//   - Maps (entry count)
func HasLength(t typ.Type) bool {
	return (typePredicate{
		allBranches:         true,
		typeParamConstraint: true,
		beforeGuard:         unwrap.IsBuiltinTableTop,
		leaf:                lengthPredicateLeaf,
	}).matches(t, typ.NewGuard())
}

// MayHaveLength checks whether a type could support the length operator (#).
//
// This is a conservative predicate used by diagnostics: it returns true when
// any feasible runtime value may have length, avoiding false positives for
// optional/union values that can be length-capable after control-flow guards.
func MayHaveLength(t typ.Type) bool {
	return (typePredicate{
		optionalInner:       true,
		typeParamConstraint: true,
		beforeGuard:         unwrap.IsBuiltinTableTop,
		leaf:                lengthPredicateLeaf,
	}).matches(t, typ.NewGuard())
}

func lengthPredicateLeaf(t typ.Type) bool {
	switch v := t.(type) {
	case *typ.Array, *typ.Map, *typ.Record, *typ.Tuple:
		return true
	case *typ.Literal:
		_, isString := v.Value.(string)
		return isString
	default:
		return t.Kind().IsPlaceholder() || t.Kind() == kind.String
	}
}

// IsStringOnly checks if type is string (not number).
func IsStringOnly(t typ.Type) bool {
	if t == nil {
		return false
	}

	if lit, ok := t.(*typ.Literal); ok {
		_, isStr := lit.Value.(string)
		return isStr
	}

	return t.Kind() == kind.String
}

// IsBitwiseNumeric checks if a type supports bitwise operators (&, |, ~, <<, >>).
//
// Only integer and number types (and placeholders) support bitwise operations.
// Optional types are rejected until narrowed.
func IsBitwiseNumeric(t typ.Type) bool {
	return (typePredicate{
		allBranches: true,
		leaf:        bitwiseNumericPredicateLeaf,
	}).matches(t, typ.NewGuard())
}

func bitwiseNumericPredicateLeaf(t typ.Type) bool {
	if lit, ok := t.(*typ.Literal); ok {
		return lit.Base == kind.Integer || lit.Base == kind.Number
	}
	k := t.Kind()
	return k == kind.Integer || k == kind.Number || k.IsPlaceholder()
}
