package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// factEpoch owns the one cold Facts generation used by one active solver
// epoch.  It is deliberately not a State payload: completed queries must be
// materialized before publication, so a later carrier reformation can discard
// this schema, its guard universe, and every typed plane together.
type factEpoch struct {
	schema    *facts.Schema
	zero      facts.Facts
	entry     facts.Facts
	commits   []func()
	committed bool
}

// newFactEpoch freezes the complete heterogeneous Facts schema and constructs
// its canonical absent and entry roots. Factors install their typed empty
// planes in declared schema order; Facts owns the shared Boolean support once.
// `zero` has false support, so it cannot seed an arbitrary coordinate; `entry`
// has true support and is selected only by the root-ingress rule. No Factor,
// rule, or transaction may add a plane after this cut.
func newFactEpoch(guards *guard.Manager, declarations []factorDeclaration) (*factEpoch, bool) {
	if guards == nil || len(declarations) == 0 {
		return nil, false
	}
	schema := facts.NewSchema(guards)
	if schema == nil {
		return nil, false
	}
	staged := make([]stagedFactorBinding, len(declarations))
	for index := range declarations {
		if declarations[index].stageFacts == nil {
			return nil, false
		}
		var ok bool
		staged[index], ok = declarations[index].stageFacts(schema, guards)
		if !ok || staged[index].initial == nil || staged[index].commit == nil {
			return nil, false
		}
	}
	if !schema.Seal() {
		return nil, false
	}
	regions := support.New(guards)
	if regions == nil {
		return nil, false
	}
	zeroRegion, entryRegion := regions.False(), regions.True()
	if !regions.Seal() {
		regions.Discard()
		return nil, false
	}
	zero, ok := schema.New(zeroRegion)
	if !ok {
		return nil, false
	}
	entry, ok := schema.New(entryRegion)
	if !ok {
		return nil, false
	}
	commits := make([]func(), len(staged))
	for index := range staged {
		zero, ok = staged[index].initial(zero)
		if !ok {
			return nil, false
		}
		entry, ok = staged[index].initial(entry)
		if !ok {
			return nil, false
		}
		commits[index] = staged[index].commit
	}
	if !schema.Valid(zero) || !schema.Valid(entry) {
		return nil, false
	}
	return &factEpoch{schema: schema, zero: zero, entry: entry, commits: commits}, true
}

// valid accepts only this active generation's complete immutable root.  It is
// intentionally private: State never needs a schema lookup after query
// materialization, and no historical-generation fallback is permitted.
func (epoch *factEpoch) valid(root facts.Facts) bool {
	return epoch != nil && epoch.schema != nil && epoch.schema.Valid(root)
}

// commit adopts every already-built typed plane at the single successful
// epoch-acceptance cut. All closures are validated before any mutation and
// are infallible after staging, so the cut cannot partially adopt a Factor.
// There is intentionally no rollback or alternate binding: a failed
// candidate is simply never committed.
func (epoch *factEpoch) commit() bool {
	if epoch == nil || !epoch.valid(epoch.zero) || !epoch.valid(epoch.entry) || epoch.committed {
		return epoch != nil && epoch.committed
	}
	for _, commit := range epoch.commits {
		if commit == nil {
			return false
		}
	}
	for _, commit := range epoch.commits {
		commit()
	}
	epoch.committed = true
	return true
}
