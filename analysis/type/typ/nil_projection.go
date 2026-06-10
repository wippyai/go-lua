package typ

import "github.com/wippyai/go-lua/analysis/type/kind"

// NilProjectionMode controls how alias wrappers are handled while removing
// explicit nil alternatives from a type representation.
type NilProjectionMode uint8

const (
	// NilProjectionStructural returns the structural non-nil payload.
	NilProjectionStructural NilProjectionMode = iota
	// NilProjectionPreserveAliases rebuilds aliases whose targets lose nil.
	NilProjectionPreserveAliases
)

// WithoutNil removes explicit typ.Nil capability from a type representation.
//
// It is intentionally representation-level only: a missing Go nil Type is not
// treated as typ.Unknown and does not imply Lua table/key policy.
func WithoutNil(t Type, mode NilProjectionMode) (nonNil Type, nilable bool) {
	t = NormalizeNilType(t)
	if t == nil {
		return nil, false
	}
	return withoutNil(t, mode)
}

func withoutNil(t Type, mode NilProjectionMode) (Type, bool) {
	t = NormalizeNilType(t)
	if t == nil {
		return nil, false
	}

	switch v := t.(type) {
	case *Annotated:
		inner, nilable := withoutNil(v.Inner, mode)
		if !nilable {
			return t, false
		}
		if inner == nil {
			inner = Never
		}
		return NewAnnotated(inner, v.Annotations), true
	case *Alias:
		target, nilable := withoutNil(v.Target, mode)
		if !nilable {
			return t, false
		}
		if target == nil {
			target = Never
		}
		if mode == NilProjectionPreserveAliases {
			return NewAlias(v.Name, target), true
		}
		return target, true
	case *Optional:
		if v.Inner == nil {
			return Never, true
		}
		inner, _ := withoutNil(v.Inner, mode)
		if inner == nil {
			inner = Never
		}
		return inner, true
	case *Union:
		nilable := false
		projected := ProjectUnionMembers(v, func(member Type) Type {
			member = NormalizeNilType(member)
			if member == nil {
				return Never
			}
			nonNil, memberNilable := withoutNil(member, mode)
			if memberNilable {
				nilable = true
			}
			if nonNil == nil {
				return member
			}
			return nonNil
		})
		if !nilable {
			return t, false
		}
		return projected, true
	default:
		if t.Kind() == kind.Nil {
			return Never, true
		}
		return t, false
	}
}
