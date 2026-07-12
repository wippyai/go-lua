// Package relationcall composes frozen lexical Relations through the canonical
// Summary-to-CallOutcome adapter. It lives above both packages so the adapter
// remains independent of transformer implementation.
package relationcall

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/callresult"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// Target binds one exact Summary identity to its lexical relation cell.
// Keeping both identities in one routing proof prevents a relation from being
// accidentally adapted under another function's SummaryKey.
type Target struct {
	Cell       transformer.CellRef
	SummaryKey summary.SummaryKey
}

// TargetFor proves that one call site resolves to one exact lexical target.
// Dynamic/unresolved calls return false and remain concrete.
type TargetFor func(transfer.NodeContext, factflow.CallSiteView) (Target, bool)

// Bindings creates the dense caller-owned bindings for one frozen callee
// relation. It owns argument/path resolution and must fail closed when any
// required binding is unavailable.
type Bindings func(
	transfer.NodeContext,
	factflow.CallSiteView,
	state.State,
	func(cfg.Point) state.State,
	transformer.Shape,
) (transformer.BindingCursor, bool)

// Specialization binds caller-state-dependent symbolic evaluators for one
// relation application. Static relations need no factory. A relation carrying
// dynamic/member reads fails closed unless the factory supplies the canonical
// read resolver for this exact caller state.
type Specialization func(
	transfer.NodeContext,
	factflow.CallSiteView,
	state.State,
	func(cfg.Point) state.State,
) (transformer.SpecializationContext, bool)

type Config struct {
	Relations      transformer.RelationSnapshot
	TargetFor      TargetFor
	Bindings       Bindings
	Specialization Specialization
	Adapter        callresult.ProviderConfig
}

// OutcomeProvider is inactive infrastructure. Application performs no body
// solve: Relation.Specialize emits the existing Summary representation, which
// the production adapter consumes. Correlated rows are joined only by
// Relation.Specialize. Missing identity/bindings and contextual relations fail
// closed with an empty outcome.
func OutcomeProvider(config Config) callpayload.CallOutcomeProvider {
	transaction := callresult.NewPreparedSummaryTransaction(config.Adapter)
	return func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
		if config.TargetFor == nil || config.Bindings == nil {
			return callpayload.CallOutcome{}
		}
		target, ok := config.TargetFor(ctx, site)
		if !ok {
			return callpayload.CallOutcome{}
		}
		relation, ok := config.Relations.Lookup(target.Cell)
		if !ok || relation.ContextualReason() != "" {
			return callpayload.CallOutcome{}
		}
		cursor, ok := config.Bindings(ctx, site, in, read, relation.Shape())
		if !ok {
			return callpayload.CallOutcome{}
		}
		var specialization transformer.SpecializationContext
		if config.Specialization != nil {
			specialization, ok = config.Specialization(ctx, site, in, read)
			if !ok {
				return callpayload.CallOutcome{}
			}
		}
		sum, ok := relation.SpecializeWithContext(cursor, nil, specialization)
		if !ok {
			return callpayload.CallOutcome{}
		}
		return transaction.Apply(ctx, site, in, read, sum, transaction.FunctionType(target.SummaryKey), false)
	}
}
