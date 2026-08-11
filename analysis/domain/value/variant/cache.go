package variant

import (
	"strconv"
	"sync"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/variant/caseset"
	"github.com/wippyai/go-lua/analysis/type/discriminant"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Cache memoizes pure variant-origin queries for one analysis database.
//
// It is intentionally caller-owned: family payloads indexed by portable
// family ID live here for exactly this check run. The cache is the semantic
// owner of reconstructed family types; no package-global catalog may retain
// type graphs beyond the caller's lifetime.
type Cache struct {
	// mu protects every mutable table in this owner. The cache may be shared by
	// analysis workers, but its state never escapes this exact analysis owner.
	mu           sync.Mutex
	detector     *discriminant.Detector
	origins      map[typ.Type]originCacheEntry
	pathLiterals map[pathLiteralCacheKey]originEvidenceCacheEntry
	narrows      map[narrowCacheKey]typeCacheEntry
	types        map[originTypeCacheKey]typeCacheEntry

	originFamilies  map[uint64]originFamily
	originRevisions map[uint64]uint64
	originPoisoned  map[uint64]struct{}
}

type originCacheEntry struct {
	family originFamily
	ok     bool
}

type originEvidenceCacheEntry struct {
	family uint64
	cases  []int
	ok     bool
}

type typeCacheEntry struct {
	t        typ.Type
	revision uint64
	ok       bool
}

type pathLiteralCacheKey struct {
	t      typ.Type
	suffix originPathKey
	lit    typ.Type
	negate bool
}

type originPathKey struct {
	count    int
	segments [4]segment.Segment
	overflow string
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
	return &Cache{detector: discriminant.NewDetector()}
}

func (c *Cache) OriginOfType(t typ.Type) (uint64, []int, bool) {
	if c == nil {
		return OriginOfType(t)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.originOfTypeLocked(t)
}

func (c *Cache) originOfTypeLocked(t typ.Type) (uint64, []int, bool) {
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
			if cached.ok && !c.originFamilyActiveLocked(cached.family.id) {
				return originFamily{}, false
			}
			return cached.family, cached.ok
		}
	}
	family, ok := originFamilyOfWithDetector(t, c.discriminantDetector())
	if !ok || !c.storeOriginFamilyLocked(family) {
		return originFamily{}, false
	}
	if c.origins == nil {
		c.origins = make(map[typ.Type]originCacheEntry)
	}
	c.origins[t] = originCacheEntry{family: family, ok: ok}
	return family, ok
}

func (c *Cache) discriminantDetector() *discriminant.Detector {
	if c.detector == nil {
		c.detector = discriminant.NewDetector()
	}
	return c.detector
}

