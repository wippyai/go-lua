package transfer

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type retainedBudget = solve.RetainedBudget

// retainedCleanGeneration keeps the canonical equation identity beside an
// opaque solve-owned retained generation. State-equation provenance stays in
// solve, which is the only layer that owns reads, emissions, revisions, and
// WTO history. The final narrowed Result is returned to the caller and is not
// duplicated here.
type retainedCleanGeneration struct {
	plan          equationPlan
	identity      equationPlanIdentity
	policy        equationSolverPolicy
	retained      *solve.RetainedSystem[cfg.Point, state.State]
	finalVersions map[cfg.Point]uint64
	released      bool
}

func (g *retainedCleanGeneration) Release() {
	if g == nil || g.released {
		return
	}
	g.released = true
	if g.retained != nil {
		g.retained.Release()
	}
	g.retained = nil
	g.plan = equationPlan{}
	g.finalVersions = nil
}

// tryRunRetainedClean is package-internal and opt-in. Default TryRun never
// calls it, so the ordinary lint path retains the exact canonical closures and
// pays no provenance branches or allocations.
func tryRunRetainedClean(config Config, budget retainedBudget) (Result, *retainedCleanGeneration, error) {
	if config.Session == nil {
		config.Context, config.Session = cancellation.Attach(config.Context)
	} else {
		config.Context = cancellation.WithSession(config.Context, config.Session)
	}
	if err := config.Session.Token().Err(); err != nil {
		return nil, nil, errors.Join(solve.ErrCanceled, err)
	}
	if err := validateConfig(config); err != nil {
		return nil, nil, err
	}
	domain, err := state.TryDomainWithOptionalLanesAndOptions(config.Registry, config.StateLanes, config.StateOptions)
	if err != nil {
		return nil, nil, err
	}
	plan := newEquationPlan(config, domain, equationPlanHooks{})
	result, versions, retained, err := solve.BuildRetainedWTO(config.Context, plan.system, plan.wto, budget)
	if err != nil {
		return nil, nil, err
	}
	generation := &retainedCleanGeneration{
		plan: plan, identity: plan.identity, policy: plan.solverPolicy,
		retained: retained, finalVersions: versions,
	}
	return Result(result), generation, nil
}
