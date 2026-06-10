package typ

import "github.com/wippyai/go-lua/analysis/type/kind"

func normalizeOptionalFieldType(t Type) Type {
	if t == nil {
		return Unknown
	}
	switch v := t.(type) {
	case *Annotated:
		inner := normalizeOptionalFieldType(v.Inner)
		if inner == v.Inner {
			return t
		}
		return NewAnnotated(inner, v.Annotations)
	case *Alias:
		return t
	case *Optional:
		if v.Inner == nil || v.Inner.Kind() == kind.Never || v.Inner.Kind() == kind.Nil {
			return t
		}
		return v.Inner
	case *Union:
		nonNil := optionalFieldUnionWithoutNil(v)
		if nonNil == nil || nonNil.Kind() == kind.Never {
			return t
		}
		return nonNil
	default:
		return t
	}
}

func optionalFieldUnionWithoutNil(u *Union) Type {
	if u == nil {
		return nil
	}
	kept := make([]Type, 0, len(u.Members))
	for _, member := range u.Members {
		kept = appendOptionalFieldNonNilMember(kept, member)
	}
	if len(kept) == 0 {
		return Never
	}
	return NewUnion(kept...)
}

func appendOptionalFieldNonNilMember(out []Type, t Type) []Type {
	if t == nil {
		return out
	}
	switch v := UnwrapAnnotated(t).(type) {
	case nil:
		return out
	case *Optional:
		return appendOptionalFieldNonNilMember(out, v.Inner)
	case *Union:
		for _, member := range v.Members {
			out = appendOptionalFieldNonNilMember(out, member)
		}
		return out
	default:
		if v.Kind() == kind.Nil || v.Kind() == kind.Never {
			return out
		}
		return append(out, t)
	}
}
