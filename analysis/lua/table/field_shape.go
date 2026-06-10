package table

import "github.com/wippyai/go-lua/analysis/type/typ"

// SplitNilableField converts a nil-capable table field value into an optional
// field shape.
func SplitNilableField(t typ.Type) (inner typ.Type, optional bool) {
	if t == nil {
		return typ.Unknown, true
	}
	if a, ok := t.(*typ.Alias); ok {
		if a == nil || a.Target == nil {
			return t, false
		}
		if opt, ok := a.Target.(*typ.Optional); ok && opt != nil && opt.Inner != nil {
			return opt.Inner, true
		}
		if u, ok := a.Target.(*typ.Union); ok && u != nil && len(u.Members) > 0 {
			return splitNilableUnionMembers(u.Members, t)
		}
		return t, false
	}
	if opt, ok := t.(*typ.Optional); ok && opt != nil && opt.Inner != nil {
		return opt.Inner, true
	}
	if u, ok := t.(*typ.Union); ok && u != nil && len(u.Members) > 0 {
		return splitNilableUnionMembers(u.Members, t)
	}
	return t, false
}

func splitNilableUnionMembers(members []typ.Type, original typ.Type) (typ.Type, bool) {
	hasNil := false
	nonNil := make([]typ.Type, 0, len(members))
	for _, m := range members {
		if m != nil && m.Kind() == typ.Nil.Kind() {
			hasNil = true
			continue
		}
		nonNil = append(nonNil, m)
	}
	if !hasNil {
		return original, false
	}
	switch len(nonNil) {
	case 0:
		return typ.Nil, true
	case 1:
		return nonNil[0], true
	default:
		return typ.NewUnion(nonNil...), true
	}
}
