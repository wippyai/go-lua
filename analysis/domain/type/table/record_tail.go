package table

import (
	"github.com/wippyai/go-lua/analysis/domain/type/kind"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// MapComponentKeyMayContainStaticMember reports whether a map-component key
// domain may include the exact bracket member key.
func MapComponentKeyMayContainStaticMember(key typ.Type, member typ.StaticMember) bool {
	switch member.Kind {
	case typ.StaticMemberStringIndex:
		return MapComponentKeyMayContainString(key, member.Name)
	case typ.StaticMemberIntIndex:
		return MapComponentKeyMayContainInt(key, member.Index)
	default:
		return false
	}
}

// MapComponentKeyMayContainString reports whether a map-component key domain
// may include the string key.
func MapComponentKeyMayContainString(key typ.Type, name string) bool {
	return mapComponentKeyMayContainAny(key, func(k typ.Type) bool {
		if keyDomainIsTop(k) {
			return true
		}
		if k, ok := k.(*typ.Literal); ok {
			return k.Base() == kind.String && k.Value() == name
		}
		return k.Kind() == kind.String
	})
}

// MapComponentKeyMayContainInt reports whether a map-component key domain may
// include the integer key.
func MapComponentKeyMayContainInt(key typ.Type, index int64) bool {
	return mapComponentKeyMayContainAny(key, func(k typ.Type) bool {
		if keyDomainIsTop(k) {
			return true
		}
		if k, ok := k.(*typ.Literal); ok {
			switch k.Base() {
			case kind.Integer:
				return k.Value() == index
			case kind.Number:
				number, ok := k.Value().(float64)
				return ok && number == float64(index)
			default:
				return false
			}
		}
		return k.Kind() == kind.Integer || k.Kind() == kind.Number
	})
}

func mapComponentKeyMayContainAny(key typ.Type, match func(typ.Type) bool) bool {
	return keyDomainAny(key, keyDomainTraversal{
		aliasPolicy: keyDomainAliasTarget,
	}, match)
}
