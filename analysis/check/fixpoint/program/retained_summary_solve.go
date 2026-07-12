package program

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/body"
	summaryprojection "github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/projectsummary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
)

// solveRetainedAttributed is the run-local WTO path behind the existing exact
// summary cache. Exact cache hits return before any retained machinery is
// touched. Cache entries remain summary-only; owner owns the body generation.
func (c *SummarySolveCache) solveRetainedAttributed(
	prepared *body.Static,
	profile string,
	resolution uint64,
	reader summary.Reader,
	build func(summary.Reader) body.Config,
	owner *retainedSummaryApplicationOwner,
	counter *int,
	transfers *int,
	dependencyChanges *int,
	dependencyChangeTransfers *int,
	hits *int,
	misses *int,
	attribution *solveAttribution,
) (summary.Summary, error) {
	if prepared == nil || build == nil || owner == nil {
		return c.solveAttributed(prepared, profile, resolution, reader, build, counter, transfers, dependencyChanges, dependencyChangeTransfers, hits, misses, attribution)
	}

	baseConfig := build(reader)
	if baseConfig.Schedule != transfer.ScheduleWTO {
		return c.solveAttributed(prepared, profile, resolution, reader, build, counter, transfers, dependencyChanges, dependencyChangeTransfers, hits, misses, attribution)
	}
	bodyDigest, err := prepared.IdentityDigestContext(baseConfig.Context)
	if err != nil {
		return summary.Summary{}, err
	}
	inputDigest, err := body.InputDigestContext(prepared, baseConfig.SolveConfig())
	if err != nil {
		return summary.Summary{}, err
	}
	cacheKey := summarySolveCacheKey{body: bodyDigest, input: inputDigest, profile: profile, resolution: resolution}
	applicationKey := retainedSummaryApplicationKey{
		body: bodyDigest, input: inputDigest, profile: profile, resolution: resolution,
	}
	if cached, ok := c.readEntry(cacheKey, reader); ok {
		if hits != nil {
			(*hits)++
		}
		owner.adoptCacheHit(applicationKey, cached.deps, cached.sum)
		return cached.sum, nil
	}
	if misses != nil {
		(*misses)++
	}

	dependencyChanged := c.dependencyChanged(cacheKey, reader)
	if dependencyChanged && dependencyChanges != nil {
		(*dependencyChanges)++
	}
	attempt := owner.begin(applicationKey, reader)
	decision := attempt.Decision()
	if decision.kind == retainedSummaryApplyReuse {
		attempt.Abort()
		return owner.published.sum.Clone(), nil
	}

	if decision.kind == retainedSummaryApplyOrdinary {
		return c.solveRetainedOrdinary(
			prepared, reader, build, owner, attempt, cacheKey, dependencyChanged,
			counter, transfers, dependencyChangeTransfers, attribution,
		)
	}

	tracked := newPointTrackingSummaryReader(c.registry(), reader)
	config := build(tracked)
	completeFlowDeps := true
	config.SummaryInputDigests = func() []uint64 {
		if decision.kind == retainedSummaryApplyRegional {
			deps, ok := completeRetainedFlowDependencies(owner, tracked)
			if !ok {
				completeFlowDeps = false
				return nil
			}
			return trackedSummaryReadDigests(c.registry(), deps)
		}
		return trackedSummaryReadDigests(c.registry(), tracked.base.deps)
	}
	attachPointSummaryTracking(&config, tracked.tracker)

	before := beginRetainedSummarySolve(config, counter)
	var result *body.Result
	var session *body.RetainedPreparedSession
	var pending *body.RetainedPreparedUpdate
	switch decision.kind {
	case retainedSummaryApplyBuild:
		result, session, err = body.SolvePreparedRetained(prepared, config.SolveConfig(), transfer.RetainedBudget{})
	case retainedSummaryApplyRegional:
		if owner.published == nil {
			err = fmt.Errorf("program: retained update has no published generation")
			break
		}
		var ok bool
		session, ok = owner.published.resource.(*body.RetainedPreparedSession)
		if !ok || session == nil {
			err = fmt.Errorf("program: retained update has incompatible generation")
			break
		}
		pending, err = session.BeginUpdate(prepared, config.SolveConfig(), decision.points, decision.forceFull)
		if err == nil {
			result = pending.Result()
		}
	case retainedSummaryApplyReproject:
		if owner.published == nil || owner.published.result == nil {
			err = fmt.Errorf("program: retained reprojection has no completed result")
			break
		}
		var ok bool
		session, ok = owner.published.resource.(*body.RetainedPreparedSession)
		if !ok || session == nil || !session.Retained() {
			err = fmt.Errorf("program: retained reprojection has no live generation")
			break
		}
		result = owner.published.result
	default:
		err = fmt.Errorf("program: unsupported retained application kind %d", decision.kind)
	}
	finishRetainedSummarySolve(config, before, dependencyChanged, counter, transfers, dependencyChangeTransfers, attribution)
	if err != nil {
		attempt.Abort()
		if pending != nil {
			pending.Abort()
		}
		if decision.kind == retainedSummaryApplyBuild && session != nil {
			session.Release()
		}
		if retainedOptimizationError(config, err) {
			return c.fallbackRetainedOrdinary(prepared, reader, build, owner, attempt, applicationKey, cacheKey, dependencyChanged, counter, transfers, dependencyChangeTransfers, attribution)
		}
		return summary.Summary{}, err
	}
	if !completeFlowDeps {
		if pending != nil {
			pending.Abort()
		}
		if session != nil && decision.kind == retainedSummaryApplyBuild {
			session.Release()
		}
		if result != nil {
			result.ReleaseTransient()
		}
		return c.fallbackRetainedOrdinary(prepared, reader, build, owner, attempt, applicationKey, cacheKey, dependencyChanged, counter, transfers, dependencyChangeTransfers, attribution)
	}
	if pending != nil {
		defer pending.Abort()
	}

	projected, result, projectErr := projectRetainedSummary(config, prepared, result, tracked.tracker, decision.kind == retainedSummaryApplyReproject)
	if projectErr != nil {
		attempt.Abort()
		if decision.kind == retainedSummaryApplyBuild && session != nil {
			session.Release()
		}
		if result != nil {
			result.ReleaseTransient()
		}
		return summary.Summary{}, projectErr
	}
	if pending != nil {
		if err := pending.Commit(); err != nil {
			attempt.Abort()
			if result != nil {
				result.ReleaseTransient()
			}
			if retainedOptimizationError(config, err) {
				return c.fallbackRetainedOrdinary(prepared, reader, build, owner, attempt, applicationKey, cacheKey, dependencyChanged, counter, transfers, dependencyChangeTransfers, attribution)
			}
			return summary.Summary{}, err
		}
	}

	// BeginUpdate deliberately falls back to a clean ordinary solve when its
	// stronger structural guard rejects reuse. Publish that result as ordinary
	// rather than preserving a released generation.
	if decision.kind != retainedSummaryApplyBuild && !session.Retained() {
		attempt.Abort()
		owner.dropPublished()
		attempt = owner.begin(applicationKey, reader)
		decision = attempt.Decision()
		if decision.kind != retainedSummaryApplyOrdinary {
			attempt.Abort()
			return summary.Summary{}, fmt.Errorf("program: structural fallback did not reset retained application")
		}
	}

	resource := retainedSummaryResource(nil)
	if decision.kind == retainedSummaryApplyBuild {
		resource = session
	}
	var previousResult *body.Result
	if owner.published != nil {
		previousResult = owner.published.result
	}
	if !attempt.publishResult(tracked.base.deps, tracked.dependencies(), resource, projected, result) {
		if resource != nil {
			resource.Release()
		}
		owner.dropPublished()
		if result != nil && result != previousResult {
			result.ReleaseTransient()
		}
		return projected, nil
	}
	// Publish the cross-unit summary only after body projection and retained
	// transaction publication both succeeded.
	c.write(cacheKey, owner.published.deps, projected)
	return projected, nil
}

