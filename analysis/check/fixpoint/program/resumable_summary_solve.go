package program

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	summaryprojection "github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/projectsummary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"reflect"
)

// resumableSummarySolves is deliberately created by one program Run.  Unlike
// SummarySolveCache it owns live CFG cells and must not survive that run.
type resumableSummarySolves struct {
	reg     *axis.Registry
	entries map[resumableVariantID]*resumableSummarySolve
}

type resumableVariantID struct {
	owner      summary.SummaryKey
	body       uint64
	input      uint64
	profile    string
	resolution uint64
	widenAt    uintptr
	widenDelay uintptr
}

type resumableSummarySolve struct {
	session   *transfer.Session
	deps      map[summary.SummaryKey]trackedSummaryRead
	pointDeps map[cfg.Point]map[summary.SummaryKey]trackedSummaryRead
	sum       summary.Summary
}

func newResumableSummarySolves(reg *axis.Registry) *resumableSummarySolves {
	if reg == nil {
		return nil
	}
	return &resumableSummarySolves{reg: reg, entries: make(map[resumableVariantID]*resumableSummarySolve)}
}

func (r *resumableSummarySolves) solve(
	owner summary.SummaryKey,
	cache *SummarySolveCache,
	profile string,
	resolution uint64,
	prepared *body.Static,
	reader summary.Reader,
	build func(summary.Reader) body.Config,
	stats *Stats,
) (summary.Summary, error) {
	if prepared == nil || build == nil {
		return summary.Summary{}, nil
	}
	base := build(reader)
	key := resumableVariantID{owner: owner, body: prepared.IdentityDigest(), input: body.InputDigest(prepared, base.SolveConfig()), profile: profile, resolution: resolution, widenAt: functionIdentity(base.WidenAt), widenDelay: functionIdentity(base.WidenDelay)}
	cacheKey := summarySolveCacheKey{body: key.body, input: key.input, profile: profile, resolution: resolution}
	if cache != nil {
		if cached, ok := cache.read(cacheKey, reader); ok {
			if hit := summaryCacheHitCounter(stats); hit != nil {
				*hit++
			}
			return cached, nil
		}
		if miss := summaryCacheMissCounter(stats); miss != nil {
			*miss++
		}
	}

	entry := r.entries[key]
	if entry != nil {
		changed, growth, scoped := changedSummaryReaders(r.reg, entry, reader)
		if !changed {
			return entry.sum.Clone(), nil
		}
		if growth && len(scoped) != 0 && wideningFreeAffected(prepared.Graph(), base.WidenAt, scoped) {
			if resumed := summaryResumeHitCounter(stats); resumed != nil {
				*resumed++
			}
			return r.resume(entry, prepared, reader, build, scoped, stats, cache, cacheKey)
		}
		if fallback := summaryResumeFallbackCounter(stats); fallback != nil {
			*fallback++
		}
	}
	return r.fresh(key, prepared, reader, build, stats, cache, cacheKey, entry != nil)
}

func functionIdentity[T any](fn T) uintptr {
	v := reflect.ValueOf(fn)
	if !v.IsValid() || v.IsNil() {
		return 0
	}
	return v.Pointer()
}

func (r *resumableSummarySolves) fresh(key resumableVariantID, prepared *body.Static, reader summary.Reader, build func(summary.Reader) body.Config, stats *Stats, cache *SummarySolveCache, cacheKey summarySolveCacheKey, dependencyChange bool) (summary.Summary, error) {
	tracked := &trackingSummaryReader{reg: r.reg, base: reader, pointDeps: make(map[cfg.Point]map[summary.SummaryKey]trackedSummaryRead)}
	config := build(tracked)
	config.Resume = transfer.NewSession()
	config.BeforePoint = func(point cfg.Point) { tracked.active = &point }
	config.AfterPoint = func(_ cfg.Point) { tracked.active = nil }
	config.SummaryInputDigests = func() []uint64 { return trackedSummaryReadDigests(r.reg, tracked.deps) }
	if dependencyChange {
		if c := summaryDependencyChangeCounter(stats); c != nil {
			(*c)++
		}
	}
	before := 0
	if dependencyChange && stats != nil {
		before = stats.Body.Transfer.Solver.TransferCalls
	}
	result, err := solvePreparedCountedWithTransfers(prepared, config, summaryCounter(stats), summaryPointTransferCounter(stats))
	if err != nil {
		return summary.Summary{}, err
	}
	if dependencyChange && stats != nil {
		if c := summaryDependencyChangePointTransferCounter(stats); c != nil {
			*c += stats.Body.Transfer.Solver.TransferCalls - before
		}
	}
	projected := summaryprojection.FromResult(result)
	entry := &resumableSummarySolve{session: config.Resume, deps: cloneTrackedSummaryReads(tracked.deps), pointDeps: clonePointDeps(tracked.pointDeps), sum: projected.Clone()}
	r.entries[key] = entry
	if cache != nil {
		cache.write(cacheKey, entry.deps, projected)
	}
	return projected, nil
}

