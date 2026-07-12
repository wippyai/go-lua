package transformer

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
)

// ResolvedPathInvalidation is a caller-bound invalidation request. It contains
// values and paths only; applying it to caller State remains the canonical call
// adapter's responsibility.
type ResolvedPathInvalidation struct {
	Target                          pathdom.Path
	Scope                           InvalidationScope
	PreserveStructuralWitness       bool
	PreserveDynamicValueMemberships bool
	Precise                         *ResolvedPreciseDynamicTarget
}

type ResolvedPreciseDynamicTarget struct {
	Table  pathdom.Path
	Key    product.Value
	Suffix []segment.Segment
}

// ResolvedIndexMutation is the complete caller-bound symbolic transaction.
// AppendCandidate is not an append proof: a resolver must preserve the
// concrete engine's canonical length-expression gate before emitting facts.
type ResolvedIndexMutation struct {
	Invalidation    ResolvedPathInvalidation
	Table           pathdom.Path
	Key             product.Value
	Value           product.Value
	KeyPath         pathdom.Path
	ValuePath       pathdom.Path
	Admission       dynamicindex.Admission
	Readback        factflow.DynamicIndexReadbackIntent
	AppendCandidate bool
	Site            EffectSite
}

// ResolvedEffect is one tagged transaction. Exactly the payload matching Kind
// is populated.
type ResolvedEffect struct {
	Kind         EffectKind
	Invalidation ResolvedPathInvalidation
	Mutation     ResolvedIndexMutation
	Allocation   ResolvedAllocationTemplate
}

// ResolvedAllocationTemplate is the specialization handoff for one shared
// allocation node. Result and heap/fresh lowering must use this same site and
// template identity.
type ResolvedAllocationTemplate struct {
	Site     operationplan.SignatureAllocationSite
	Template operationplan.SignatureAllocationOperation
	Result   product.Value
}

// EffectSummaryResolver lowers one complete correlated row of resolved effects
// into a transaction-local fragment of the existing Summary representation.
// Returning false rejects the entire relation specialization. The callback is
// deliberately unable to mutate caller State.
type EffectSummaryResolver func([]ResolvedEffect) (summary.Summary, bool)

func (a *EffectArena) resolve(term EffectTerm, cursor BindingCursor, context SpecializationContext) (ResolvedEffect, bool) {
	if a == nil || a.terms == nil || term == 0 || int(term) >= len(a.nodes) {
		return ResolvedEffect{}, false
	}
	node := a.nodes[term]
	if node.kind == EffectAllocationTemplate {
		if !a.terms.validAllocation(node.allocation) {
			return ResolvedEffect{}, false
		}
		op := a.terms.allocations[node.allocation].op
		result, ok := a.terms.allocationResult(node.allocation, op.Template().ReturnIndex)
		if !ok {
			return ResolvedEffect{}, false
		}
		return ResolvedEffect{Kind: node.kind, Allocation: ResolvedAllocationTemplate{Site: op.Site(), Template: op, Result: result}}, true
	}
	invalidation, ok := a.resolveInvalidation(node.invalidation, cursor, context)
	if !ok {
		return ResolvedEffect{}, false
	}
	if node.kind == EffectInvalidatePath {
		return ResolvedEffect{Kind: node.kind, Invalidation: invalidation}, true
	}
	if node.kind != EffectIndexMutation {
		return ResolvedEffect{}, false
	}
	table, ok := a.terms.evalPath(node.table, cursor)
	if !ok {
		return ResolvedEffect{}, false
	}
	key, ok := a.terms.evalValue(node.key, cursor, context)
	if !ok {
		return ResolvedEffect{}, false
	}
	value, ok := a.terms.evalValue(node.value, cursor, context)
	if !ok {
		return ResolvedEffect{}, false
	}
	keyPath, valuePath := pathdom.Path{}, pathdom.Path{}
	if node.keyPath != 0 {
		keyPath, ok = a.terms.evalPath(node.keyPath, cursor)
		if !ok {
			return ResolvedEffect{}, false
		}
	}
	if node.valuePath != 0 {
		valuePath, ok = a.terms.evalPath(node.valuePath, cursor)
		if !ok {
			return ResolvedEffect{}, false
		}
	}
	return ResolvedEffect{Kind: node.kind, Mutation: ResolvedIndexMutation{
		Invalidation: invalidation, Table: table, Key: key, Value: value,
		KeyPath: keyPath, ValuePath: valuePath, Admission: node.admission,
		Readback: node.readback, AppendCandidate: node.appendMode, Site: node.site,
	}}, true
}

func (a *EffectArena) resolveInvalidation(config InvalidatePathConfig, cursor BindingCursor, context SpecializationContext) (ResolvedPathInvalidation, bool) {
	target, ok := a.terms.evalPath(config.Target, cursor)
	if !ok {
		return ResolvedPathInvalidation{}, false
	}
	out := ResolvedPathInvalidation{
		Target: target, Scope: config.Scope,
		PreserveStructuralWitness:       config.PreserveStructuralWitness,
		PreserveDynamicValueMemberships: config.PreserveDynamicValueMemberships,
	}
	if config.Precise != nil {
		table, tableOK := a.terms.evalPath(config.Precise.Table, cursor)
		key, keyOK := a.terms.evalValue(config.Precise.Key, cursor, context)
		if !tableOK || !keyOK {
			return ResolvedPathInvalidation{}, false
		}
		out.Precise = &ResolvedPreciseDynamicTarget{
			Table: table, Key: key,
			Suffix: append([]segment.Segment(nil), config.Precise.Suffix...),
		}
	}
	return out, true
}