func projectRetainedSummary(
	config body.Config,
	prepared *body.Static,
	result *body.Result,
	tracker *pointSummaryDependencyTracker,
	rebind bool,
) (summary.Summary, *body.Result, error) {
	tracker.beginProjection()
	defer tracker.endProjection()
	var err error
	if rebind {
		result, err = body.RebindBoundaryProviders(result, prepared, config.SolveConfig())
		if err != nil {
			return summary.Summary{}, result, err
		}
	}
	projected, err := summaryprojection.FromResultContext(config.Context, result)
	return projected, result, err
}

func (c *SummarySolveCache) fallbackRetainedOrdinary(
	prepared *body.Static,
	reader summary.Reader,
	build func(summary.Reader) body.Config,
	owner *retainedSummaryApplicationOwner,
	attempt *retainedSummaryApplicationAttempt,
	applicationKey retainedSummaryApplicationKey,
	cacheKey summarySolveCacheKey,
	dependencyChanged bool,
	counter, transfers, dependencyChangeTransfers *int,
	attribution *solveAttribution,
) (summary.Summary, error) {
	attempt.Abort()
	owner.dropPublished()
	ordinary := owner.begin(applicationKey, reader)
	if ordinary.Decision().kind != retainedSummaryApplyOrdinary {
		ordinary.Abort()
		return summary.Summary{}, fmt.Errorf("program: retained fallback did not reset application")
	}
	return c.solveRetainedOrdinary(prepared, reader, build, owner, ordinary, cacheKey, dependencyChanged, counter, transfers, dependencyChangeTransfers, attribution)
}

