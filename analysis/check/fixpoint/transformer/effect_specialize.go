package transformer

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
)

// ResolvedPathInvalidation is a caller-bound invalidation request. It contains
// values and paths only; applying it to caller State remains the canonical call
// adapter's responsibility.
type ResolvedPathInvalidation struct {
	Target                          pathdom.Path
	TargetRef                       ResolvedEffectTarget
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
	TableTarget     ResolvedEffectTarget
	Key             product.Value
	Value           product.Value
	KeyPath         pathdom.Path
	ValuePath       pathdom.Path
	Admission       dynamicindex.Admission
	Readback        factflow.DynamicIndexReadbackIntent
	AppendCandidate bool
	Site            EffectSite
}

// ResolvedEffectTarget is the caller-bound form of EffectTargetTerm. Path and
// Allocation expose mutually exclusive payloads.
type ResolvedEffectTarget struct {
	kind       effectTargetKind
	path       pathdom.Path
	allocation ResolvedAllocationTemplate
}

func (t ResolvedEffectTarget) Path() (pathdom.Path, bool) {
	return t.path, t.kind == effectTargetPath
}

func (t ResolvedEffectTarget) Allocation() (ResolvedAllocationTemplate, bool) {
	return t.allocation, t.kind == effectTargetAllocation
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

func (a *relationOutputAuthority) allowsEffectFragment(effects []ResolvedEffect, fragment summary.Summary) bool {
	if a == nil || len(effects) == 0 {
		return false
	}
	// NormalReturnFacts has its own descriptor-driven nested lane catalog.
	// Preserve effect origin by requiring every effect in the ordered sequence
	// to authorize every emitted boundary kind; a union would let one effect
	// smuggle lanes through another effect's descriptor. OutputCapabilityRegistry
	// separately certifies the row-owned Summary at Build time; effect fragments
	// are certified by EffectDescriptor.BoundaryKinds.
	boundaryKinds := make([]callboundary.BoundaryFactKind, 0)
	for _, lane := range callboundary.NormalReturnFactLanes() {
		if lane.Len(fragment.NormalReturnFacts) != 0 {
			boundaryKinds = append(boundaryKinds, callboundary.BoundaryFactKind(lane.ID()))
		}
	}
	for _, kind := range summary.PresentFactKinds(fragment) {
		if kind != callboundary.BoundaryFactKind("NormalReturnFacts") {
			boundaryKinds = append(boundaryKinds, kind)
		}
	}
	if len(boundaryKinds) == 0 {
		// Every currently admitted EffectKind represents an observable semantic
		// transaction. An empty fragment would silently drop it while claiming
		// exact specialization.
		return false
	}
	for _, effect := range effects {
		allowedKinds, ok := a.effects[effect.Kind]
		if !ok {
			return false
		}
		for _, kind := range boundaryKinds {
			if _, allowed := allowedKinds[kind]; !allowed {
				return false
			}
		}
	}
	return true
}

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
	tableTarget, ok := a.resolveTarget(node.table, cursor)
	if !ok {
		return ResolvedEffect{}, false
	}
	table, _ := tableTarget.Path()
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
		Invalidation: invalidation, Table: table, TableTarget: tableTarget, Key: key, Value: value,
		KeyPath: keyPath, ValuePath: valuePath, Admission: node.admission,
		Readback: node.readback, AppendCandidate: node.appendMode, Site: node.site,
	}}, true
}

func (a *EffectArena) resolveInvalidation(config InvalidatePathConfig, cursor BindingCursor, context SpecializationContext) (ResolvedPathInvalidation, bool) {
	targetRef, ok := a.resolveTarget(config.Target, cursor)
	if !ok {
		return ResolvedPathInvalidation{}, false
	}
	target, _ := targetRef.Path()
	out := ResolvedPathInvalidation{
		Target: target, TargetRef: targetRef, Scope: config.Scope,
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

func (a *EffectArena) resolveTarget(target EffectTargetTerm, cursor BindingCursor) (ResolvedEffectTarget, bool) {
	if !a.ownsTarget(target) {
		return ResolvedEffectTarget{}, false
	}
	if target.kind == effectTargetPath {
		path, ok := a.terms.evalPath(target.path, cursor)
		return ResolvedEffectTarget{kind: effectTargetPath, path: path}, ok
	}
	op := a.terms.allocations[target.allocation].op
	result, ok := a.terms.allocationResult(target.allocation, op.Template().ReturnIndex)
	if !ok {
		return ResolvedEffectTarget{}, false
	}
	allocation := ResolvedAllocationTemplate{Site: op.Site(), Template: op, Result: result}
	return ResolvedEffectTarget{kind: effectTargetAllocation, allocation: allocation}, true
}
