package table

import (
	"github.com/wippyai/go-lua/analysis/domain/type/kind"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/unwrap"
)

// MapComponentKeyAdmitsType reports whether every key represented by key may
// be admitted by the map-component key domain under static read semantics.
func MapComponentKeyAdmitsType(keyDomain typ.Type, key typ.Type) bool {
	keyDomain = unwrap.Annotated(keyDomain)
	key = unwrap.Annotated(key)
	if keyDomain == nil || key == nil {
		return false
	}

	switch v := key.(type) {
	case *typ.Literal:
		return mapComponentKeyAdmitsLiteral(keyDomain, v)
	case *typ.Union:
		if len(v.Members) == 0 {
			return false
		}
		for _, member := range v.Members {
			if !MapComponentKeyAdmitsType(keyDomain, member) {
				return false
			}
		}
		return true
	case *typ.Intersection:
		if len(v.Members) == 0 {
			return true
		}
		for _, member := range v.Members {
			if !MapComponentKeyAdmitsType(keyDomain, member) {
				return false
			}
		}
		return true
	case *typ.Alias:
		return MapComponentKeyAdmitsType(keyDomain, v.UnaliasedTarget())
	}

	switch key.Kind() {
	case kind.Any, kind.Unknown:
		return keyDomainIsTop(keyDomain)
	case kind.Never:
		return true
	case kind.String:
		return mapComponentKeyAdmitsStringType(keyDomain)
	case kind.Integer:
		return mapComponentKeyAdmitsIntType(keyDomain)
	case kind.Number:
		return mapComponentKeyAdmitsNumberType(keyDomain)
	case kind.Boolean:
		return mapComponentKeyAdmitsBooleanType(keyDomain)
	default:
		return false
	}
}

func mapComponentKeyAdmitsLiteral(keyDomain typ.Type, lit *typ.Literal) bool {
	if lit == nil {
		return false
	}
	switch lit.Base() {
	case kind.String:
		name, ok := lit.Value().(string)
		return ok && mapComponentKeyAdmitsStringLiteral(keyDomain, name)
	case kind.Integer:
		index, ok := lit.Value().(int64)
		return ok && mapComponentKeyAdmitsIntLiteral(keyDomain, index)
	case kind.Number:
		return mapComponentKeyAdmitsNumberLiteral(keyDomain, lit)
	case kind.Boolean:
		value, ok := lit.Value().(bool)
		return ok && mapComponentKeyAdmitsBooleanLiteral(keyDomain, value)
	default:
		return false
	}
}

func mapComponentKeyAdmitsStringType(keyDomain typ.Type) bool {
	return mapComponentKeyDomainAdmitsAny(keyDomain, mapComponentKeyAdmitsStringDomain)
}

func mapComponentKeyAdmitsIntType(keyDomain typ.Type) bool {
	return mapComponentKeyDomainAdmitsAny(keyDomain, mapComponentKeyAdmitsIntegerDomain)
}

func mapComponentKeyAdmitsNumberType(keyDomain typ.Type) bool {
	return mapComponentKeyDomainAdmitsAny(keyDomain, mapComponentKeyAdmitsNumberDomain)
}

func mapComponentKeyAdmitsBooleanType(keyDomain typ.Type) bool {
	return mapComponentKeyDomainAdmitsAny(keyDomain, mapComponentKeyAdmitsBooleanDomain)
}

func mapComponentKeyAdmitsStringLiteral(keyDomain typ.Type, name string) bool {
	return mapComponentKeyAdmitsTypedLiteral(keyDomain, kind.String, name, mapComponentKeyAdmitsStringDomain)
}

// mapComponentKeyAdmitsTypedLiteral reports whether the key domain admits the
// literal want of base kind base: a matching literal key, or a non-literal key
// of that base kind (or any/unknown).
func mapComponentKeyAdmitsTypedLiteral[T comparable](keyDomain typ.Type, base kind.Kind, want T, admitsDomain func(typ.Type) bool) bool {
	return mapComponentKeyDomainAdmitsAny(keyDomain, func(k typ.Type) bool {
		switch lit := k.(type) {
		case *typ.Literal:
			other, ok := lit.Value().(T)
			return lit.Base() == base && ok && other == want
		default:
			return admitsDomain != nil && admitsDomain(k)
		}
	})
}

func mapComponentKeyAdmitsIntLiteral(keyDomain typ.Type, index int64) bool {
	return mapComponentKeyAdmitsTypedLiteral(keyDomain, kind.Integer, index, mapComponentKeyAdmitsIntegerDomain)
}

func mapComponentKeyAdmitsNumberLiteral(keyDomain typ.Type, lit *typ.Literal) bool {
	value, ok := lit.Value().(float64)
	if !ok {
		return false
	}
	return mapComponentKeyAdmitsTypedLiteral(keyDomain, kind.Number, value, mapComponentKeyAdmitsNumberDomain)
}

func mapComponentKeyAdmitsBooleanLiteral(keyDomain typ.Type, value bool) bool {
	return mapComponentKeyAdmitsTypedLiteral(keyDomain, kind.Boolean, value, mapComponentKeyAdmitsBooleanDomain)
}

func mapComponentKeyDomainAdmitsAny(keyDomain typ.Type, match func(typ.Type) bool) bool {
	return keyDomainAny(keyDomain, keyDomainTraversal{
		unwrapAnnotated: true,
		aliasPolicy:     keyDomainAliasUnaliasedTarget,
		intersections:   keyDomainIntersectionAll,
	}, match)
}

func mapComponentKeyAdmitsStringDomain(t typ.Type) bool {
	return t.Kind() == kind.String || keyDomainIsTop(t)
}

func mapComponentKeyAdmitsIntegerDomain(t typ.Type) bool {
	return t.Kind() == kind.Integer || t.Kind() == kind.Number || keyDomainIsTop(t)
}

func mapComponentKeyAdmitsNumberDomain(t typ.Type) bool {
	return t.Kind() == kind.Number || keyDomainIsTop(t)
}

func mapComponentKeyAdmitsBooleanDomain(t typ.Type) bool {
	return t.Kind() == kind.Boolean || keyDomainIsTop(t)
}