func (r *resumableSummarySolves) resume(entry *resumableSummarySolve, prepared *body.Static, reader summary.Reader, build func(summary.Reader) body.Config, points []cfg.Point, stats *Stats, cache *SummarySolveCache, cacheKey summarySolveCacheKey) (summary.Summary, error) {
	tracked := &trackingSummaryReader{reg: r.reg, base: reader, pointDeps: clonePointDeps(entry.pointDeps)}
	config := build(tracked)
	// Reads performed while building providers are configuration dependencies;
	// keep them in the new dependency snapshot but do not pretend they belong
	// to one call point.
	configDeps := cloneTrackedSummaryReads(tracked.deps)
	for _, point := range points {
		delete(tracked.pointDeps, point)
	}
	config.Resume = entry.session
	config.ResumePoints = points
	config.BeforePoint = func(point cfg.Point) { tracked.active = &point }
	config.AfterPoint = func(_ cfg.Point) { tracked.active = nil }
	config.SummaryInputDigests = func() []uint64 {
		return trackedSummaryReadDigests(r.reg, mergePointDeps(configDeps, tracked.pointDeps))
	}
	if c := summaryDependencyChangeCounter(stats); c != nil {
		*c++
	}
	before := 0
	if stats != nil {
		before = stats.Body.Transfer.Solver.TransferCalls
	}
	result, err := solvePreparedCountedWithTransfers(prepared, config, summaryCounter(stats), summaryPointTransferCounter(stats))
	if err != nil {
		return summary.Summary{}, err
	}
	if c := summaryDependencyChangePointTransferCounter(stats); c != nil && stats != nil {
		*c += stats.Body.Transfer.Solver.TransferCalls - before
	}
	entry.deps = mergePointDeps(configDeps, tracked.pointDeps)
	entry.pointDeps = clonePointDeps(tracked.pointDeps)
	entry.sum = summaryprojection.FromResult(result).Clone()
	if cache != nil {
		cache.write(cacheKey, entry.deps, entry.sum)
	}
	return entry.sum.Clone(), nil
}

func changedSummaryReaders(reg *axis.Registry, entry *resumableSummarySolve, reader summary.Reader) (changed, growth bool, points []cfg.Point) {
	growth = true
	changedKeys := make(map[summary.SummaryKey]struct{})
	for key, old := range entry.deps {
		next, present := readOwnedNormalizedSummary(reg, reader, key)
		if present == old.present && (!present || summary.EqualNormalized(reg, old.sum, next)) {
			continue
		}
		changed = true
		changedKeys[key] = struct{}{}
		if old.present && (!present || !summary.LessOrEq(reg, old.sum, next)) {
			growth = false
		}
	}
	if !changed {
		return false, true, nil
	}
	for point, deps := range entry.pointDeps {
		for key := range deps {
			if _, ok := changedKeys[key]; ok {
				points = append(points, point)
				break
			}
		}
	}
	return changed, growth, points
}

func wideningFreeAffected(graph cfg.Graph, custom func(cfg.Point) bool, starts []cfg.Point) bool {
	// A custom predicate has no certificate that it cuts every cycle.  The
	// documented safe policy is therefore restart-only for custom widening.
	if graph == nil || custom != nil {
		return false
	}
	widen := transfer.DefaultWidenAt(graph)
	seen := make(map[cfg.Point]struct{}, len(starts))
	queue := append([]cfg.Point(nil), starts...)
	for len(queue) != 0 {
		p := queue[0]
		queue = queue[1:]
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		if widen(p) {
			return false
		}
		queue = append(queue, cfg.SuccessorsReadOnly(graph, p)...)
	}
	return true
}

func clonePointDeps(in map[cfg.Point]map[summary.SummaryKey]trackedSummaryRead) map[cfg.Point]map[summary.SummaryKey]trackedSummaryRead {
	out := make(map[cfg.Point]map[summary.SummaryKey]trackedSummaryRead, len(in))
	for p, deps := range in {
		out[p] = cloneTrackedSummaryReads(deps)
	}
	return out
}

func mergePointDeps(base map[summary.SummaryKey]trackedSummaryRead, points map[cfg.Point]map[summary.SummaryKey]trackedSummaryRead) map[summary.SummaryKey]trackedSummaryRead {
	out := cloneTrackedSummaryReads(base)
	if out == nil {
		out = make(map[summary.SummaryKey]trackedSummaryRead)
	}
	for _, deps := range points {
		for key, dep := range deps {
			out[key] = dep
		}
	}
	return out
}
