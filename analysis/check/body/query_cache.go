package body

import (
	"github.com/wippyai/go-lua/analysis/check/body/internal/readexpr"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// resultQueryCache owns memoized derived queries for one solved Result. The
// keys include the read mode and CFG point where needed, so cached values do not
// cross semantic boundaries such as before-node vs boundary-state reads.
type resultQueryCache struct {
	sourceValues        cachedProductValueCache[sourceValueCacheKey]
	pathValues          cachedProductValueCache[pathValueCacheKey]
	callOutcomes        map[cfg.Point]callpayload.CallOutcome
	edgeNormal          map[edgeNormalCacheKey]bool
	normalReachable     map[cfg.Point]bool
	normalReachableSet  bool
	memberReadSources   []dominatingMemberReadPresenceSource
	memberReadSourcesOK bool
	memberReadPresence  map[dominatingMemberReadPresenceKey]bool
	signatureTypes      map[string]cachedSignatureType
	reachability        *cfg.Reachability
	immediateDominators map[cfg.Point]cfg.Point
	readContexts        [sourceValueReadModeCount]readexpr.Context
	sourceResolvers     [sourceValueReadModeCount]sourcevalue.SourceValues
	callOutcomeCapacity int
}

const resultQueryInline = 4

func newResultQueryCache(facts factflow.Facts) resultQueryCache {
	return resultQueryCache{callOutcomeCapacity: facts.CallSiteCount()}
}

func (c *resultQueryCache) reset() {
	if c == nil {
		return
	}
	c.sourceValues.reset()
	c.pathValues.reset()
	c.callOutcomes = nil
	c.edgeNormal = nil
	c.normalReachable = nil
	c.normalReachableSet = false
	c.memberReadSources = nil
	c.memberReadSourcesOK = false
	c.memberReadPresence = nil
	c.signatureTypes = nil
	c.reachability = nil
	c.immediateDominators = nil
	c.readContexts = [sourceValueReadModeCount]readexpr.Context{}
	c.sourceResolvers = [sourceValueReadModeCount]sourcevalue.SourceValues{}
}

type sourceValueReadMode uint8

const (
	sourceValueReadBoundary sourceValueReadMode = iota
	sourceValueReadBeforeBoundary
	sourceValueReadExplanationBoundary
	sourceValueReadModeCount
)

type sourceValueCacheKey struct {
	mode   sourceValueReadMode
	point  cfg.Point
	source factflow.ValueSource
}

type pathValueCacheKey struct {
	mode  sourceValueReadMode
	point cfg.Point
	path  keyspace.PathIdentity
}

func newPathValueCacheKey(ks *keyspace.KeySpace, mode sourceValueReadMode, point cfg.Point, p pathdom.Path) (pathValueCacheKey, bool) {
	pathID, ok := keyspace.PathIdentityFromPath(ks, p)
	if !ok {
		return pathValueCacheKey{}, false
	}
	return pathValueCacheKey{mode: mode, point: point, path: pathID}, true
}

type dominatingMemberReadPresenceKey struct {
	point cfg.Point
	path  keyspace.PathIdentity
}

func newDominatingMemberReadPresenceKey(ks *keyspace.KeySpace, point cfg.Point, p pathdom.Path) (dominatingMemberReadPresenceKey, bool) {
	pathID, ok := keyspace.PathIdentityFromPath(ks, p)
	if !ok {
		return dominatingMemberReadPresenceKey{}, false
	}
	return dominatingMemberReadPresenceKey{point: point, path: pathID}, true
}

type cachedProductValue struct {
	value product.Value
	ok    bool
}

type cachedProductValueCache[K comparable] struct {
	values    map[K]cachedProductValue
	inline    [resultQueryInline]cachedProductValueEntry[K]
	inlineLen int
}

type cachedProductValueEntry[K comparable] struct {
	key   K
	value cachedProductValue
}

type edgeNormalCacheKey struct {
	from cfg.Point
	to   cfg.Point
}

type cachedSignatureType struct {
	value *typ.Function
	ok    bool
}

func (c *resultQueryCache) sourceValue(
	key sourceValueCacheKey,
	compute func() (product.Value, bool),
) (product.Value, bool) {
	if compute == nil {
		return product.Value{}, false
	}
	if cached, ok := c.sourceValues.lookup(key); ok {
		return cached.value, cached.ok
	}
	value, ok := compute()
	c.sourceValues.remember(key, cachedProductValue{value: value, ok: ok})
	return value, ok
}

func (c *resultQueryCache) pathValue(
	key pathValueCacheKey,
	compute func() (product.Value, bool),
) (product.Value, bool) {
	if compute == nil {
		return product.Value{}, false
	}
	if cached, ok := c.pathValues.lookup(key); ok {
		return cached.value, cached.ok
	}
	value, ok := compute()
	c.pathValues.remember(key, cachedProductValue{value: value, ok: ok})
	return value, ok
}

func (c *cachedProductValueCache[K]) reset() {
	if c == nil {
		return
	}
	c.values = nil
	for i := 0; i < c.inlineLen; i++ {
		c.inline[i] = cachedProductValueEntry[K]{}
	}
	c.inlineLen = 0
}

func (c *cachedProductValueCache[K]) lookup(key K) (cachedProductValue, bool) {
	if c == nil {
		return cachedProductValue{}, false
	}
	for i := 0; i < c.inlineLen; i++ {
		entry := c.inline[i]
		if entry.key == key {
			return entry.value, true
		}
	}
	if c.values == nil {
		return cachedProductValue{}, false
	}
	cached, ok := c.values[key]
	return cached, ok
}

func (c *cachedProductValueCache[K]) remember(key K, value cachedProductValue) {
	if c.values != nil {
		c.values[key] = value
		return
	}
	if c.inlineLen < len(c.inline) {
		c.inline[c.inlineLen] = cachedProductValueEntry[K]{key: key, value: value}
		c.inlineLen++
		return
	}
	c.values = make(map[K]cachedProductValue, len(c.inline)+1)
	for i := 0; i < c.inlineLen; i++ {
		entry := c.inline[i]
		c.values[entry.key] = entry.value
		c.inline[i] = cachedProductValueEntry[K]{}
	}
	c.inlineLen = 0
	c.values[key] = value
}

func (c *cachedProductValueCache[K]) count() int {
	if c == nil {
		return 0
	}
	return c.inlineLen + len(c.values)
}

func (c *cachedProductValueCache[K]) forEachKey(fn func(K) bool) {
	if c == nil || fn == nil {
		return
	}
	for i := 0; i < c.inlineLen; i++ {
		if !fn(c.inline[i].key) {
			return
		}
	}
	for key := range c.values {
		if !fn(key) {
			return
		}
	}
}

func (c *resultQueryCache) pathValueCount() int {
	if c == nil {
		return 0
	}
	return c.pathValues.count()
}

func (c *resultQueryCache) forEachPathValueKey(fn func(pathValueCacheKey) bool) {
	if c == nil {
		return
	}
	c.pathValues.forEachKey(fn)
}

func (c *resultQueryCache) callOutcome(point cfg.Point) (callpayload.CallOutcome, bool) {
	if c.callOutcomes == nil {
		return callpayload.CallOutcome{}, false
	}
	outcome, ok := c.callOutcomes[point]
	return outcome, ok
}

func (c *resultQueryCache) rememberCallOutcome(point cfg.Point, outcome callpayload.CallOutcome) {
	if c.callOutcomes == nil {
		c.callOutcomes = make(map[cfg.Point]callpayload.CallOutcome, c.callOutcomeCapacity)
	}
	c.callOutcomes[point] = outcome
}

func (c *resultQueryCache) edgeCanCompleteNormally(key edgeNormalCacheKey) (bool, bool) {
	if c.edgeNormal == nil {
		return false, false
	}
	normal, ok := c.edgeNormal[key]
	return normal, ok
}

func (c *resultQueryCache) rememberEdgeCanCompleteNormally(key edgeNormalCacheKey, normal bool) {
	if c.edgeNormal == nil {
		c.edgeNormal = make(map[edgeNormalCacheKey]bool)
	}
	c.edgeNormal[key] = normal
}

func (c *resultQueryCache) signatureType(name string) (cachedSignatureType, bool) {
	if c.signatureTypes == nil {
		return cachedSignatureType{}, false
	}
	cached, ok := c.signatureTypes[name]
	return cached, ok
}

func (c *resultQueryCache) rememberSignatureType(name string, cached cachedSignatureType) {
	if c.signatureTypes == nil {
		c.signatureTypes = make(map[string]cachedSignatureType)
	}
	c.signatureTypes[name] = cached
}

func (c *resultQueryCache) controlReachability(graph cfg.Graph) *cfg.Reachability {
	if c == nil || graph == nil {
		return nil
	}
	if c.reachability == nil {
		c.reachability = cfg.NewReachability(graph)
	}
	return c.reachability
}

func (c *resultQueryCache) immediateDominatorMap(graph cfg.Graph) map[cfg.Point]cfg.Point {
	if c == nil || graph == nil {
		return nil
	}
	if c.immediateDominators == nil {
		c.immediateDominators = dominance.ComputeImmediateDominatorInfo(graph).Map()
	}
	return c.immediateDominators
}

func (c *resultQueryCache) readContext(mode sourceValueReadMode) *readexpr.Context {
	if mode >= sourceValueReadModeCount {
		return nil
	}
	return &c.readContexts[mode]
}

func (c *resultQueryCache) sourceResolver(mode sourceValueReadMode) sourcevalue.SourceValues {
	if mode >= sourceValueReadModeCount {
		return nil
	}
	return c.sourceResolvers[mode]
}

func (c *resultQueryCache) rememberSourceResolver(mode sourceValueReadMode, sources sourcevalue.SourceValues) {
	if mode >= sourceValueReadModeCount {
		return
	}
	c.sourceResolvers[mode] = sources
}
