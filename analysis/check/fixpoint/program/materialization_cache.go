package program

import (
	"fmt"
	"hash/fnv"
	"slices"

	"github.com/wippyai/go-lua/analysis/check/body"
	summaryprojection "github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/projectsummary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

type materializedProgram struct {
	root        *body.Result
	resultKey   map[*body.Result]summary.SummaryKey
	projections *resultSummaryProjectionCache
	keys        programKeys
}

type resultSummaryProjectionCache struct {
	entries map[*body.Result]summary.Summary
}

func newResultSummaryProjectionCache() *resultSummaryProjectionCache {
	return &resultSummaryProjectionCache{}
}

func (c *resultSummaryProjectionCache) invalidate(result *body.Result) {
	if c == nil || result == nil || len(c.entries) == 0 {
		return
	}
	delete(c.entries, result)
}

// releaseDiscarded drops materialized bodies superseded by a proof-driven
// rematerialization pass. The solve cache and projection cache are both
// long-lived for the duration of that pass; without this boundary, replaced
// flows remain reachable until the entire program finishes materializing.
func (c *resultSummaryProjectionCache) releaseDiscarded(previous, next materializedProgram) {
	if len(previous.resultKey) == 0 {
		return
	}
	for result := range previous.resultKey {
		if _, retained := next.resultKey[result]; retained {
			continue
		}
		if c != nil && len(c.entries) != 0 {
			delete(c.entries, result)
		}
		result.ReleaseTransient()
	}
}

func (c *resultSummaryProjectionCache) project(result *body.Result) (summary.Summary, bool) {
	if result == nil {
		return summary.Summary{}, false
	}
	if c == nil {
		return summaryprojection.FromResult(result), true
	}
	if len(c.entries) != 0 {
		if got, ok := c.entries[result]; ok {
			return got.Clone(), true
		}
	}
	projected := summaryprojection.FromResult(result)
	if c.entries == nil {
		c.entries = make(map[*body.Result]summary.Summary)
	}
	c.entries[result] = projected
	return projected.Clone(), true
}

type materializedSolveCache struct {
	reg     *axis.Registry
	entries map[materializedSolveCacheKey]materializedSolveCacheEntry
}

type materializedSolveCacheKey struct {
	prepared *body.Static
	owner    summary.SummaryKey
}

type materializedSolveCacheEntry struct {
	// routing identifies the owner-local call-context routing table captured by
	// this solve. It deliberately excludes unrelated program contexts.
	routing uint64
	entry   materializedSolveEntryState
	deps    map[summary.SummaryKey]trackedSummaryRead
	// noDepUniverse pins solves with zero tracked summary reads to the summary
	// universe they observed. A later materialization pass can make a callee
	// summary nameable even though the first solve had no dependency to track.
	noDepUniverseKnown bool
	noDepUniverse      []summary.EntrySummary

	result *body.Result
}

type materializedSolveEntryState struct {
	state state.State
	ok    bool
}

type trackedSummaryRead struct {
	present bool
	sum     summary.Summary
}

type trackingSummaryReader struct {
	reg  *axis.Registry
	base summary.Reader
	deps map[summary.SummaryKey]trackedSummaryRead
}

func newMaterializedSolveCache(reg *axis.Registry) *materializedSolveCache {
	if reg == nil {
		return nil
	}
	return &materializedSolveCache{reg: reg}
}

func (r *trackingSummaryReader) Read(key summary.SummaryKey) (summary.Summary, bool) {
	if r == nil || r.base == nil {
		if r != nil {
			r.remember(key, summary.Summary{}, false)
		}
		return summary.Summary{}, false
	}
	if owned, ok := r.base.(summary.OwnedNormalizedReader); ok {
		got, ok := owned.ReadOwnedNormalized(key)
		r.rememberOwned(key, got, ok)
		if !ok {
			return summary.Summary{}, false
		}
		return got.Clone(), true
	}
	got, ok := r.base.Read(key)
	r.remember(key, got, ok)
	return got, ok
}

func (r *trackingSummaryReader) ReadOwnedNormalized(key summary.SummaryKey) (summary.Summary, bool) {
	if r == nil || r.base == nil {
		if r != nil {
			r.rememberOwned(key, summary.Summary{}, false)
		}
		return summary.Summary{}, false
	}
	if owned, ok := r.base.(summary.OwnedNormalizedReader); ok {
		got, ok := owned.ReadOwnedNormalized(key)
		r.rememberOwned(key, got, ok)
		return got, ok
	}
	got, ok := r.base.Read(key)
	if !ok {
		r.rememberOwned(key, summary.Summary{}, false)
		return summary.Summary{}, false
	}
	normalized := summary.Normalize(r.reg, got)
	r.rememberOwned(key, normalized, true)
	return normalized, true
}

func (r *trackingSummaryReader) remember(key summary.SummaryKey, got summary.Summary, ok bool) {
	if r == nil {
		return
	}
	if r.deps == nil {
		r.deps = make(map[summary.SummaryKey]trackedSummaryRead)
	}
	if !ok {
		r.deps[key] = trackedSummaryRead{}
		return
	}
	dep := trackedSummaryRead{present: true, sum: summary.Normalize(r.reg, got)}
	r.deps[key] = dep
}

func (r *trackingSummaryReader) rememberOwned(key summary.SummaryKey, got summary.Summary, ok bool) {
	if r == nil {
		return
	}
	if r.deps == nil {
		r.deps = make(map[summary.SummaryKey]trackedSummaryRead)
	}
	if !ok {
		r.deps[key] = trackedSummaryRead{}
		return
	}
	dep := trackedSummaryRead{present: true, sum: got}
	r.deps[key] = dep
}

func solveMaterializedPrepared(
	cache *materializedSolveCache,
	prepared *body.Static,
	owner summary.SummaryKey,
	routing uint64,
	entry materializedSolveEntryState,
	summaries summary.Reader,
	buildConfig func(summary.Reader) body.Config,
	counter *int,
) (*body.Result, bool, error) {
	if prepared == nil || buildConfig == nil {
		return nil, false, nil
	}
	if cache == nil {
		result, err := solvePreparedCounted(prepared, buildConfig(summaries), counter)
		return result, true, err
	}
	if cached, ok := cache.read(prepared, owner, routing, entry, summaries); ok {
		config := buildConfig(summaries)
		return body.RebindBoundaryProviders(cached, prepared, config.SolveConfig()), false, nil
	}
	tracked := &trackingSummaryReader{reg: cache.reg, base: summaries}
	config := buildConfig(tracked)
	config.SummaryInputDigests = func() []uint64 {
		return trackedSummaryReadDigests(cache.reg, tracked.deps)
	}
	result, err := solvePreparedCounted(prepared, config, counter)
	if err != nil {
		return nil, true, err
	}
	cache.write(prepared, owner, routing, entry, summaries, tracked.deps, result)
	return result, true, nil
}

func trackedSummaryReadDigests(reg *axis.Registry, deps map[summary.SummaryKey]trackedSummaryRead) []uint64 {
	if len(deps) == 0 {
		return nil
	}
	out := make([]uint64, 0, len(deps))
	for _, dep := range deps {
		h := fnv.New64a()
		if dep.present {
			_, _ = h.Write([]byte("present:"))
			payload := uint64(summary.NormalizedPayloadDigest(reg, dep.sum))
			fmt.Fprintf(h, "%d;", payload)
		} else {
			_, _ = h.Write([]byte("missing"))
		}
		out = append(out, h.Sum64())
	}
	slices.Sort(out)
	return out
}

func (c *materializedSolveCache) read(
	prepared *body.Static,
	owner summary.SummaryKey,
	routing uint64,
	entry materializedSolveEntryState,
	summaries summary.Reader,
) (*body.Result, bool) {
	if c == nil || prepared == nil || summaries == nil || len(c.entries) == 0 {
		return nil, false
	}
	cached, ok := c.entries[materializedSolveCacheKey{prepared: prepared, owner: owner}]
	if !ok || cached.result == nil || cached.routing != routing {
		return nil, false
	}
	if !materializedSolveEntryStatesEqual(c.reg, cached.entry, entry) {
		return nil, false
	}
	if len(cached.deps) == 0 {
		if !cached.noDepUniverseKnown {
			return nil, false
		}
		current, ok := materializedSummaryUniverse(summaries)
		if !ok || !summaryEntryUniversesEqual(c.reg, cached.noDepUniverse, current) {
			return nil, false
		}
	}
	for key, dep := range cached.deps {
		got, gotOK := readOwnedNormalizedSummary(c.reg, summaries, key)
		if gotOK != dep.present {
			return nil, false
		}
		if !gotOK {
			continue
		}
		if !summary.EqualNormalized(c.reg, got, dep.sum) {
			return nil, false
		}
	}
	return cached.result, true
}

func readOwnedNormalizedSummary(reg *axis.Registry, reader summary.Reader, key summary.SummaryKey) (summary.Summary, bool) {
	if reader == nil {
		return summary.Summary{}, false
	}
	if owned, ok := reader.(summary.OwnedNormalizedReader); ok {
		return owned.ReadOwnedNormalized(key)
	}
	got, ok := reader.Read(key)
	if !ok {
		return summary.Summary{}, false
	}
	return summary.Normalize(reg, got), true
}

func (c *materializedSolveCache) write(
	prepared *body.Static,
	owner summary.SummaryKey,
	routing uint64,
	entry materializedSolveEntryState,
	summaries summary.Reader,
	deps map[summary.SummaryKey]trackedSummaryRead,
	result *body.Result,
) {
	if c == nil || prepared == nil || result == nil {
		return
	}
	if c.entries == nil {
		c.entries = make(map[materializedSolveCacheKey]materializedSolveCacheEntry)
	}
	var noDepUniverse []summary.EntrySummary
	noDepUniverseKnown := false
	if len(deps) == 0 {
		noDepUniverse, noDepUniverseKnown = materializedSummaryUniverse(summaries)
	}
	c.entries[materializedSolveCacheKey{prepared: prepared, owner: owner}] = materializedSolveCacheEntry{
		routing:            routing,
		entry:              entry,
		deps:               cloneTrackedSummaryReads(deps),
		noDepUniverseKnown: noDepUniverseKnown,
		noDepUniverse:      noDepUniverse,
		result:             result,
	}
}

func materializedSummaryUniverse(reader summary.Reader) ([]summary.EntrySummary, bool) {
	entriesReader, ok := reader.(interface{ EntriesOwnedNormalized() []summary.EntrySummary })
	if !ok {
		return nil, false
	}
	entries := entriesReader.EntriesOwnedNormalized()
	if len(entries) == 0 {
		return nil, true
	}
	return slices.Clone(entries), true
}

func summaryEntryUniversesEqual(reg *axis.Registry, left, right []summary.EntrySummary) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i].Key != right[i].Key {
			return false
		}
		if !summary.EqualNormalized(reg, left[i].Summary, right[i].Summary) {
			return false
		}
	}
	return true
}

