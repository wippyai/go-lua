package subtype

import "github.com/wippyai/go-lua/analysis/type/typ"

// Cache memoizes pure subtype queries for one analysis run.
//
// The cache is intentionally scoped by its owner. It never stores recursive
// pairs because recursive placeholders can receive bodies during construction;
// those remain handled by the checker's per-query coinductive memo.
type Cache struct {
	subtypes        map[typePair]bool
	freshAssignable map[typePair]bool
}

func NewCache() *Cache {
	return &Cache{}
}

func (c *Cache) IsSubtype(sub, super typ.Type) bool {
	if c == nil {
		return IsSubtype(sub, super)
	}
	pair, cacheable := cacheableTypePair(sub, super)
	if cacheable && c.subtypes != nil {
		if result, ok := c.subtypes[pair]; ok {
			return result
		}
	}
	result := (&checker{}).check(sub, super, 0)
	if cacheable {
		if c.subtypes == nil {
			c.subtypes = make(map[typePair]bool)
		}
		c.subtypes[pair] = result
	}
	return result
}

func (c *Cache) IsFreshAssignable(sub, super typ.Type) bool {
	if c == nil {
		return IsFreshAssignable(sub, super)
	}
	pair, cacheable := cacheableTypePair(sub, super)
	if cacheable && c.freshAssignable != nil {
		if result, ok := c.freshAssignable[pair]; ok {
			return result
		}
	}
	result := (&checker{}).check(sub, super, 0)
	if !result {
		result = (&checker{}).canWidenTo(sub, super, 0)
	}
	if cacheable {
		if c.freshAssignable == nil {
			c.freshAssignable = make(map[typePair]bool)
		}
		c.freshAssignable[pair] = result
	}
	return result
}

func cacheableTypePair(sub, super typ.Type) (typePair, bool) {
	if typ.ContainsRecursive(sub) || typ.ContainsRecursive(super) {
		return typePair{}, false
	}
	return newTypePair(sub, super)
}
