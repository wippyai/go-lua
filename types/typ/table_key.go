package typ

// NormalizeTableKey removes impossible nil alternatives from Lua table key
// domains. Nil from iterators is the termination sentinel; it is never an
// inhabited table key.
func NormalizeTableKey(t Type) Type {
	if t == nil {
		return Unknown
	}
	switch v := t.(type) {
	case *Annotated:
		inner := NormalizeTableKey(v.Inner)
		if inner == v.Inner {
			return t
		}
		return NewAnnotated(inner, v.Annotations)
	case *Alias:
		inner := NormalizeTableKey(v.Target)
		if inner == nil || IsNever(inner) {
			return inner
		}
		if inner == v.Target {
			return t
		}
		return NewAlias(v.Name, inner)
	case *Optional:
		return NormalizeTableKey(v.Inner)
	case *Union:
		members := make([]Type, 0, len(v.Members))
		changed := false
		for _, member := range v.Members {
			if member == nil || member.Kind() == Nil.Kind() {
				changed = true
				continue
			}
			normalized := NormalizeTableKey(member)
			if normalized == nil || IsNever(normalized) {
				changed = true
				continue
			}
			if normalized != member {
				changed = true
			}
			members = append(members, normalized)
		}
		if len(members) == 0 {
			return Never
		}
		if !changed {
			return t
		}
		return NewUnion(members...)
	default:
		if t.Kind() == Nil.Kind() {
			return Never
		}
		return t
	}
}
