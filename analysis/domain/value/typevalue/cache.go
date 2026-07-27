package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/domain/value/variant/caseset"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Cache memoizes pure type-to-value projections for one analysis database.
//
// This is the local Salsa-style query surface for type-derived value evidence:
// callers own its lifetime, pass it through a check run, and discard it when
// analysis inputs change. It deliberately avoids package-global memoization.
type Cache struct {
	variants         *variant.Cache
	values           map[typeValueCacheKey]product.Value
	witnesses        map[typeValueCacheKey]product.Value
	valuesByShape    map[typeValueShapeKey][]cachedTypeValue
	witnessesByShape map[typeValueShapeKey][]cachedTypeValue
	typeProfiles     map[typeProfileCacheKey]cachedTypeProfile
	unknownTypes     map[typ.Type]cachedContainsUnknown
}

type typeValueCacheKey struct {
	reg *axis.Registry
	typ typ.Type
}

type typeValueShapeKey struct {
	reg  *axis.Registry
	hash uint64
}

type cachedTypeValue struct {
	typ   typ.Type
	value product.Value
}

type typeProfileCacheKey struct {
	reg   *axis.Registry
	value product.Value
}

type cachedTypeProfile struct {
	profile RuntimeTypeProfile
	ok      bool
}

type cachedContainsUnknown struct {
	value bool
	open  bool
}

func NewCache() *Cache {
	return &Cache{variants: variant.NewCache()}
}

func (c *Cache) IsSubtype(sub, super typ.Type) bool {
	return subtype.IsSubtype(sub, super)
}

func (c *Cache) IsFreshAssignable(sub, super typ.Type) bool {
	return subtype.IsFreshAssignable(sub, super)
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
	if cached, ok := c.cachedByShape(reg, t, c.valuesByShape); ok {
		c.rememberExactValue(&c.values, key, cached)
		return cached
	}
	value := fromType(reg, t, c)
	c.rememberTypeValue(reg, t, value, &c.values, &c.valuesByShape)
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
	if cached, ok := c.cachedByShape(reg, t, c.witnessesByShape); ok {
		c.rememberExactValue(&c.witnesses, key, cached)
		return cached
	}
	value := WithWitness(reg, c.FromType(reg, t), t)
	c.rememberTypeValue(reg, t, value, &c.witnesses, &c.witnessesByShape)
	return value
}

func (c *Cache) cachedByShape(reg *axis.Registry, t typ.Type, cache map[typeValueShapeKey][]cachedTypeValue) (product.Value, bool) {
	if len(cache) == 0 || t == nil {
		return product.Value{}, false
	}
	key := typeValueShapeKey{reg: reg, hash: typ.EqualityHash(t)}
	entries := cache[key]
	for i := range entries {
		if !cachedTypeShapeEqual(entries[i].typ, t) {
			continue
		}
		if !c.cachedProductActive(reg, entries[i].value) {
			continue
		}
		return entries[i].value, true
	}
	return product.Value{}, false
}

func (c *Cache) rememberTypeValue(
	reg *axis.Registry,
	t typ.Type,
	value product.Value,
	exact *map[typeValueCacheKey]product.Value,
	shape *map[typeValueShapeKey][]cachedTypeValue,
) {
	c.rememberExactValue(exact, typeValueCacheKey{reg: reg, typ: t}, value)
	if t == nil {
		return
	}
	key := typeValueShapeKey{reg: reg, hash: typ.EqualityHash(t)}
	if *shape == nil {
		*shape = make(map[typeValueShapeKey][]cachedTypeValue)
	}
	entries := (*shape)[key]
	for i := range entries {
		if cachedTypeShapeEqual(entries[i].typ, t) {
			entries[i].typ = t
			entries[i].value = value
			(*shape)[key] = entries
			return
		}
	}
	(*shape)[key] = append(entries, cachedTypeValue{typ: t, value: value})
}

func cachedTypeShapeEqual(a, b typ.Type) bool {
	if typ.SameNodeOrAcyclicEqual(a, b) {
		return true
	}
	return typ.TypeEquals(a, b)
}

func (c *Cache) rememberExactValue(cache *map[typeValueCacheKey]product.Value, key typeValueCacheKey, value product.Value) {
	if *cache == nil {
		*cache = make(map[typeValueCacheKey]product.Value)
	}
	(*cache)[key] = value
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

// OriginOfType returns finite variant-origin evidence for t, using the cache's
// variant catalog when present.
func (c *Cache) OriginOfType(t typ.Type) (uint64, []int, bool) {
	return c.originOfType(t)
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

// NarrowVariantByOriginView narrows using an immutable canonical case view.
func (c *Cache) NarrowVariantByOriginView(t typ.Type, family uint64, cases caseset.View) (typ.Type, bool) {
	if c == nil {
		return variant.NarrowByOriginView(t, family, cases)
	}
	if c.variants == nil {
		c.variants = variant.NewCache()
	}
	return c.variants.NarrowByOriginView(t, family, cases)
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

// TypeFromVariantOriginView reconstructs using an immutable canonical case view.
func (c *Cache) TypeFromVariantOriginView(family uint64, cases caseset.View) (typ.Type, bool) {
	if c == nil {
		return variant.TypeFromOriginView(family, cases)
	}
	if c.variants == nil {
		c.variants = variant.NewCache()
	}
	return c.variants.TypeFromOriginView(family, cases)
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
	_, ok := c.TypeFromVariantOriginView(origin.Family(), origin.CasesView())
	return ok
}
