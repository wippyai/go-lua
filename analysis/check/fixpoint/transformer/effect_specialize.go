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

// EffectContribution certifies the boundary lanes emitted by one resolved
// effect. Contributions are positional: entry i must describe resolved effect
// i, preserving the semantic order of non-commuting transactions.
type EffectContribution struct {
	Kind          EffectKind
	BoundaryKinds []callboundary.BoundaryFactKind
}

// EffectResolution is the atomic result of lowering one complete correlated
// effect sequence. Summary is published only when Contributions proves one
// non-empty, descriptor-authorized record per input effect and their union is
// exactly the set of populated boundary lanes in Summary.
type EffectResolution struct {
	Summary       summary.Summary
	Contributions []EffectContribution
}

// EffectSummaryResolver lowers one complete correlated row of resolved effects
// into a transaction-local fragment of the existing Summary representation.
// Returning false rejects the entire relation specialization. The callback is
// deliberately unable to mutate caller State.
type EffectSummaryResolver func([]ResolvedEffect) (EffectResolution, bool)

func (a *relationOutputAuthority) allowsEffectResolution(effects []ResolvedEffect, resolution EffectResolution) bool {
	if a == nil || len(effects) == 0 {
		return false
	}
	if len(resolution.Contributions) != len(effects) {
		return false
	}
	actual := presentEffectBoundaryKinds(resolution.Summary)
	if len(actual) == 0 {
		return false
	}
	emitted := make(map[callboundary.BoundaryFactKind]struct{}, len(actual))
	for i, contribution := range resolution.Contributions {
		if contribution.Kind != effects[i].Kind || len(contribution.BoundaryKinds) == 0 {
			return false
		}
		allowedKinds, ok := a.effects[contribution.Kind]
		if !ok {
			return false
		}
		local := make(map[callboundary.BoundaryFactKind]struct{}, len(contribution.BoundaryKinds))
		for _, kind := range contribution.BoundaryKinds {
			if kind == "" {
				return false
			}
			if _, duplicate := local[kind]; duplicate {
				return false
			}
			local[kind] = struct{}{}
			if _, allowed := allowedKinds[kind]; !allowed {
				return false
			}
			emitted[kind] = struct{}{}
		}
	}
	if len(emitted) != len(actual) {
		return false
	}
	for kind := range actual {
		if _, ok := emitted[kind]; !ok {
			return false
		}
	}
	return true
}

// presentEffectBoundaryKinds flattens the nested NormalReturnFacts family to
// its stable storage-lane IDs and keeps all other Summary descriptor kinds.
// This is the canonical vocabulary used by EffectDescriptor.BoundaryKinds.
func presentEffectBoundaryKinds(fragment summary.Summary) map[callboundary.BoundaryFactKind]struct{} {
	boundaryKinds := make(map[callboundary.BoundaryFactKind]struct{})
	for _, lane := range callboundary.NormalReturnFactLanes() {
		if lane.Len(fragment.NormalReturnFacts) != 0 {
			boundaryKinds[callboundary.BoundaryFactKind(lane.ID())] = struct{}{}
		}
	}
	for _, kind := range summary.PresentFactKinds(fragment) {
		if kind != callboundary.BoundaryFactKind("NormalReturnFacts") {
			boundaryKinds[kind] = struct{}{}
		}
	}
	return boundaryKinds
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
