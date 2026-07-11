package transfer

import (
	"context"
	"errors"

	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// RetainedBudget bounds the provenance owned by an opt-in retained solve.
// Zero fields are unlimited.
type RetainedBudget = solve.RetainedBudget

// ErrRetainedRequiresWTO reports an attempt to retain a schedule whose
// ascending generation does not have WTO provenance.
var ErrRetainedRequiresWTO = errors.New("transfer: retained solve requires WTO schedule")

// retainedCleanGeneration keeps the canonical equation identity beside an
// opaque solve-owned retained generation. State-equation provenance stays in
// solve, which is the only layer that owns reads, emissions, revisions, and
// WTO history. The final narrowed Result is returned to the caller and is not
// duplicated here.
type RetainedSession struct {
	plan          equationPlan
	identity      equationPlanIdentity
	policy        equationSolverPolicy
	retained      *solve.RetainedSystem[cfg.Point, state.State]
	finalVersions map[cfg.Point]uint64
	released      bool
}

func (g *RetainedSession) Release() {
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

// TryRunRetained is opt-in. Default TryRun never calls it, so the ordinary lint
// path retains the exact canonical closures and pays no provenance branches or
// allocations.
func TryRunRetained(config Config, budget RetainedBudget) (Result, *RetainedSession, error) {
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
	if config.Schedule != ScheduleWTO {
		return nil, nil, ErrRetainedRequiresWTO
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
	if config.FinalizeNodeObservations != nil {
		config.FinalizeNodeObservations(func(point cfg.Point) uint64 { return versions[point] })
	}
	generation := &RetainedSession{
		plan: plan, identity: plan.identity, policy: plan.solverPolicy,
		retained: retained, finalVersions: versions,
	}
	return Result(result), generation, nil
}

// RetainedUpdate is a transaction over a retained ascending generation. Run
// and Result never mutate the session. Commit is the only publication point;
// Abort leaves the prior generation reusable.
type RetainedUpdate struct {
	update   *solve.Update[cfg.Point, state.State]
	context  context.Context
	finalize func(func(cfg.Point) uint64)
	result   Result
	versions map[cfg.Point]uint64
	done     bool
}

// BeginUpdate replaces only the dynamic equation bindings in config. Static
// equation inputs (initial states, lattice, widening policy and WTO plan) stay
// owned by the retained session. The body layer is responsible for validating
// its stable prepared-body and provisional-input identities before calling.
func (g *RetainedSession) BeginUpdate(config Config, changed []cfg.Point, forceFull bool) (*RetainedUpdate, error) {
	if g == nil || g.released || g.retained == nil {
		return nil, solve.ErrRetainedReleased
	}
	if config.Session == nil {
		config.Context, config.Session = cancellation.Attach(config.Context)
	} else {
		config.Context = cancellation.WithSession(config.Context, config.Session)
	}
	if err := config.Session.Token().Err(); err != nil {
		return nil, errors.Join(solve.ErrCanceled, err)
	}
	if err := validateConfig(config); err != nil {
		return nil, err
	}
	domain, err := state.TryDomainWithOptionalLanesAndOptions(config.Registry, config.StateLanes, config.StateOptions)
	if err != nil {
		return nil, err
	}
	replacement := newEquationPlan(config, domain, equationPlanHooks{})
	if replacement.solverPolicy != g.policy {
		return nil, solve.ErrUpdateState
	}
	update, err := g.retained.BeginUpdate(changed, replacement.system.Transfer, replacement.system.TransferVersioned)
	if err != nil {
		return nil, err
	}
	if err := update.SetStats(replacement.system.Stats); err != nil {
		update.Abort()
		return nil, err
	}
	if forceFull {
		update.RequireFullFallback()
	}
	return &RetainedUpdate{update: update, context: config.Context, finalize: config.FinalizeNodeObservations}, nil
}

// Run converges and narrows transaction-owned scratch state. On every error it
// publishes no Result and leaves the retained session unchanged.
func (u *RetainedUpdate) Run() (Result, error) {
	if u == nil || u.done || u.update == nil {
		return nil, solve.ErrUpdateState
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			u.Abort()
			panic(recovered)
		}
	}()
	if err := u.update.Run(u.context); err != nil {
		u.Abort()
		return nil, err
	}
	values, versions, err := u.update.Publish(u.context)
	if err != nil {
		u.Abort()
		return nil, err
	}
	u.result, u.versions = Result(values), versions
	if u.finalize != nil {
		u.finalize(func(point cfg.Point) uint64 { return versions[point] })
	}
	return u.result, nil
}

// Commit atomically advances the retained generation after the owning body has
// completed observation sealing and result validation.
func (u *RetainedUpdate) Commit() error {
	if u == nil || u.done || u.update == nil || u.result == nil {
		return solve.ErrUpdateState
	}
	if err := u.update.Commit(); err != nil {
		return err
	}
	u.done = true
	u.result, u.versions = nil, nil
	return nil
}

// Abort discards transaction scratch and preserves the prior generation.
func (u *RetainedUpdate) Abort() {
	if u == nil || u.done {
		return
	}
	if u.update != nil {
		u.update.Abort()
	}
	u.done = true
	u.result, u.versions = nil, nil
}
