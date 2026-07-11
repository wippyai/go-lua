package body

import (
	"context"

	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	internalhash "github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// RetainedPreparedSession owns one prepared body's retained ascending solve.
// It is deliberately opaque: equation provenance and revisions remain owned
// by transfer/solve, while body owns the stable semantic identity guard.
type RetainedPreparedSession struct {
	transfer            *transfer.RetainedSession
	bodyIdentity        uint64
	provisionalIdentity uint64
	released            bool
}

// SolvePreparedRetained performs an ordinary body publication while retaining
// the pre-narrowing WTO generation for later regional updates. It is opt-in;
// SolvePrepared remains unchanged.
func SolvePreparedRetained(prepared *Static, config SolveConfig, budget transfer.RetainedBudget) (*Result, *RetainedPreparedSession, error) {
	if prepared == nil {
		return nil, nil, ErrStaticRequired
	}
	bodyIdentity, provisionalIdentity, err := retainedIdentities(prepared, config)
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
	result, err := prepared.solveWithFlow(config, func(transferConfig transfer.Config) (*bodyFlowTransaction, error) {
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
		transfer: retained, bodyIdentity: bodyIdentity,
		provisionalIdentity: provisionalIdentity,
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
	bodyIdentity, provisionalIdentity, err := retainedIdentities(prepared, config)
	if err != nil {
		return nil, err
	}
	if r == nil || r.released || r.transfer == nil || r.bodyIdentity != bodyIdentity || r.provisionalIdentity != provisionalIdentity {
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
	result, err := prepared.solveWithFlow(config, func(transferConfig transfer.Config) (*bodyFlowTransaction, error) {
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
	r.bodyIdentity, r.provisionalIdentity = 0, 0
}

// Retained reports whether the session still owns a reusable generation.
func (r *RetainedPreparedSession) Retained() bool {
	return r != nil && !r.released && r.transfer != nil
}

func retainedIdentities(prepared *Static, config SolveConfig) (uint64, uint64, error) {
	ctx := config.Context
	if ctx == nil {
		ctx = context.Background()
	}
	bodyIdentity, err := prepared.IdentityDigestContext(ctx)
	if err != nil {
		return 0, 0, err
	}
	// Summary contents are the replaceable dynamic binding. Their discovered
	// digests are validated by the program owner and intentionally excluded from
	// the provisional equation identity used here.
	provisional := config
	provisional.SummaryInputDigests = nil
	typeValues := prepared.solveTypeValues(provisional)
	entry, initial := prepared.solveEntryState(typeValues, provisional.EntryState, provisional.Initial)
	base, err := computeResultVersion(prepared, provisional, entry, initial)
	if err != nil {
		return 0, 0, err
	}
	w := internalhash.NewWriter()
	w.WriteUintDecimal(base)
	if provisional.WidenAt == nil {
		_ = w.WriteByte('N')
	} else {
		_ = w.WriteByte('W')
		for _, point := range prepared.cfg.Graph.RPO() {
			w.WriteBool(provisional.WidenAt(point))
		}
	}
	if provisional.WidenDelay == nil {
		_ = w.WriteByte('N')
	} else {
		_ = w.WriteByte('D')
		for _, point := range prepared.cfg.Graph.RPO() {
			w.WriteIntDecimal(int64(provisional.WidenDelay(point)))
		}
	}
	return bodyIdentity, w.Sum64(), nil
}
