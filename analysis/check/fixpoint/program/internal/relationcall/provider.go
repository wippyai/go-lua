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
	Catalog        *Catalog
	TargetFor      TargetFor
	Bindings       Bindings
	Specialization Specialization
	EffectResolver transformer.EffectSummaryResolver
	Adapter        callresult.ProviderConfig
}

// TryOutcomeProvider distinguishes an exact handled application from a
// fail-closed miss. Call routing uses this boolean to choose either Relation or
// the legacy provider exactly once; an empty CallOutcome is not itself a miss.
type TryOutcomeProvider func(
	transfer.NodeContext,
	factflow.CallSiteView,
	state.State,
	func(cfg.Point) state.State,
) (callpayload.CallOutcome, bool)

// Resolver is the handled-aware relation call seam.
type Resolver = TryOutcomeProvider

// Exclusive gives one handled-aware resolver first and exclusive ownership of
// a call. A miss invokes fallback exactly once. In particular, a handled empty
// outcome is authoritative and never falls through to the legacy provider.
func Exclusive(resolver Resolver, fallback callpayload.CallOutcomeProvider) callpayload.CallOutcomeProvider {
	return func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
		if resolver != nil {
			if out, handled := resolver(ctx, site, in, read); handled {
				return out
			}
		}
		if fallback != nil {
			return fallback(ctx, site, in, read)
		}
		return callpayload.CallOutcome{}
	}
}

// OutcomeProvider is inactive infrastructure. Application performs no body
// solve: Relation.Specialize emits the existing Summary representation, which
// the production adapter consumes. Correlated rows are joined only by
// Relation.Specialize. Missing identity/bindings and contextual relations fail
// closed with an empty outcome.
func OutcomeProvider(config Config) callpayload.CallOutcomeProvider {
	try := NewResolver(config)
	return func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
		out, _ := try(ctx, site, in, read)
		return out
	}
}

// NewResolver prepares an immutable handled-aware relation call resolver.
func NewResolver(config Config) Resolver {
	transaction := callresult.NewPreparedSummaryTransaction(config.Adapter)
	return func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) (callpayload.CallOutcome, bool) {
		if config.Bindings == nil {
			return callpayload.CallOutcome{}, false
		}
		target, ok := resolveTarget(config, ctx, site)
		if !ok {
			return callpayload.CallOutcome{}, false
		}
		relation, ok := config.Relations.Lookup(target.Cell)
		if !ok || relation.ContextualReason() != "" || relation.Widened() {
			return callpayload.CallOutcome{}, false
		}
		cursor, ok := config.Bindings(ctx, site, in, read, relation.Shape())
		if !ok {
			return callpayload.CallOutcome{}, false
		}
		var specialization transformer.SpecializationContext
		if config.Specialization != nil {
			specialization, ok = config.Specialization(ctx, site, in, read)
			if !ok {
				return callpayload.CallOutcome{}, false
			}
		}
		sum, ok := relation.SpecializeWithEffects(cursor, nil, specialization, config.EffectResolver)
		if !ok {
			return callpayload.CallOutcome{}, false
		}
		return transaction.Apply(ctx, site, in, read, sum, transaction.FunctionType(target.SummaryKey), false), true
	}
}

func resolveTarget(config Config, ctx transfer.NodeContext, site factflow.CallSiteView) (Target, bool) {
	if config.Catalog != nil {
		point, ok := site.Point()
		if !ok {
			return Target{}, false
		}
		return config.Catalog.Lookup(point)
	}
	if config.TargetFor == nil {
		return Target{}, false
	}
	return config.TargetFor(ctx, site)
}

// TryOutcome is the exclusive routing seam. False guarantees a zero outcome
// and means the caller may invoke its fallback; true means the Relation path
// owns this call even when its exact semantic outcome is empty.
func TryOutcome(config Config) TryOutcomeProvider {
	return NewResolver(config)
}
