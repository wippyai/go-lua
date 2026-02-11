package ops

import (
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

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
	return isNumericGuard(t, typ.NewGuard())
}

func isNumericGuard(t typ.Type, guard internal.RecursionGuard) bool {
	if t == nil {
		return false
	}
	next, ok := guard.Enter(t)
	if !ok {
		return false
	}

	switch v := t.(type) {
	case *typ.Union:
		for _, m := range v.Members {
			if !isNumericGuard(m, next) {
				return false
			}
		}

		return len(v.Members) > 0

	case *typ.Intersection:
		for _, m := range v.Members {
			if !isNumericGuard(m, next) {
				return false
			}
		}

		return len(v.Members) > 0

	case *typ.Alias:
		if v.Target == nil {
			return false
		}

		return isNumericGuard(v.Target, next)

	case *typ.Optional:
		// Optional types are NOT numeric - must be narrowed first
		return false

	case *typ.Literal:
		switch v.Value.(type) {
		case float64, int64:
			return true
		default:
			return false
		}

	case *typ.TypeParam:
		if v.Constraint != nil {
			return isNumericGuard(v.Constraint, next)
		}

		return false

	default:
		k := t.Kind()
		return k == kind.Number || k == kind.Integer || k.IsPlaceholder()
	}
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
	return isOrderableGuard(t, typ.NewGuard())
}

func isOrderableGuard(t typ.Type, guard internal.RecursionGuard) bool {
	if t == nil {
		return false
	}
	next, ok := guard.Enter(t)
	if !ok {
		return false
	}

	switch v := t.(type) {
	case *typ.Union:
		for _, m := range v.Members {
			if !isOrderableGuard(m, next) {
				return false
			}
		}

		return len(v.Members) > 0

	case *typ.Intersection:
		for _, m := range v.Members {
			if !isOrderableGuard(m, next) {
				return false
			}
		}

		return len(v.Members) > 0

	case *typ.Alias:
		if v.Target == nil {
			return false
		}

		return isOrderableGuard(v.Target, next)

	case *typ.Optional:
		return false

	case *typ.Literal:
		switch v.Value.(type) {
		case float64, int64, string:
			return true
		default:
			return false
		}

	case *typ.TypeParam:
		if v.Constraint != nil {
			return isOrderableGuard(v.Constraint, next)
		}

		return false

	default:
		k := t.Kind()
		return k == kind.Number || k == kind.Integer || k == kind.String || k.IsPlaceholder()
	}
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
	return isStringableGuard(t, typ.NewGuard())
}

func isStringableGuard(t typ.Type, guard internal.RecursionGuard) bool {
	if t == nil {
		return false
	}

	t = ExtractFirstValue(t)
	if t == nil {
		return false
	}
	next, ok := guard.Enter(t)
	if !ok {
		return false
	}
	if t != nil && t.Equals(typ.LuaError) {
		return true
	}

	switch v := t.(type) {
	case *typ.Union:
		for _, m := range v.Members {
			if !isStringableGuard(m, next) {
				return false
			}
		}

		return len(v.Members) > 0

	case *typ.Intersection:
		for _, m := range v.Members {
			if !isStringableGuard(m, next) {
				return false
			}
		}

		return len(v.Members) > 0

	case *typ.Alias:
		if v.Target == nil {
			return false
		}

		return isStringableGuard(v.Target, next)

	case *typ.Optional:
		return false

	case *typ.Literal:
		switch v.Value.(type) {
		case string, float64, int64:
			return true
		default:
			return false
		}

	case *typ.TypeParam:
		if v.Constraint != nil {
			return isStringableGuard(v.Constraint, next)
		}

		return false
	case *typ.Interface:
		if v.Name == "Error" {
			return true
		}

		return false

	default:
		k := t.Kind()
		if k.IsPlaceholder() {
			return true
		}

		if k == kind.String || k == kind.Number || k == kind.Integer {
			return true
		}
		// Types with __tostring metamethod are subtypes of string
		return subtype.IsSubtype(t, typ.String)
	}
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
	return hasLengthGuard(t, typ.NewGuard())
}

func hasLengthGuard(t typ.Type, guard internal.RecursionGuard) bool {
	if t == nil {
		return false
	}

	t = ExtractFirstValue(t)
	if t == nil {
		return false
	}
	next, ok := guard.Enter(t)
	if !ok {
		return false
	}

	switch v := t.(type) {
	case *typ.Array, *typ.Map, *typ.Record, *typ.Tuple:
		return true

	case *typ.Union:
		for _, m := range v.Members {
			if !hasLengthGuard(m, next) {
				return false
			}
		}

		return len(v.Members) > 0

	case *typ.Intersection:
		for _, m := range v.Members {
			if !hasLengthGuard(m, next) {
				return false
			}
		}

		return len(v.Members) > 0

	case *typ.Alias:
		if v.Target == nil {
			return false
		}

		return hasLengthGuard(v.Target, next)

	case *typ.Optional:
		return false

	case *typ.Literal:
		_, isStr := v.Value.(string)
		return isStr

	case *typ.TypeParam:
		if v.Constraint != nil {
			return hasLengthGuard(v.Constraint, next)
		}

		return false

	default:
		k := t.Kind()
		if k.IsPlaceholder() {
			return true
		}

		return k == kind.String
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
// Unlike IsNumeric, this does not recursively check unions/aliases.
func IsBitwiseNumeric(t typ.Type) bool {
	if t == nil {
		return false
	}

	k := t.Kind()

	return k == kind.Integer || k == kind.Number || k.IsPlaceholder()
}

// IsLiteralBoolean checks if a type is a literal boolean (true or false).
func IsLiteralBoolean(t typ.Type) bool {
	if t == nil {
		return false
	}
	if lit, ok := t.(*typ.Literal); ok {
		return lit.Base == kind.Boolean
	}
	return false
}