func materializedSolveEntryFor(prepared *body.Static, fn keyedFunction) materializedSolveEntryState {
	if prepared == nil || !fn.hasEntryState {
		return materializedSolveEntryState{}
	}
	return materializedSolveEntryState{
		state: fn.entryState.RekeyPathEvidence(fn.entryKeys, prepared.KeySpace()),
		ok:    true,
	}
}

func materializedSolveEntryStatesEqual(reg *axis.Registry, a, b materializedSolveEntryState) bool {
	if a.ok != b.ok {
		return false
	}
	if !a.ok {
		return true
	}
	return state.Domain(reg).Equal(a.state, b.state)
}

func cloneTrackedSummaryReads(in map[summary.SummaryKey]trackedSummaryRead) map[summary.SummaryKey]trackedSummaryRead {
	if len(in) == 0 {
		return nil
	}
	out := make(map[summary.SummaryKey]trackedSummaryRead, len(in))
	for key, dep := range in {
		if dep.present {
			dep.sum = dep.sum.Clone()
		}
		out[key] = dep
	}
	return out
}

type materializedSummaryCache struct {
	reg         *axis.Registry
	base        summary.Reader
	projections *resultSummaryProjectionCache
	entries     map[summary.SummaryKey]summary.Summary
}

func newMaterializedSummaryCache(reg *axis.Registry, base summary.Reader, projections *resultSummaryProjectionCache) *materializedSummaryCache {
	return &materializedSummaryCache{reg: reg, base: base, projections: projections}
}

