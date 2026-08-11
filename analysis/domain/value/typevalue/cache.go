package typevalue

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Cache memoizes pure type-to-value projections for one analysis database.
//
// This is the local Salsa-style query surface for type-derived value evidence:
// callers own its lifetime, pass it through a check run, and discard it when
// analysis inputs change. It deliberately avoids package-global memoization.
type Cache struct {
	// mu owns every memo table and the lazy variant plane below. Public query
	// methods take this lock once; their private *Locked helpers never re-enter
	// it while composing another cached query.
	mu               sync.Mutex
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
	return &Cache{}
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
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.fromTypeLocked(reg, t)
}

func (c *Cache) fromTypeLocked(reg *axis.Registry, t typ.Type) product.Value {
	key := typeValueCacheKey{reg: reg, typ: t}
	if c.values != nil {
		if cached, ok := c.values[key]; ok {
			if c.cachedProductActiveLocked(reg, cached) {
				return cached
			}
			delete(c.values, key)
		}
	}
	if cached, ok := c.cachedByShapeLocked(reg, t, c.valuesByShape); ok {
		c.rememberExactValueLocked(&c.values, key, cached)
		return cached
	}
	value := fromType(reg, t, c)
	c.rememberTypeValueLocked(reg, t, value, &c.values, &c.valuesByShape)
	return value
}

func (c *Cache) FromTypeWithWitness(reg *axis.Registry, t typ.Type) product.Value {
	if c == nil {
		return WithWitness(reg, FromType(reg, t), t)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.fromTypeWithWitnessLocked(reg, t)
}

func (c *Cache) fromTypeWithWitnessLocked(reg *axis.Registry, t typ.Type) product.Value {
	key := typeValueCacheKey{reg: reg, typ: t}
	if c.witnesses != nil {
		if cached, ok := c.witnesses[key]; ok {
			if c.cachedProductActiveLocked(reg, cached) {
				return cached
			}
			delete(c.witnesses, key)
		}
	}
	if cached, ok := c.cachedByShapeLocked(reg, t, c.witnessesByShape); ok {
		c.rememberExactValueLocked(&c.witnesses, key, cached)
		return cached
	}
	value := WithWitness(reg, c.fromTypeLocked(reg, t), t)
	c.rememberTypeValueLocked(reg, t, value, &c.witnesses, &c.witnessesByShape)
	return value
}

func (c *Cache) cachedByShapeLocked(reg *axis.Registry, t typ.Type, cache map[typeValueShapeKey][]cachedTypeValue) (product.Value, bool) {
	if len(cache) == 0 || t == nil {
		return product.Value{}, false
	}
	key := typeValueShapeKey{reg: reg, hash: typ.EqualityHash(t)}
	entries := cache[key]
	for i := range entries {
		if !cachedTypeShapeEqual(entries[i].typ, t) {
			continue
		}
		if !c.cachedProductActiveLocked(reg, entries[i].value) {
			continue
		}
		return entries[i].value, true
	}
	return product.Value{}, false
}

func (c *Cache) rememberTypeValueLocked(
	reg *axis.Registry,
	t typ.Type,
	value product.Value,
	exact *map[typeValueCacheKey]product.Value,
	shape *map[typeValueShapeKey][]cachedTypeValue,
) {
	c.rememberExactValueLocked(exact, typeValueCacheKey{reg: reg, typ: t}, value)
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

func (c *Cache) rememberExactValueLocked(cache *map[typeValueCacheKey]product.Value, key typeValueCacheKey, value product.Value) {
	if *cache == nil {
		*cache = make(map[typeValueCacheKey]product.Value)
	}
	(*cache)[key] = value
}

// Variants returns the cache's variant query plane. A nil Cache deliberately
// returns a nil variant cache, whose methods provide the uncached default.
func (c *Cache) Variants() *variant.Cache {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.variantsLocked()
}

func (c *Cache) variantsLocked() *variant.Cache {
	if c == nil {
		return nil
	}
	if c.variants == nil {
		c.variants = variant.NewCache()
	}
	return c.variants
}

func (c *Cache) cachedProductActiveLocked(reg *axis.Registry, value product.Value) bool {
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
	_, ok := c.variantsLocked().TypeFromOrigin(origin.Family(), origin.CasesView())
	return ok
}
