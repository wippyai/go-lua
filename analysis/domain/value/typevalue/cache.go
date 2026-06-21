package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Cache memoizes pure type-to-value projections for one analysis database.
//
// This is the local Salsa-style query surface for type-derived value evidence:
// callers own its lifetime, pass it through a check run, and discard it when
// analysis inputs change. It deliberately avoids package-global memoization.
type Cache struct {
	variants  *variant.Cache
	values    map[typeValueCacheKey]product.Value
	witnesses map[typeValueCacheKey]product.Value
}

type typeValueCacheKey struct {
	reg *axis.Registry
	typ typ.Type
}

func NewCache() *Cache {
	return &Cache{variants: variant.NewCache()}
}

func (c *Cache) FromType(reg *axis.Registry, t typ.Type) product.Value {
	if c == nil {
		return FromType(reg, t)
	}
	key := typeValueCacheKey{reg: reg, typ: t}
	if c.values != nil {
		if cached, ok := c.values[key]; ok {
			if c.cachedProductActive(reg, cached) {
				return cached
			}
			delete(c.values, key)
		}
	}
	value := fromType(reg, t, c)
	if c.values == nil {
		c.values = make(map[typeValueCacheKey]product.Value)
	}
	c.values[key] = value
	return value
}

func (c *Cache) FromTypeWithWitness(reg *axis.Registry, t typ.Type) product.Value {
	if c == nil {
		return WithWitness(reg, FromType(reg, t), t)
	}
	key := typeValueCacheKey{reg: reg, typ: t}
	if c.witnesses != nil {
		if cached, ok := c.witnesses[key]; ok {
			if c.cachedProductActive(reg, cached) {
				return cached
			}
			delete(c.witnesses, key)
		}
	}
	value := WithWitness(reg, c.FromType(reg, t), t)
	if c.witnesses == nil {
		c.witnesses = make(map[typeValueCacheKey]product.Value)
	}
	c.witnesses[key] = value
	return value
}

func (c *Cache) originOfType(t typ.Type) (uint64, []int, bool) {
	if c == nil {
		return variant.OriginOfType(t)
	}
	if c.variants == nil {
		c.variants = variant.NewCache()
	}
	return c.variants.OriginOfType(t)
}

func (c *Cache) NarrowVariantByOrigin(t typ.Type, family uint64, cases []int) (typ.Type, bool) {
	if c == nil {
		return variant.NarrowByOrigin(t, family, cases)
	}
	if c.variants == nil {
		c.variants = variant.NewCache()
	}
	return c.variants.NarrowByOrigin(t, family, cases)
}

func (c *Cache) TypeFromVariantOrigin(family uint64, cases []int) (typ.Type, bool) {
	if c == nil {
		return variant.TypeFromOrigin(family, cases)
	}
	if c.variants == nil {
		c.variants = variant.NewCache()
	}
	return c.variants.TypeFromOrigin(family, cases)
}

func (c *Cache) OriginByPathLiteral(t typ.Type, suffix []segment.Segment, lit typ.Type) (uint64, []int, bool) {
	if c == nil {
		return variant.OriginByPathLiteral(t, suffix, lit)
	}
	if c.variants == nil {
		c.variants = variant.NewCache()
	}
	return c.variants.OriginByPathLiteral(t, suffix, lit)
}

func (c *Cache) OriginByPathLiteralNot(t typ.Type, suffix []segment.Segment, lit typ.Type) (uint64, []int, bool) {
	if c == nil {
		return variant.OriginByPathLiteralNot(t, suffix, lit)
	}
	if c.variants == nil {
		c.variants = variant.NewCache()
	}
	return c.variants.OriginByPathLiteralNot(t, suffix, lit)
}

func (c *Cache) cachedProductActive(reg *axis.Registry, value product.Value) bool {
	if reg == nil {
		return true
	}
	if _, ok := reg.LookupErased(variantorigin.Key.ID()); !ok {
		return true
	}
	origin := product.Get(reg, value, variantorigin.Key)
	if origin.IsBottom() || origin.IsTop() {
		return true
	}
	_, ok := c.TypeFromVariantOrigin(origin.Family(), origin.CasesRef())
	return ok
}