func retainedOptimizationError(config body.Config, err error) bool {
	if err == nil || contextErr(config.Context) != nil {
		return false
	}
	return !errors.Is(err, solve.ErrCanceled)
}

func (c *SummarySolveCache) solveRetainedOrdinary(
	prepared *body.Static,
	reader summary.Reader,
	build func(summary.Reader) body.Config,
	owner *retainedSummaryApplicationOwner,
	attempt *retainedSummaryApplicationAttempt,
	cacheKey summarySolveCacheKey,
	dependencyChanged bool,
	counter, transfers, dependencyChangeTransfers *int,
	attribution *solveAttribution,
) (summary.Summary, error) {
	tracked := &trackingSummaryReader{reg: c.registry(), base: reader}
	config := build(tracked)
	config.SummaryInputDigests = func() []uint64 {
		return trackedSummaryReadDigests(c.registry(), tracked.deps)
	}
	beforeTransfers := 0
	if config.Stats != nil {
		beforeTransfers = config.Stats.Transfer.Solver.TransferCalls
	}
	result, err := solvePreparedCountedWithTransfers(prepared, config, counter, transfers, func() *solveAttribution {
		if dependencyChanged {
			return attribution.afterDependencyChange()
		}
		return attribution
	}())
	if err != nil {
		attempt.Abort()
		return summary.Summary{}, err
	}
	projected, err := summaryprojection.FromResultContext(config.Context, result)
	if err != nil {
		attempt.Abort()
		result.ReleaseTransient()
		return summary.Summary{}, err
	}
	if dependencyChanged && dependencyChangeTransfers != nil && config.Stats != nil {
		*dependencyChangeTransfers += config.Stats.Transfer.Solver.TransferCalls - beforeTransfers
	}
	if !attempt.publishResult(tracked.deps, pointSummaryDependencies{}, nil, projected, result) {
		result.ReleaseTransient()
		return summary.Summary{}, fmt.Errorf("program: ordinary summary publication rejected")
	}
	c.write(cacheKey, tracked.deps, projected)
	return projected, nil
}

func completeRetainedFlowDependencies(
	owner *retainedSummaryApplicationOwner,
	tracked *pointTrackingSummaryReader,
) (map[summary.SummaryKey]trackedSummaryRead, bool) {
	if owner == nil || owner.published == nil || tracked == nil {
		return nil, false
	}
	update := tracked.dependencies()
	update.projection = nil
	update.projectionObserved = false
	points := owner.published.points.mergeUpdate(update)
	points.projection = nil
	points.projectionObserved = false
	return mergeRetainedSummaryDependencies(owner.published.deps, tracked.base.deps, points)
}

func beginRetainedSummarySolve(config body.Config, counter *int) int {
	if counter != nil {
		(*counter)++
	}
	if config.Stats == nil {
		return 0
	}
	return config.Stats.Transfer.Solver.TransferCalls
}

func finishRetainedSummarySolve(
	config body.Config,
	before int,
	dependencyChanged bool,
	counter, transfers, dependencyChangeTransfers *int,
	attribution *solveAttribution,
) {
	delta := 0
	if config.Stats != nil {
		delta = config.Stats.Transfer.Solver.TransferCalls - before
	}
	if transfers != nil {
		*transfers += delta
	}
	if dependencyChanged && dependencyChangeTransfers != nil {
		*dependencyChangeTransfers += delta
	}
	if attribution != nil {
		if dependencyChanged {
			attribution = attribution.afterDependencyChange()
		}
		attribution.stats.recordBodySolve(attribution, delta)
	}
	_ = counter // incremented by beginRetainedSummarySolve
}
