package body

import (
	"github.com/wippyai/go-lua/analysis/check/body/internal/readexpr"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// resultQueryCache owns memoized derived queries for one solved Result. The
// keys include the read mode and CFG point where needed, so cached values do not
// cross semantic boundaries such as before-node vs boundary-state reads.
type resultQueryCache struct {
	sourceValues           cachedProductValueCache[sourceValueCacheKey]
	pathValues             cachedProductValueCache[pathValueCacheKey]
	normalReachable        map[cfg.Point]bool
	normalReachableSet     bool
	memberReadSources      []dominatingMemberReadPresenceSource
	memberReadSourcesOK    bool
	memberReadPresence     map[dominatingMemberReadPresenceKey]bool
	signatureTypes         map[string]cachedSignatureType
	reachability           *cfg.Reachability
	immediateDominatorInfo *dominance.ImmediateDominators
	immediateDominators    map[cfg.Point]cfg.Point
	readContexts           [sourceValueReadModeCount]readexpr.Context
	sourceResolvers        [sourceValueReadModeCount]sourcevalue.SourceValues
	branchSites            map[cfg.Point]branchSite
	branchSitesOK          bool
	returnFacts            map[cfg.Point]ReturnFact
	returnFactsOK          bool
	sourceCalls            map[cfg.Point]SourceCallFact
	sourceCallsOK          bool
	numericForFacts        map[cfg.Point]NumericForFact
	numericForFactsOK      bool
	expressionsByID        map[wir.ExpressionID]ast.Expr
	expressionsByIDOK      bool
}

const resultQueryInline = 4

func newResultQueryCache() resultQueryCache {
	return resultQueryCache{}
}

func (c *resultQueryCache) reset() {
	if c == nil {
		return
	}
	c.sourceValues.reset()
	c.pathValues.reset()
	c.normalReachable = nil
	c.normalReachableSet = false
	c.memberReadSources = nil
	c.memberReadSourcesOK = false
	c.memberReadPresence = nil
	c.signatureTypes = nil
	c.reachability = nil
	c.immediateDominatorInfo = nil
	c.immediateDominators = nil
	c.readContexts = [sourceValueReadModeCount]readexpr.Context{}
	c.sourceResolvers = [sourceValueReadModeCount]sourcevalue.SourceValues{}
	c.branchSites = nil
	c.branchSitesOK = false
	c.returnFacts = nil
	c.returnFactsOK = false
	c.sourceCalls = nil
	c.sourceCallsOK = false
	c.numericForFacts = nil
	c.numericForFactsOK = false
	c.expressionsByID = nil
	c.expressionsByIDOK = false
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
	path  keyspace.Key
}

func newPathValueCacheKey(ks *keyspace.KeySpace, mode sourceValueReadMode, point cfg.Point, p pathdom.Path) (pathValueCacheKey, bool) {
	if ks == nil || !ks.Valid() || p.IsEmpty() {
		return pathValueCacheKey{}, false
	}
	pathKey := ks.FromPath(p)
	if pathKey.Kind == keyspace.KindInvalid {
		return pathValueCacheKey{}, false
	}
	return pathValueCacheKey{mode: mode, point: point, path: pathKey}, true
}

type dominatingMemberReadPresenceKey struct {
	point cfg.Point
	path  keyspace.Key
}

func newDominatingMemberReadPresenceKey(ks *keyspace.KeySpace, point cfg.Point, p pathdom.Path) (dominatingMemberReadPresenceKey, bool) {
	if ks == nil || !ks.Valid() || p.IsEmpty() {
		return dominatingMemberReadPresenceKey{}, false
	}
	pathKey := ks.FromPath(p)
	if pathKey.Kind == keyspace.KindInvalid {
		return dominatingMemberReadPresenceKey{}, false
	}
	return dominatingMemberReadPresenceKey{point: point, path: pathKey}, true
}

type cachedProductValue struct {
	value product.Value
	ok    bool
}

type cachedProductValueCache[K comparable] struct {
	values     map[K]cachedProductValue
	inProgress map[K]cachedProductValue
	inline     [resultQueryInline]cachedProductValueEntry[K]
	inlineLen  int
}

type cachedProductValueEntry[K comparable] struct {
	key   K
	value cachedProductValue
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

// boundaryPathValue resolves a boundary path in two phases. The seed is the
// value projected from the sealed boundary-node output; it is published as an
// in-progress result before follow-up recovery/proof reads run. Those reads
// may legitimately ask for the same (boundary node, path), in which case the
// sealed value is the stable answer rather than a reason to replay resolution.
func (c *resultQueryCache) boundaryPathValue(
	key pathValueCacheKey,
	seed func() (product.Value, bool),
	resolve func() (product.Value, bool),
) (value product.Value, ok bool) {
	if seed == nil || resolve == nil {
		return product.Value{}, false
	}
	if cached, found := c.pathValues.lookup(key); found {
		return cached.value, cached.ok
	}
	value, ok = seed()
	c.pathValues.begin(key, cachedProductValue{value: value, ok: ok})
	defer func() {
		c.pathValues.finish(key)
		c.pathValues.remember(key, cachedProductValue{value: value, ok: ok})
	}()
	return resolve()
}

func (c *cachedProductValueCache[K]) reset() {
	if c == nil {
		return
	}
	c.values = nil
	c.inProgress = nil
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
		if c.inProgress == nil {
			return cachedProductValue{}, false
		}
		cached, ok := c.inProgress[key]
		return cached, ok
	}
	cached, ok := c.values[key]
	if ok {
		return cached, true
	}
	if c.inProgress == nil {
		return cachedProductValue{}, false
	}
	cached, ok = c.inProgress[key]
	return cached, ok
}

func (c *cachedProductValueCache[K]) begin(key K, value cachedProductValue) {
	if c == nil {
		return
	}
	if c.inProgress == nil {
		c.inProgress = make(map[K]cachedProductValue)
	}
	c.inProgress[key] = value
}

func (c *cachedProductValueCache[K]) finish(key K) {
	if c == nil || c.inProgress == nil {
		return
	}
	delete(c.inProgress, key)
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
		c.immediateDominators = c.immediateDominatorInfoFor(graph).Map()
	}
	return c.immediateDominators
}

func (c *resultQueryCache) immediateDominatorInfoFor(graph cfg.Graph) *dominance.ImmediateDominators {
	if c == nil || graph == nil {
		return nil
	}
	if c.immediateDominatorInfo == nil {
		c.immediateDominatorInfo = dominance.ComputeImmediateDominatorInfo(graph)
	}
	return c.immediateDominatorInfo
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