func (c *materializedSummaryCache) Read(key summary.SummaryKey) (summary.Summary, bool) {
	if c == nil {
		return summary.Summary{}, false
	}
	if len(c.entries) != 0 {
		if got, ok := c.entries[key]; ok {
			return got.Clone(), true
		}
	}
	if c.base == nil {
		return summary.Summary{}, false
	}
	if owned, ok := c.base.(summary.OwnedNormalizedReader); ok {
		got, ok := owned.ReadOwnedNormalized(key)
		if !ok {
			return summary.Summary{}, false
		}
		return got.Clone(), true
	}
	return c.base.Read(key)
}

func (c *materializedSummaryCache) ReadOwnedNormalized(key summary.SummaryKey) (summary.Summary, bool) {
	if c == nil {
		return summary.Summary{}, false
	}
	if len(c.entries) != 0 {
		if got, ok := c.entries[key]; ok {
			return got, true
		}
	}
	if c.base == nil {
		return summary.Summary{}, false
	}
	if owned, ok := c.base.(summary.OwnedNormalizedReader); ok {
		return owned.ReadOwnedNormalized(key)
	}
	got, ok := c.base.Read(key)
	if !ok {
		return summary.Summary{}, false
	}
	return summary.Normalize(c.reg, got), true
}

func (c *materializedSummaryCache) readOwned(key summary.SummaryKey) (summary.Summary, bool) {
	return c.ReadOwnedNormalized(key)
}

