package table

import (
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/unwrap"
)

type keyDomainAliasPolicy uint8

const (
	keyDomainAliasTarget keyDomainAliasPolicy = iota
	keyDomainAliasUnaliasedTarget
)

type keyDomainIntersectionPolicy uint8

const (
	keyDomainIntersectionOpaque keyDomainIntersectionPolicy = iota
	keyDomainIntersectionAll
)

type keyDomainTraversal struct {
	unwrapAnnotated bool
	aliasPolicy     keyDomainAliasPolicy
	intersections   keyDomainIntersectionPolicy
}

// keyDomainAny walks the structural key-domain wrappers shared by exact tail
// containment and static-read admission, then applies match to leaf domains.
func keyDomainAny(t typ.Type, traversal keyDomainTraversal, match func(typ.Type) bool) bool {
	if traversal.unwrapAnnotated {
		t = unwrap.Annotated(t)
	}

	switch v := t.(type) {
	case nil:
		return false
	case *typ.Alias:
		return keyDomainAny(keyDomainAliasTargetFor(v, traversal.aliasPolicy), traversal, match)
	case *typ.Union:
		if len(v.Members) == 0 {
			return false
		}
		for _, member := range v.Members {
			if keyDomainAny(member, traversal, match) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		if traversal.intersections != keyDomainIntersectionAll {
			return match(v)
		}
		for _, member := range v.Members {
			if !keyDomainAny(member, traversal, match) {
				return false
			}
		}
		return true
	default:
		return match(v)
	}
}

func keyDomainAliasTargetFor(alias *typ.Alias, policy keyDomainAliasPolicy) typ.Type {
	if alias == nil {
		return nil
	}
	if policy == keyDomainAliasUnaliasedTarget {
		return alias.UnaliasedTarget()
	}
	return alias.Target
}

func keyDomainIsTop(t typ.Type) bool {
	return typ.IsAny(t) || typ.IsUnknown(t)
}
