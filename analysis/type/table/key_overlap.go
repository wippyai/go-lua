package table

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// MapComponentKeyMayOverlapType reports whether at least one runtime key
// represented by key may be admitted by the map-component key domain. Unlike
// MapComponentKeyAdmitsType, this is an overlap predicate for reads whose
// result remains nilable when only some represented keys can hit the map.
func MapComponentKeyMayOverlapType(keyDomain typ.Type, key typ.Type) bool {
	keyDomain = unwrap.Annotated(keyDomain)
	key = unwrap.Annotated(key)
	if keyDomain == nil || key == nil {
		return false
	}
	if keyDomainIsTop(key) {
		return true
	}

	switch v := key.(type) {
	case *typ.Literal:
		return mapComponentKeyMayOverlapLiteral(keyDomain, v)
	case *typ.Union:
		for _, member := range v.Members {
			if MapComponentKeyMayOverlapType(keyDomain, member) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		if len(v.Members) == 0 {
			return false
		}
		for _, member := range v.Members {
			if !MapComponentKeyMayOverlapType(keyDomain, member) {
				return false
			}
		}
		return true
	case *typ.Alias:
		return MapComponentKeyMayOverlapType(keyDomain, v.UnaliasedTarget())
	}

	switch key.Kind() {
	case kind.Never:
		return false
	case kind.String:
		return mapComponentKeyMayOverlapStringType(keyDomain)
	case kind.Integer:
		return mapComponentKeyMayOverlapIntegerType(keyDomain)
	case kind.Number:
		return mapComponentKeyMayOverlapNumberType(keyDomain)
	case kind.Boolean:
		return mapComponentKeyMayOverlapBooleanType(keyDomain)
	default:
		return false
	}
}

func mapComponentKeyMayOverlapLiteral(keyDomain typ.Type, lit *typ.Literal) bool {
	if lit == nil {
		return false
	}
	switch lit.Base() {
	case kind.String:
		name, ok := lit.Value().(string)
		return ok && MapComponentKeyMayContainString(keyDomain, name)
	case kind.Integer:
		index, ok := lit.Value().(int64)
		return ok && MapComponentKeyMayContainInt(keyDomain, index)
	case kind.Number:
		value, ok := lit.Value().(float64)
		return ok && mapComponentKeyMayOverlapNumberLiteral(keyDomain, value)
	case kind.Boolean:
		value, ok := lit.Value().(bool)
		return ok && mapComponentKeyMayOverlapBooleanLiteral(keyDomain, value)
	default:
		return false
	}
}

func mapComponentKeyMayOverlapStringType(keyDomain typ.Type) bool {
	return mapComponentKeyMayOverlapBaseType(
		keyDomain,
		func(lit *typ.Literal) bool { return lit.Base() == kind.String },
		func(k kind.Kind) bool { return k == kind.String },
	)
}

func mapComponentKeyMayOverlapIntegerType(keyDomain typ.Type) bool {
	return mapComponentKeyMayOverlapBaseType(
		keyDomain,
		func(lit *typ.Literal) bool {
			switch lit.Base() {
			case kind.Integer:
				return true
			case kind.Number:
				value, ok := lit.Value().(float64)
				return ok && value == float64(int64(value))
			default:
				return false
			}
		},
		func(k kind.Kind) bool { return k == kind.Integer || k == kind.Number },
	)
}

func mapComponentKeyMayOverlapNumberType(keyDomain typ.Type) bool {
	return mapComponentKeyMayOverlapBaseType(
		keyDomain,
		func(lit *typ.Literal) bool { return lit.Base() == kind.Integer || lit.Base() == kind.Number },
		func(k kind.Kind) bool { return k == kind.Integer || k == kind.Number },
	)
}

func mapComponentKeyMayOverlapBooleanType(keyDomain typ.Type) bool {
	return mapComponentKeyMayOverlapBaseType(
		keyDomain,
		func(lit *typ.Literal) bool { return lit.Base() == kind.Boolean },
		func(k kind.Kind) bool { return k == kind.Boolean },
	)
}

func mapComponentKeyMayOverlapBaseType(keyDomain typ.Type, literalMatch func(*typ.Literal) bool, kindMatch func(kind.Kind) bool) bool {
	return keyDomainMayOverlapAny(keyDomain, func(k typ.Type) bool {
		if keyDomainIsTop(k) {
			return true
		}
		if lit, ok := k.(*typ.Literal); ok {
			return literalMatch != nil && literalMatch(lit)
		}
		return kindMatch != nil && kindMatch(k.Kind())
	})
}

func mapComponentKeyMayOverlapNumberLiteral(keyDomain typ.Type, value float64) bool {
	isInteger := value == float64(int64(value))
	return keyDomainMayOverlapAny(keyDomain, func(k typ.Type) bool {
		if keyDomainIsTop(k) {
			return true
		}
		if lit, ok := k.(*typ.Literal); ok {
			switch lit.Base() {
			case kind.Integer:
				other, ok := lit.Value().(int64)
				return ok && isInteger && other == int64(value)
			case kind.Number:
				other, ok := lit.Value().(float64)
				return ok && other == value
			default:
				return false
			}
		}
		return k.Kind() == kind.Number || (isInteger && k.Kind() == kind.Integer)
	})
}

func mapComponentKeyMayOverlapBooleanLiteral(keyDomain typ.Type, value bool) bool {
	return keyDomainMayOverlapAny(keyDomain, func(k typ.Type) bool {
		if keyDomainIsTop(k) {
			return true
		}
		if lit, ok := k.(*typ.Literal); ok {
			other, ok := lit.Value().(bool)
			return lit.Base() == kind.Boolean && ok && other == value
		}
		return k.Kind() == kind.Boolean
	})
}

func keyDomainMayOverlapAny(keyDomain typ.Type, match func(typ.Type) bool) bool {
	return keyDomainAny(keyDomain, keyDomainTraversal{
		unwrapAnnotated: true,
		aliasPolicy:     keyDomainAliasUnaliasedTarget,
	}, match)
}
