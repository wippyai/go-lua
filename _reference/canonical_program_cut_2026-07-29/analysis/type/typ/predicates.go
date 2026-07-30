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
	return admitsFalse(t, &typePath{})
}

func admitsFalse(t Type, active *typePath) bool {
	switch tt := t.(type) {
	case nil:
		return false
	case *Literal:
		return tt.Base == kind.Boolean && tt.Value == false
	case *Annotated:
		if tt == nil || tt.Inner == nil || tt.Inner == t {
			return false
		}
		if !active.enter(t) {
			return false
		}
		defer active.leave(t)
		return admitsFalse(tt.Inner, active)
	case *Alias:
		if tt == nil {
			return false
		}
		next := tt.UnaliasedTarget()
		if next == nil || next == t {
			return false
		}
		if !active.enter(t) {
			return false
		}
		defer active.leave(t)
		return admitsFalse(next, active)
	case *Optional:
		return tt != nil && admitsFalse(tt.Inner, active)
	case *Union:
		for _, member := range tt.Members {
			if admitsFalse(member, active) {
				return true
			}
		}
		return false
	case *Intersection:
		if len(tt.Members) == 0 {
			return false
		}
		for _, member := range tt.Members {
			if !admitsFalse(member, active) {
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
	return isBooleanType(t, &typePath{})
}

func isBooleanType(t Type, active *typePath) bool {
	t = NormalizeNil(t)
	if t == nil {
		return false
	}
	t = stripIndexTransparentWrappers(t)
	if t == nil || !active.enter(t) {
		return false
	}
	defer active.leave(t)
	switch tt := t.(type) {
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
			if !isBooleanType(member, active) {
				return false
			}
		}
		return true
	case *Intersection:
		for _, member := range tt.Members {
			if isBooleanType(member, active) {
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
	return isIntegerIndexType(t, &typePath{})
}

func isIntegerIndexType(t Type, active *typePath) bool {
	t = NormalizeNil(t)
	if t == nil {
		return false
	}
	t = stripIndexTransparentWrappers(t)
	if t == nil || !active.enter(t) {
		return false
	}
	defer active.leave(t)
	switch tt := t.(type) {
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
			if !isIntegerIndexType(member, active) {
				return false
			}
		}
		return true
	case *Intersection:
		for _, member := range tt.Members {
			if isIntegerIndexType(member, active) {
				return true
			}
		}
		return false
	default:
		return TypeEquals(tt, Integer)
	}
}

func stripIndexTransparentWrappers(t Type) Type {
	var seen typePath
	for {
		if !seen.enter(t) {
			return nil
		}
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
}

// AbsentOrUnknown reports whether t is missing (nil) or unknown.
//
// This intentionally does not treat the explicit nil type as unknown.
func AbsentOrUnknown(t Type) bool {
	return t == nil || IsUnknown(t)
}
