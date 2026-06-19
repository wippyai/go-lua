package variant

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/variant/internal/discriminant"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Cache memoizes pure variant-origin queries for one analysis database.
//
// It is intentionally caller-owned: package-level origin catalogs preserve
// reconstructed family payloads by family id, while this cache avoids repeating
// the expensive "does this type form a variant family?" proof for the same
// immutable type node inside a check run.
type Cache struct {
	detector     *discriminant.Detector
	origins      map[typ.Type]originCacheEntry
	pathLiterals map[pathLiteralCacheKey]originEvidenceCacheEntry
	narrows      map[narrowCacheKey]typeCacheEntry
	types        map[originTypeCacheKey]typeCacheEntry
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
			if cached.ok && !originFamilyActive(cached.family.id) {
				return originFamily{}, false
			}
			return cached.family, cached.ok
		}
	}
	family, ok := originFamilyOfWithDetector(t, c.discriminantDetector())
	if !ok {
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
	return c.originByPathLiteral(t, suffix, lit, false)
}

func (c *Cache) OriginByPathLiteralNot(t typ.Type, suffix []segment.Segment, lit typ.Type) (uint64, []int, bool) {
	if c == nil {
		return OriginByPathLiteralNot(t, suffix, lit)
	}
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
			if cached.ok && !originFamilyActive(cached.family) {
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
			revision, active := originFamilyRevision(familyID)
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
		revision, _ := originFamilyRevision(familyID)
		c.narrows[key] = typeCacheEntry{t: t, revision: revision, ok: false}
		return t, false
	}
	revision, active := originFamilyRevision(familyID)
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
			revision, active := originFamilyRevision(familyID)
			if !active {
				return nil, false
			}
			if cached.revision == revision {
				return cached.t, cached.ok
			}
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
	revision, active := originFamilyRevision(familyID)
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

func originFamilyActive(id uint64) bool {
	_, ok := originFamilyRevision(id)
	return ok
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
