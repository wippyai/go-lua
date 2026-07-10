package typ

import "github.com/wippyai/go-lua/analysis/type/kind"

// IsUnknown reports whether t is explicitly the unknown type.
func IsUnknown(t Type) bool {
	return t != nil && t.Kind() == kind.Unknown
}

// IsAny reports whether t is explicitly the any type.
func IsAny(t Type) bool {
	return t != nil && t.Kind() == kind.Any
}

// IsNever reports whether t is explicitly the never type.
func IsNever(t Type) bool {
	return t != nil && t.Kind() == kind.Never
}

// IsTopLike reports whether t is an explicit top-like type.
//
// A missing type (nil) is not top-like; use AbsentOrTopLike when absence should
// be treated as unknown by a caller.
func IsTopLike(t Type) bool {
	return IsAny(t) || IsUnknown(t)
}

// AbsentOrTopLike reports whether t is missing or explicitly top-like.
func AbsentOrTopLike(t Type) bool {
	return t == nil || IsTopLike(t)
}

// AdmitsFalse reports whether t's value set can contain the literal false.
func AdmitsFalse(t Type) bool {
	return admitsFalse(t, 0)
}

func admitsFalse(t Type, depth int) bool {
	if depth > DefaultRecursionDepth {
		return false
	}
	switch tt := t.(type) {
	case nil:
		return false
	case *Literal:
		return tt.Base == kind.Boolean && tt.Value == false
	case *Annotated:
		if tt == nil || tt.Inner == nil || tt.Inner == t {
			return false
		}
		return admitsFalse(tt.Inner, depth+1)
	case *Alias:
		if tt == nil {
			return false
		}
		next := tt.UnaliasedTarget()
		if next == nil || next == t {
			return false
		}
		return admitsFalse(next, depth+1)
	case *Optional:
		return tt != nil && admitsFalse(tt.Inner, depth+1)
	case *Union:
		for _, member := range tt.Members {
			if admitsFalse(member, depth+1) {
				return true
			}
		}
		return false
	case *Intersection:
		if len(tt.Members) == 0 {
			return false
		}
		for _, member := range tt.Members {
			if !admitsFalse(member, depth+1) {
				return false
			}
		}
		return true
	default:
		return TypeEquals(tt, Boolean)
	}
}

// IsBooleanType reports whether t is definitely contained in boolean.
func IsBooleanType(t Type) bool {
	return isBooleanType(t, 0)
}

func isBooleanType(t Type, depth int) bool {
	if depth > DefaultRecursionDepth {
		return false
	}
	t = NormalizeNil(t)
	if t == nil {
		return false
	}
	switch tt := stripIndexTransparentWrappers(t, depth).(type) {
	case nil:
		return false
	case *Literal:
		return tt.Base == kind.Boolean
	case *Optional:
		return false
	case *Union:
		if len(tt.Members) == 0 {
			return false
		}
		for _, member := range tt.Members {
			if !isBooleanType(member, depth+1) {
				return false
			}
		}
		return true
	case *Intersection:
		for _, member := range tt.Members {
			if isBooleanType(member, depth+1) {
				return true
			}
		}
		return false
	default:
		return TypeEquals(tt, Boolean)
	}
}

// IsIntegerIndexType reports whether t is definitely usable as an integer
// index. It is a type-domain predicate used by analyses that need proof that a
// dynamic key ranges over integer slots, such as ipairs-style generic-for
// transfer.
func IsIntegerIndexType(t Type) bool {
	return isIntegerIndexType(t, 0)
}

func isIntegerIndexType(t Type, depth int) bool {
	if depth > DefaultRecursionDepth {
		return false
	}
	t = NormalizeNil(t)
	if t == nil {
		return false
	}
	switch tt := stripIndexTransparentWrappers(t, depth).(type) {
	case nil:
		return false
	case *Literal:
		return tt.Base == kind.Integer
	case *Optional:
		return false
	case *Union:
		if len(tt.Members) == 0 {
			return false
		}
		for _, member := range tt.Members {
			if !isIntegerIndexType(member, depth+1) {
				return false
			}
		}
		return true
	case *Intersection:
		for _, member := range tt.Members {
			if isIntegerIndexType(member, depth+1) {
				return true
			}
		}
		return false
	default:
		return TypeEquals(tt, Integer)
	}
}

func stripIndexTransparentWrappers(t Type, depth int) Type {
	for ; depth <= DefaultRecursionDepth; depth++ {
		switch tt := t.(type) {
		case *Annotated:
			if tt == nil || tt.Inner == nil || tt.Inner == t {
				return t
			}
			t = tt.Inner
		case *Alias:
			if tt == nil {
				return nil
			}
			next := tt.UnaliasedTarget()
			if next == nil || next == t {
				return next
			}
			t = next
		default:
			return t
		}
	}
	return nil
}

// AbsentOrUnknown reports whether t is missing (nil) or unknown.
//
// This intentionally does not treat the explicit nil type as unknown.
func AbsentOrUnknown(t Type) bool {
	return t == nil || IsUnknown(t)
}