func (c *materializedSummaryCache) EntriesOwnedNormalized() []summary.EntrySummary {
	if c == nil {
		return nil
	}
	byKey := make(map[summary.SummaryKey]summary.Summary)
	if entries, ok := c.base.(interface{ EntriesOwnedNormalized() []summary.EntrySummary }); ok {
		for _, entry := range entries.EntriesOwnedNormalized() {
			byKey[entry.Key] = entry.Summary
		}
	}
	for key, got := range c.entries {
		byKey[key] = got
	}
	if len(byKey) == 0 {
		return nil
	}
	keys := make([]summary.SummaryKey, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b summary.SummaryKey) int {
		if a.Less(b) {
			return -1
		}
		if b.Less(a) {
			return 1
		}
		return 0
	})
	out := make([]summary.EntrySummary, 0, len(keys))
	for _, key := range keys {
		out = append(out, summary.EntrySummary{Key: key, Summary: byKey[key]})
	}
	return out
}

func (c *materializedSummaryCache) write(key summary.SummaryKey, sum summary.Summary) {
	if c == nil || c.reg == nil {
		return
	}
	next := summary.NormalizeOwned(c.reg, sum)
	if current, ok := c.readOwned(key); ok && summary.EqualNormalized(c.reg, current, next) {
		return
	}
	if c.entries == nil {
		c.entries = make(map[summary.SummaryKey]summary.Summary)
	}
	c.entries[key] = next
}

func (c *materializedSummaryCache) writeResult(key summary.SummaryKey, result *body.Result) {
	if c == nil || result == nil {
		return
	}
	if current, ok := c.readOwned(key); ok {
		entries := map[summary.SummaryKey]summary.Summary{key: current}
		if overlayMaterializedSummaryProofsForResult(c.reg, entries, key, result, c.projections) {
			c.write(key, entries[key])
		}
		return
	}
	projected, ok := c.projections.project(result)
	if !ok {
		return
	}
	c.write(key, projected)
}
