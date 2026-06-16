package variant

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Cache memoizes pure variant-origin queries for one analysis database.
//
// It is intentionally caller-owned: package-level origin catalogs preserve
// reconstructed family payloads by family id, while this cache avoids repeating
// the expensive "does this type form a variant family?" proof for the same
// immutable type node inside a check run.
type Cache struct {
	origins map[typ.Type]originCacheEntry
	narrows map[narrowCacheKey]typeCacheEntry
	types   map[originTypeCacheKey]typeCacheEntry
}

type originCacheEntry struct {
	family originFamily
	ok     bool
}

type typeCacheEntry struct {
	t  typ.Type
	ok bool
}

type narrowCacheKey struct {
	t      typ.Type
	family uint64
	cases  originCasesKey
}

type originTypeCacheKey struct {
	family uint64
	cases  originCasesKey
}

type originCasesKey struct {
	count    int
	values   [4]int
	overflow string
}

func NewCache() *Cache {
	return &Cache{}
}

func (c *Cache) OriginOfType(t typ.Type) (uint64, []int, bool) {
	if c == nil {
		return OriginOfType(t)
	}
	family, ok := c.originFamilyOf(t)
	if !ok {
		return 0, nil, false
	}
	return family.id, originFamilyCases(family), true
}

func (c *Cache) originFamilyOf(t typ.Type) (originFamily, bool) {
	if t == nil {
		return originFamily{}, false
	}
	if c.origins != nil {
		if cached, ok := c.origins[t]; ok {
			return cached.family, cached.ok
		}
	}
	family, ok := originFamilyOf(t)
	if c.origins == nil {
		c.origins = make(map[typ.Type]originCacheEntry)
	}
	c.origins[t] = originCacheEntry{family: family, ok: ok}
	return family, ok
}

func (c *Cache) NarrowByOrigin(t typ.Type, familyID uint64, cases []int) (typ.Type, bool) {
	if c == nil {
		return NarrowByOrigin(t, familyID, cases)
	}
	if familyID == 0 || len(cases) == 0 {
		return t, false
	}
	key := narrowCacheKey{t: t, family: familyID, cases: originCaseKey(cases)}
	if c.narrows != nil {
		if cached, ok := c.narrows[key]; ok {
			return cached.t, cached.ok
		}
	}
	family, ok := c.originFamilyOf(t)
	if !ok || family.id != familyID {
		if c.narrows == nil {
			c.narrows = make(map[narrowCacheKey]typeCacheEntry)
		}
		c.narrows[key] = typeCacheEntry{t: t, ok: false}
		return t, false
	}
	narrowed, changed := narrowByOriginFamily(t, family, cases)
	if c.narrows == nil {
		c.narrows = make(map[narrowCacheKey]typeCacheEntry)
	}
	c.narrows[key] = typeCacheEntry{t: narrowed, ok: changed}
	return narrowed, changed
}

func (c *Cache) TypeFromOrigin(familyID uint64, cases []int) (typ.Type, bool) {
	if c == nil {
		return TypeFromOrigin(familyID, cases)
	}
	if familyID == 0 || len(cases) == 0 {
		return nil, false
	}
	key := originTypeCacheKey{family: familyID, cases: originCaseKey(cases)}
	if c.types != nil {
		if cached, ok := c.types[key]; ok {
			return cached.t, cached.ok
		}
	}
	family, ok := loadOriginFamily(familyID)
	if !ok {
		if c.types == nil {
			c.types = make(map[originTypeCacheKey]typeCacheEntry)
		}
		c.types[key] = typeCacheEntry{ok: false}
		return nil, false
	}
	t, ok := typeFromOriginFamily(family, cases)
	if c.types == nil {
		c.types = make(map[originTypeCacheKey]typeCacheEntry)
	}
	c.types[key] = typeCacheEntry{t: t, ok: ok}
	return t, ok
}

func originCaseKey(cases []int) originCasesKey {
	if len(cases) == 0 {
		return originCasesKey{}
	}
	var key originCasesKey
	for _, c := range cases {
		if !insertOriginCase(&key, c) {
			compact := compactInts(append([]int(nil), cases...))
			return originCaseOverflowKey(compact)
		}
	}
	return key
}

func insertOriginCase(key *originCasesKey, value int) bool {
	i := 0
	for i < key.count && key.values[i] < value {
		i++
	}
	if i < key.count && key.values[i] == value {
		return true
	}
	if key.count == len(key.values) {
		return false
	}
	copy(key.values[i+1:], key.values[i:key.count])
	key.values[i] = value
	key.count++
	return true
}

func originCaseOverflowKey(compact []int) originCasesKey {
	key := originCasesKey{count: len(compact)}
	copy(key.values[:], compact)
	if len(compact) <= len(key.values) {
		return key
	}
	buf := make([]byte, 0, len(compact)*4)
	for _, c := range compact {
		buf = strconv.AppendInt(buf, int64(c), 10)
		buf = append(buf, ',')
	}
	key.overflow = string(buf)
	return key
}