func (c *Cache) OriginByPathLiteral(t typ.Type, suffix []segment.Segment, lit typ.Type) (uint64, []int, bool) {
	if c == nil {
		return OriginByPathLiteral(t, suffix, lit)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.originByPathLiteral(t, suffix, lit, false)
}

func (c *Cache) OriginByPathLiteralNot(t typ.Type, suffix []segment.Segment, lit typ.Type) (uint64, []int, bool) {
	if c == nil {
		return OriginByPathLiteralNot(t, suffix, lit)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.originByPathLiteral(t, suffix, lit, true)
}

func (c *Cache) originByPathLiteral(t typ.Type, suffix []segment.Segment, lit typ.Type, negate bool) (uint64, []int, bool) {
	key := pathLiteralCacheKey{
		t:      t,
		suffix: originPathLiteralKey(suffix),
		lit:    lit,
		negate: negate,
	}
	if c.pathLiterals != nil {
		if cached, ok := c.pathLiterals[key]; ok {
			if cached.ok && !c.originFamilyActiveLocked(cached.family) {
				return 0, nil, false
			}
			return cached.family, append([]int(nil), cached.cases...), cached.ok
		}
	}
	family, cases, ok := originByPathLiteralWithCache(c, t, suffix, lit, negate)
	if c.pathLiterals == nil {
		c.pathLiterals = make(map[pathLiteralCacheKey]originEvidenceCacheEntry)
	}
	c.pathLiterals[key] = originEvidenceCacheEntry{
		family: family,
		cases:  append([]int(nil), cases...),
		ok:     ok,
	}
	return family, cases, ok
}

func originPathLiteralKey(suffix []segment.Segment) originPathKey {
	if len(suffix) == 0 {
		return originPathKey{}
	}
	key := originPathKey{count: len(suffix)}
	copy(key.segments[:], suffix)
	if len(suffix) > len(key.segments) {
		key.overflow = segment.FormatSegments(suffix)
	}
	return key
}

func (c *Cache) NarrowByOrigin(t typ.Type, familyID uint64, cases caseset.View) (typ.Type, bool) {
	if c == nil {
		return NarrowByOrigin(t, familyID, cases)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.narrowByOrigin(t, familyID, cases, originCaseKey(cases))
}

func (c *Cache) narrowByOrigin(t typ.Type, familyID uint64, cases caseset.View, caseKey originCasesKey) (typ.Type, bool) {
	if familyID == 0 || cases.Len() == 0 {
		return t, false
	}
	key := narrowCacheKey{t: t, family: familyID, cases: caseKey}
	if c.narrows != nil {
		if cached, ok := c.narrows[key]; ok {
			revision, active := c.originFamilyRevisionLocked(familyID)
			if !active {
				return t, false
			}
			if cached.revision == revision {
				return cached.t, cached.ok
			}
		}
	}
	family, ok := c.originFamilyOf(t)
	if !ok || family.id != familyID {
		if c.narrows == nil {
			c.narrows = make(map[narrowCacheKey]typeCacheEntry)
		}
		revision, _ := c.originFamilyRevisionLocked(familyID)
		c.narrows[key] = typeCacheEntry{t: t, revision: revision, ok: false}
		return t, false
	}
	revision, active := c.originFamilyRevisionLocked(familyID)
	if !active {
		return t, false
	}
	narrowed, changed := narrowByOriginFamily(t, family, cases)
	if c.narrows == nil {
		c.narrows = make(map[narrowCacheKey]typeCacheEntry)
	}
	c.narrows[key] = typeCacheEntry{t: narrowed, revision: revision, ok: changed}
	return narrowed, changed
}

func (c *Cache) TypeFromOrigin(familyID uint64, cases caseset.View) (typ.Type, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.typeFromOrigin(familyID, cases, originCaseKey(cases))
}

func (c *Cache) typeFromOrigin(familyID uint64, cases caseset.View, caseKey originCasesKey) (typ.Type, bool) {
	if familyID == 0 || cases.Len() == 0 {
		return nil, false
	}
	key := originTypeCacheKey{family: familyID, cases: caseKey}
	if c.types != nil {
		if cached, ok := c.types[key]; ok {
			revision, active := c.originFamilyRevisionLocked(familyID)
			if !active {
				return nil, false
			}
			if cached.revision == revision {
				return cached.t, cached.ok
			}
		}
	}
	family, ok := c.loadOriginFamilyLocked(familyID)
	if !ok {
		if c.types == nil {
			c.types = make(map[originTypeCacheKey]typeCacheEntry)
		}
		c.types[key] = typeCacheEntry{ok: false}
		return nil, false
	}
	revision, active := c.originFamilyRevisionLocked(familyID)
	if !active {
		return nil, false
	}
	t, ok := typeFromOriginFamily(family, cases)
	if c.types == nil {
		c.types = make(map[originTypeCacheKey]typeCacheEntry)
	}
	c.types[key] = typeCacheEntry{t: t, revision: revision, ok: ok}
	return t, ok
}

func (c *Cache) originFamilyActiveLocked(id uint64) bool {
	_, ok := c.originFamilyRevisionLocked(id)
	return ok
}

func originCaseKey(cases caseset.View) originCasesKey {
	if cases.Len() == 0 {
		return originCasesKey{}
	}
	if cases.Len() <= 4 {
		key := originCasesKey{count: cases.Len()}
		for i := 0; i < cases.Len(); i++ {
			key.values[i] = cases.At(i)
		}
		return key
	}
	key := originCasesKey{count: cases.Len()}
	for i := range key.values {
		key.values[i] = cases.At(i)
	}
	buf := make([]byte, 0, cases.Len()*4)
	for i := 0; i < cases.Len(); i++ {
		buf = strconv.AppendInt(buf, int64(cases.At(i)), 10)
		buf = append(buf, ',')
	}
	key.overflow = string(buf)
	return key
}
