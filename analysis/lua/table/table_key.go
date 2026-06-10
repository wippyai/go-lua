package table

import "github.com/wippyai/go-lua/analysis/type/typ"

// NormalizeKey removes nil alternatives from table key domains.
func NormalizeKey(t typ.Type) typ.Type {
	if t == nil {
		return typ.Unknown
	}
	switch v := t.(type) {
	case *typ.Annotated:
		inner := NormalizeKey(v.Inner)
		if inner == v.Inner {
			return t
		}
		return typ.NewAnnotated(inner, v.Annotations)
	case *typ.Alias:
		inner := NormalizeKey(v.Target)
		if inner == nil || typ.IsNever(inner) {
			return inner
		}
		if inner == v.Target {
			return t
		}
		return typ.NewAlias(v.Name, inner)
	case *typ.Optional:
		return NormalizeKey(v.Inner)
	case *typ.Union:
		members := make([]typ.Type, 0, len(v.Members))
		changed := false
		for _, member := range v.Members {
			if member == nil || member.Kind() == typ.Nil.Kind() {
				changed = true
				continue
			}
			normalized := NormalizeKey(member)
			if normalized == nil || typ.IsNever(normalized) {
				changed = true
				continue
			}
			if normalized != member {
				changed = true
			}
			members = append(members, normalized)
		}
		if len(members) == 0 {
			return typ.Never
		}
		if !changed {
			return t
		}
		return typ.NewUnion(members...)
	default:
		if t.Kind() == typ.Nil.Kind() {
			return typ.Never
		}
		return t
	}
}
