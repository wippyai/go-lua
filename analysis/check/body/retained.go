package body

import (
	"context"
	"errors"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// RetainedPreparedSession owns one prepared body's retained ascending solve.
// It is deliberately opaque: equation provenance and revisions remain owned
// by transfer/solve, while body owns the exact run-local Static and normalized
// structural-input witness. Call/argument/summary providers are replaceable
// dynamic bindings: the program owner validates their dependencies and names
// the affected CFG points before a regional update.
type RetainedPreparedSession struct {
	transfer            *transfer.RetainedSession
	prepared            *Static
	bodyIdentity        uint64
	provisionalIdentity uint64
	inputWitness        retainedInputWitness
	released            bool
}

// SolvePreparedRetained performs an ordinary body publication while retaining
// the pre-narrowing WTO generation for later regional updates. It is opt-in;
// SolvePrepared remains unchanged.
func SolvePreparedRetained(prepared *Static, config SolveConfig, budget transfer.RetainedBudget) (*Result, *RetainedPreparedSession, error) {
	if prepared == nil {
		return nil, nil, ErrStaticRequired
	}
	bodyIdentity, provisionalIdentity, inputWitness, frozenConfig, err := retainedIdentities(prepared, config)
	if err != nil {
		return nil, nil, err
	}
	var retained *transfer.RetainedSession
	defer func() {
		if recovered := recover(); recovered != nil {
			if retained != nil {
				retained.Release()
			}
			panic(recovered)
		}
	}()
	result, err := prepared.solveWithFlow(frozenConfig, func(transferConfig transfer.Config) (*bodyFlowTransaction, error) {
		flow, session, runErr := transfer.TryRunRetained(transferConfig, budget)
		if runErr != nil {
			return nil, runErr
		}
		retained = session
		return &bodyFlowTransaction{flow: flow, abortFn: session.Release}, nil
	})
	if err != nil {
		return nil, nil, err
	}
	return result, &RetainedPreparedSession{
		transfer: retained, prepared: prepared, bodyIdentity: bodyIdentity,
		provisionalIdentity: provisionalIdentity,
		inputWitness:        inputWitness,
	}, nil
}

// RetainedPreparedUpdate holds a fully validated body Result whose retained
// ascent is still transaction-owned. Program-level summary projection may
// inspect Result before choosing Commit or Abort. Every candidate must be
// finished exactly once; callers normally defer Abort until Commit succeeds.
type RetainedPreparedUpdate struct {
	result *Result
	update *transfer.RetainedUpdate
	done   bool
}

func (u *RetainedPreparedUpdate) Result() *Result {
	if u == nil {
		return nil
	}
	return u.result
}

func (u *RetainedPreparedUpdate) Commit() error {
	if u == nil || u.done {
		return solve.ErrUpdateState
	}
	if u.update != nil {
		if err := u.update.Commit(); err != nil {
			return err
		}
	}
	u.done = true
	return nil
}

func (u *RetainedPreparedUpdate) Abort() {
	if u == nil || u.done {
		return
	}
	if u.update != nil {
		u.update.Abort()
	}
	u.done = true
	u.result = nil
}

// BeginUpdate re-solves the affected CFG region with new dynamic summary bindings.
// A stable body/input mismatch releases retention and falls back to the exact
// clean SolvePrepared path. No initial-state or structural equation input is
// ever replaced inside an existing retained generation.
func (r *RetainedPreparedSession) BeginUpdate(prepared *Static, config SolveConfig, changed []cfg.Point, forceFull bool) (*RetainedPreparedUpdate, error) {
	if prepared == nil {
		return nil, ErrStaticRequired
	}
	bodyIdentity, provisionalIdentity, inputWitness, frozenConfig, err := retainedIdentities(prepared, config)
	if err != nil {
		return nil, err
	}
	if r == nil || r.released || r.transfer == nil || r.prepared != prepared ||
		!r.inputWitness.equal(prepared.registry, inputWitness) ||
		r.bodyIdentity != bodyIdentity || r.provisionalIdentity != provisionalIdentity {
		if r != nil {
			r.Release()
		}
		result, solveErr := SolvePrepared(prepared, config)
		if solveErr != nil {
			return nil, solveErr
		}
		return &RetainedPreparedUpdate{result: result}, nil
	}
	var pending *transfer.RetainedUpdate
	defer func() {
		if recovered := recover(); recovered != nil {
			if pending != nil {
				pending.Abort()
			}
			panic(recovered)
		}
	}()
	result, err := prepared.solveWithFlow(frozenConfig, func(transferConfig transfer.Config) (*bodyFlowTransaction, error) {
		update, beginErr := r.transfer.BeginUpdate(transferConfig, changed, forceFull)
		if beginErr != nil {
			return nil, beginErr
		}
		pending = update
		flow, runErr := update.Run()
		if runErr != nil {
			return nil, runErr
		}
		// Body publication marks this wrapper complete only after observation
		// sealing and ResultVersion. The underlying transfer transaction remains
		// pending for its program-level projection owner.
		return &bodyFlowTransaction{flow: flow, abortFn: update.Abort}, nil
	})
	if err != nil {
		return nil, err
	}
	return &RetainedPreparedUpdate{result: result, update: pending}, nil
}

// Release drops every retained state and transfer reference. It is idempotent.
func (r *RetainedPreparedSession) Release() {
	if r == nil || r.released {
		return
	}
	r.released = true
	if r.transfer != nil {
		r.transfer.Release()
	}
	r.transfer = nil
	r.prepared = nil
	r.bodyIdentity, r.provisionalIdentity = 0, 0
	r.inputWitness = retainedInputWitness{}
}

// Retained reports whether the session still owns a reusable generation.
func (r *RetainedPreparedSession) Retained() bool {
	return r != nil && !r.released && r.transfer != nil
}

// StructurallyCompatible reports whether config describes the same immutable
// equation system and boundary state as the retained generation. It is not an
// adoption verdict: dynamic semantic providers and summary payloads are
// deliberately excluded, so the program owner must separately prove the exact
// normalized dynamic-binding/dependency snapshot before reusing any result.
//
// Unlike BeginUpdate, StructurallyCompatible is observational. A mismatch
// never releases the retained generation and never starts a clean fallback.
func (r *RetainedPreparedSession) StructurallyCompatible(prepared *Static, config SolveConfig) (bool, error) {
	if r == nil || r.released || r.transfer == nil || prepared == nil {
		return false, nil
	}
	bodyIdentity, provisionalIdentity, inputWitness, _, err := retainedIdentities(prepared, config)
	if err != nil {
		return false, err
	}
	return r.prepared == prepared && r.inputWitness.equal(prepared.registry, inputWitness) &&
		r.bodyIdentity == bodyIdentity &&
		r.provisionalIdentity == provisionalIdentity, nil
}

type retainedInputWitness struct {
	lanes         []state.LaneID
	entry         state.State
	initial       []retainedInitialState
	schedule      transfer.Schedule
	hasWidenAt    bool
	widenAt       []bool
	hasWidenDelay bool
	widenDelay    []int
	closedDynamic []factapply.ClosedDynamicAllValueInvariant
	typeValues    *typevalue.Cache
}

type retainedInitialState struct {
	point cfg.Point
	state state.State
}

func (w retainedInputWitness) equal(reg *axis.Registry, other retainedInputWitness) bool {
	if reg == nil || len(w.lanes) != len(other.lanes) {
		return false
	}
	for i := range w.lanes {
		if w.lanes[i] != other.lanes[i] {
			return false
		}
	}
	domain, err := state.TryDomainWithOptionalLanes(reg, w.lanes)
	if err != nil || !domain.Equal(w.entry, other.entry) || len(w.initial) != len(other.initial) {
		return false
	}
	for i := range w.initial {
		if w.initial[i].point != other.initial[i].point || !domain.Equal(w.initial[i].state, other.initial[i].state) {
			return false
		}
	}
	if w.schedule != other.schedule || w.hasWidenAt != other.hasWidenAt ||
		w.hasWidenDelay != other.hasWidenDelay ||
		len(w.widenAt) != len(other.widenAt) || len(w.widenDelay) != len(other.widenDelay) {
		return false
	}
	for i := range w.widenAt {
		if w.widenAt[i] != other.widenAt[i] {
			return false
		}
	}
	for i := range w.widenDelay {
		if w.widenDelay[i] != other.widenDelay[i] {
			return false
		}
	}
	if len(w.closedDynamic) != len(other.closedDynamic) || w.typeValues != other.typeValues {
		return false
	}
	matched := make([]bool, len(other.closedDynamic))
	for _, left := range w.closedDynamic {
		found := false
		for i, right := range other.closedDynamic {
			if matched[i] || !left.Container.Equal(right.Container) || !left.Table.Equal(right.Table) {
				continue
			}
			matched[i] = true
			found = true
			break
		}
		if !found {
			return false
		}
	}
	return true
}

func retainedIdentities(prepared *Static, config SolveConfig) (uint64, uint64, retainedInputWitness, SolveConfig, error) {
	frozen, err := freezeRetainedSolveConfig(prepared, config)
	if err != nil {
		return 0, 0, retainedInputWitness{}, SolveConfig{}, err
	}
	ctx := frozen.Context
	if ctx == nil {
		ctx = context.Background()
	}
	bodyIdentity, err := prepared.IdentityDigestContext(ctx)
	if err != nil {
		return 0, 0, retainedInputWitness{}, SolveConfig{}, err
	}
	// Summary contents are the replaceable dynamic binding. Their discovered
	// digests are validated by the program owner and intentionally excluded from
	// the provisional equation identity used here.
	provisional := frozen
	provisional.SummaryInputDigests = nil
	typeValues := prepared.solveTypeValues(provisional)
	entry, initial := prepared.solveEntryState(typeValues, provisional.EntryState, provisional.Initial)
	witness, capturedInitial, err := captureRetainedInputWitness(prepared, provisional, typeValues, entry, initial)
	if err != nil {
		return 0, 0, retainedInputWitness{}, SolveConfig{}, err
	}
	digestConfig := provisional
	pointOrdinals := make(map[cfg.Point]int, len(witness.widenAt)+len(witness.widenDelay))
	for ordinal, point := range prepared.cfg.Graph.RPO() {
		pointOrdinals[point] = ordinal
	}
	if witness.hasWidenAt {
		digestConfig.WidenAt = func(point cfg.Point) bool {
			return witness.widenAt[pointOrdinals[point]]
		}
	}
	if witness.hasWidenDelay {
		digestConfig.WidenDelay = func(point cfg.Point) int {
			return witness.widenDelay[pointOrdinals[point]]
		}
	}
	base, err := computeResultVersion(prepared, digestConfig, witness.entry, capturedInitial)
	if err != nil {
		return 0, 0, retainedInputWitness{}, SolveConfig{}, err
	}
	return bodyIdentity, base, witness, frozen, nil
}

// freezeRetainedSolveConfig evaluates every structural callback exactly once
// per CFG point and replaces it with an immutable replay provider. The same
// frozen config is then used for identity/witness construction and the actual
// retained solve, eliminating callback TOCTOU between those phases.
func freezeRetainedSolveConfig(prepared *Static, config SolveConfig) (SolveConfig, error) {
	if config.Resume != nil || config.ResumePoints != nil {
		return SolveConfig{}, ErrRetainedResume
	}
	if err := retainedFreezeContextError(config.Context); err != nil {
		return SolveConfig{}, err
	}
	frozen := config
	frozen.EntryState = config.EntryState.Snapshot()
	frozen.StateLanes = state.CloneLanes(config.StateLanes)
	frozen.ResumePoints = append([]cfg.Point(nil), config.ResumePoints...)
	frozen.ClosedDynamicAllValues = cloneClosedDynamicInvariants(config.ClosedDynamicAllValues)
	points := prepared.cfg.Graph.RPO()

	if config.Initial != nil {
		initial := make(map[cfg.Point]state.State)
		for _, point := range points {
			if err := retainedFreezeContextError(config.Context); err != nil {
				return SolveConfig{}, err
			}
			st, ok := config.Initial(point)
			if err := retainedFreezeContextError(config.Context); err != nil {
				return SolveConfig{}, err
			}
			if ok {
				initial[point] = st.Snapshot()
			}
		}
		frozen.Initial = func(point cfg.Point) (state.State, bool) {
			st, ok := initial[point]
			return st, ok
		}
	}

	if config.WidenAt != nil {
		widenAt := make(map[cfg.Point]bool, len(points))
		for _, point := range points {
			if err := retainedFreezeContextError(config.Context); err != nil {
				return SolveConfig{}, err
			}
			widenAt[point] = config.WidenAt(point)
			if err := retainedFreezeContextError(config.Context); err != nil {
				return SolveConfig{}, err
			}
		}
		frozen.WidenAt = func(point cfg.Point) bool { return widenAt[point] }
	}
	if config.WidenDelay != nil {
		widenDelay := make(map[cfg.Point]int, len(points))
		for _, point := range points {
			if err := retainedFreezeContextError(config.Context); err != nil {
				return SolveConfig{}, err
			}
			widenDelay[point] = config.WidenDelay(point)
			if err := retainedFreezeContextError(config.Context); err != nil {
				return SolveConfig{}, err
			}
		}
		frozen.WidenDelay = func(point cfg.Point) int { return widenDelay[point] }
	}
	return frozen, nil
}

func retainedFreezeContextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(solve.ErrCanceled, err)
	}
	return nil
}

