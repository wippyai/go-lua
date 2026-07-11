package program

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/check/body"
	summaryprojection "github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/projectsummary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

// SummarySolveCache shares exact body-to-summary applications. Unlike the
// materialization cache, it retains only normalized summaries: retaining a
// body.Result here would retain every CFG-point state from every unit.
//
// Entries are keyed by stable body content and solve input digests, then
// validated against every summary actually read by the body. The latter is
// essential: the set of reads is discovered while applying a body, so it is
// not available in the provisional key.
type SummarySolveCache struct {
	mu      sync.RWMutex
	reg     *axis.Registry
	entries map[summarySolveCacheKey][]summarySolveCacheEntry
}

type summarySolveCacheKey struct {
	body    uint64
	input   uint64
	profile string
	// resolution identifies the call-to-summary-key routing graph visible to
	// the owner. It prevents reuse when a body has identical local inputs but
	// a different program resolves one of its calls to another callee key.
	resolution uint64
}

type summarySolveCacheEntry struct {
	deps map[summary.SummaryKey]trackedSummaryRead
	sum  summary.Summary
}

// NewSummarySolveCache creates a cache suitable for sharing by all solves in
// one BatchSession. A nil registry disables caching safely.
func NewSummarySolveCache(reg *axis.Registry) *SummarySolveCache {
	if reg == nil {
		return nil
	}
	return &SummarySolveCache{reg: reg}
}

func (c *SummarySolveCache) solve(
	prepared *body.Static,
	profile string,
	resolution uint64,
	reader summary.Reader,
	build func(summary.Reader) body.Config,
	counter *int,
	transfers *int,
	dependencyChanges *int,
	dependencyChangeTransfers *int,
	hits *int,
	misses *int,
) (summary.Summary, error) {
	if prepared == nil || build == nil {
		return summary.Summary{}, nil
	}
	config := build(reader)
	bodyDigest, err := prepared.IdentityDigestContext(config.Context)
	if err != nil {
		return summary.Summary{}, err
	}
	inputDigest, err := body.InputDigestContext(prepared, config.SolveConfig())
	if err != nil {
		return summary.Summary{}, err
	}
	key := summarySolveCacheKey{
		body:       bodyDigest,
		input:      inputDigest,
		profile:    profile,
		resolution: resolution,
	}
	if cached, ok := c.read(key, reader); ok {
		if hits != nil {
			(*hits)++
		}
		return cached, nil
	}
	if misses != nil {
		(*misses)++
	}
	dependencyChanged := c.dependencyChanged(key, reader)
	tracked := &trackingSummaryReader{reg: c.registry(), base: reader}
	config = build(tracked)
	config.SummaryInputDigests = func() []uint64 {
		return trackedSummaryReadDigests(c.registry(), tracked.deps)
	}
	beforeTransfers := 0
	if dependencyChanged && config.Stats != nil {
		beforeTransfers = config.Stats.Transfer.Solver.TransferCalls
	}
	if dependencyChanged && dependencyChanges != nil {
		(*dependencyChanges)++
	}
	result, err := solvePreparedCountedWithTransfers(prepared, config, counter, transfers)
	if dependencyChanged && dependencyChangeTransfers != nil && config.Stats != nil {
		*dependencyChangeTransfers += config.Stats.Transfer.Solver.TransferCalls - beforeTransfers
	}
	if err != nil {
		return summary.Summary{}, err
	}
	projected, err := summaryprojection.FromResultContext(config.Context, result)
	if err != nil {
		return summary.Summary{}, err
	}
	c.write(key, tracked.deps, projected)
	return projected, nil
}

// dependencyChanged reports whether a previous exact application exists for
// this body variant but cannot be reused under reader. It deliberately does
// not inspect summary values itself: trackedSummaryReadsMatch owns normalized
// equality and missing-summary handling.
func (c *SummarySolveCache) dependencyChanged(key summarySolveCacheKey, reader summary.Reader) bool {
	if c == nil || reader == nil {
		return false
	}
	c.mu.RLock()
	entries := append([]summarySolveCacheEntry(nil), c.entries[key]...)
	reg := c.reg
	c.mu.RUnlock()
	for _, entry := range entries {
		if !trackedSummaryReadsMatch(reg, entry.deps, reader) {
			return true
		}
	}
	return false
}

func (c *SummarySolveCache) registry() *axis.Registry {
	if c == nil {
		return nil
	}
	return c.reg
}

func (c *SummarySolveCache) read(key summarySolveCacheKey, reader summary.Reader) (summary.Summary, bool) {
	if c == nil || reader == nil {
		return summary.Summary{}, false
	}
	c.mu.RLock()
	entries := append([]summarySolveCacheEntry(nil), c.entries[key]...)
	reg := c.reg
	c.mu.RUnlock()
	for _, entry := range entries {
		if trackedSummaryReadsMatch(reg, entry.deps, reader) {
			return entry.sum.Clone(), true
		}
	}
	return summary.Summary{}, false
}

func (c *SummarySolveCache) write(key summarySolveCacheKey, deps map[summary.SummaryKey]trackedSummaryRead, sum summary.Summary) {
	if c == nil {
		return
	}
	entry := summarySolveCacheEntry{deps: cloneTrackedSummaryReads(deps), sum: sum.Clone()}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[summarySolveCacheKey][]summarySolveCacheEntry)
	}
	for _, existing := range c.entries[key] {
		if trackedSummaryReadSetsEqual(c.reg, existing.deps, entry.deps) {
			return
		}
	}
	c.entries[key] = append(c.entries[key], entry)
}

func trackedSummaryReadsMatch(reg *axis.Registry, deps map[summary.SummaryKey]trackedSummaryRead, reader summary.Reader) bool {
	for key, dep := range deps {
		got, ok := readOwnedNormalizedSummary(reg, reader, key)
		if ok != dep.present {
			return false
		}
		if ok && !summary.EqualNormalized(reg, got, dep.sum) {
			return false
		}
	}
	return true
}

func trackedSummaryReadSetsEqual(reg *axis.Registry, left, right map[summary.SummaryKey]trackedSummaryRead) bool {
	if len(left) != len(right) {
		return false
	}
	for key, want := range left {
		got, ok := right[key]
		if !ok || got.present != want.present {
			return false
		}
		if got.present && !summary.EqualNormalized(reg, got.sum, want.sum) {
			return false
		}
	}
	return true
}