func cloneClosedDynamicInvariants(in []factapply.ClosedDynamicAllValueInvariant) []factapply.ClosedDynamicAllValueInvariant {
	if in == nil {
		return nil
	}
	out := make([]factapply.ClosedDynamicAllValueInvariant, len(in))
	for i, invariant := range in {
		out[i] = invariant
		out[i].Container.Segments = append([]segment.Segment(nil), invariant.Container.Segments...)
		out[i].Table.Segments = append([]segment.Segment(nil), invariant.Table.Segments...)
	}
	return out
}

func captureRetainedInputWitness(
	prepared *Static,
	config SolveConfig,
	typeValues *typevalue.Cache,
	entry state.State,
	initial transfer.InitialState,
) (retainedInputWitness, transfer.InitialState, error) {
	domain, err := state.TryDomainWithOptionalLanes(prepared.registry, config.StateLanes)
	if err != nil {
		return retainedInputWitness{}, nil, err
	}
	canonicalLanes := state.CloneLanes(config.StateLanes)
	if canonicalLanes == nil {
		canonicalLanes = state.DefaultLanes()
	} else {
		selected := state.NewLaneSet(canonicalLanes...)
		ordered := make([]state.LaneID, 0, selected.Len())
		for _, lane := range state.DefaultLanes() {
			if selected.Has(lane) {
				ordered = append(ordered, lane)
			}
		}
		canonicalLanes = ordered
	}
	witness := retainedInputWitness{
		lanes:         canonicalLanes,
		entry:         state.NormalizeForDomain(domain, entry),
		schedule:      config.Schedule,
		hasWidenAt:    config.WidenAt != nil,
		hasWidenDelay: config.WidenDelay != nil,
		typeValues:    typeValues,
	}
	witness.closedDynamic = cloneClosedDynamicInvariants(config.ClosedDynamicAllValues)
	for _, point := range prepared.cfg.Graph.RPO() {
		if config.WidenAt != nil {
			witness.widenAt = append(witness.widenAt, config.WidenAt(point))
		}
		if config.WidenDelay != nil {
			witness.widenDelay = append(witness.widenDelay, config.WidenDelay(point))
		}
	}
	if initial != nil {
		for _, point := range prepared.cfg.Graph.RPO() {
			st, ok := initial(point)
			if !ok {
				continue
			}
			witness.initial = append(witness.initial, retainedInitialState{
				point: point,
				state: state.NormalizeForDomain(domain, st),
			})
		}
	}
	if len(witness.initial) == 0 {
		return witness, nil, nil
	}
	byPoint := make(map[cfg.Point]state.State, len(witness.initial))
	for _, item := range witness.initial {
		byPoint[item.point] = item.state
	}
	return witness, func(point cfg.Point) (state.State, bool) {
		st, ok := byPoint[point]
		return st, ok
	}, nil
}
