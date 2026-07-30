package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signature"
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

type ResolvedPathStore struct {
	Assignment          factapply.ResolvedPathStoreWrite
	Static              factapply.ResolvedPathStoreWrite
	HasAssignment       bool
	HasStatic           bool
	Site                EffectSite
	StaticHasAnnotation bool
	Object              factapply.ResolvedPathStoreObject
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
	Object       factapply.ResolvedPathStoreObject
	PathStore    ResolvedPathStore
}

// ResolvedAllocationTemplate is the specialization handoff for one shared
// allocation node. Result and heap/fresh lowering must use this same site and
// template identity.
type ResolvedAllocationTemplate struct {
	Site       operationplan.SignatureAllocationSite
	Template   operationplan.SignatureAllocationOperation
	Result     product.Value
	identities map[signature.AllocationTemplateID]identity.Term
}

// freezeTransaction is the allocation materialization fence. The route-owned
// authority substitutes every arena-owned AllocationTerm before the concrete
// heap graph is constructed, so result, heap, placement, and freshness cannot
// split across identity namespaces.

func (a ResolvedAllocationTemplate) freezeTransaction(reg *axis.Registry, keys *keyspace.KeySpace, allocations *state.BoundaryAllocationAuthority) (factapply.AllocationTemplateTransaction, error) {
	if allocations == nil || len(a.identities) == 0 {
		return factapply.AllocationTemplateTransaction{}, fmt.Errorf("transformer: allocation transaction has no boundary authority")
	}
	concrete := make(map[signature.AllocationTemplateID]identity.Term, len(a.identities))
	for name, term := range a.identities {
		template, symbolic := term.Allocation()
		if !symbolic {
			return factapply.AllocationTemplateTransaction{}, fmt.Errorf("transformer: allocation transaction retained a non-template identity")
		}
		actual, exact := allocations.RebaseAllocation(template)
		if !exact {
			return factapply.AllocationTemplateTransaction{}, fmt.Errorf("transformer: allocation transaction is outside boundary authority")
		}
		concrete[name] = identity.ConcreteTerm(actual)
	}
	materialized, ok := effectlowering.MaterializeStaticAllocation(
		reg, nil, keys, cfg.Point(a.Site.Ordinal), a.Template.Template(), concrete,
	)
	if !ok {
		return factapply.AllocationTemplateTransaction{}, fmt.Errorf("transformer: allocation transaction failed concrete boundary materialization")
	}
	rootTerm, rootBound := concrete[a.Template.Template().Root]
	rootID, rootConcrete := rootTerm.Concrete()
	if !rootBound || !rootConcrete {
		return factapply.AllocationTemplateTransaction{}, fmt.Errorf("transformer: allocation transaction has no concrete root image")
	}
	expected := product.Set(reg, a.Result, identity.Key, identity.Singleton(rootID))
	if !product.Equal(reg, materialized.Result, expected) {
		return factapply.AllocationTemplateTransaction{}, fmt.Errorf("transformer: allocation transaction diverged from its symbolic result")
	}
	return factapply.NewAllocationTemplateTransaction(reg, factapply.AllocationTemplateMaterialization{
		Result: materialized.Result, Objects: materialized.Objects, Placements: materialized.Placements, KeySpace: materialized.KeySpace,
	})
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

type effectValueResolver func(ValueTerm) (product.Value, bool)

// resolve is the concrete adapter for the canonical effect assembler. Value
// syntax is owned by Arena's value algebra; effect assembly only consumes the
// resulting leaves.
func (a *EffectArena) resolve(term EffectTerm, cursor BindingCursor, context SpecializationContext) (ResolvedEffect, bool) {
	return a.resolveWithValues(term, cursor, func(value ValueTerm) (product.Value, bool) {
		return a.terms.evalValue(value, cursor, context)
	})
}

// resolveWithValues assembles one typed effect from already-resolved semantic
// ValueTerm leaves and structural paths. Guarded execution supplies leaves
// from guardedValueDecision; concrete callers supply the canonical scalar
// value algebra through resolve above. There is only one effect assembly law.
func (a *EffectArena) resolveWithValues(term EffectTerm, cursor BindingCursor, resolveValue effectValueResolver) (ResolvedEffect, bool) {
	if a == nil || a.terms == nil || term == 0 || int(term) >= len(a.nodes) {
		return ResolvedEffect{}, false
	}
	if resolveValue == nil {
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
		return ResolvedEffect{Kind: node.kind, Allocation: ResolvedAllocationTemplate{
			Site: op.Site(), Template: op, Result: result,
			identities: cloneAllocationIdentityMap(a.terms.allocations[node.allocation].identities),
		}}, true
	}
	if node.kind == EffectObjectMaterialization {
		object, ok := a.resolvePathStoreObject(node.pathStoreObject, cursor, resolveValue)
		if !ok || node.site.Owner == 0 || len(object.Heaps) == 0 || len(object.Entries) != 0 || object.ListFloor != 0 {
			return ResolvedEffect{}, false
		}
		return ResolvedEffect{Kind: node.kind, Object: object}, true
	}
	if node.kind == EffectPathStore {
		assignment, assignmentOK := a.resolvePathStoreWrite(node.pathStoreAssignment, node.pathStoreHasAssignment, cursor, resolveValue)
		static, staticOK := a.resolvePathStoreWrite(node.pathStoreStatic, node.pathStoreHasStatic, cursor, resolveValue)
		object, objectOK := a.resolvePathStoreObject(node.pathStoreObject, cursor, resolveValue)
		if !assignmentOK || !staticOK || !objectOK || node.site.Owner == 0 {
			return ResolvedEffect{}, false
		}
		return ResolvedEffect{Kind: node.kind, PathStore: ResolvedPathStore{
			Assignment: assignment, Static: static, HasAssignment: node.pathStoreHasAssignment, HasStatic: node.pathStoreHasStatic,
			Site: node.site, StaticHasAnnotation: node.pathStoreStaticHasAnnotation, Object: object,
		}}, true
	}
	invalidation, ok := a.resolveInvalidation(node.invalidation, cursor, resolveValue)
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
	key, ok := resolveValue(node.key)
	if !ok {
		return ResolvedEffect{}, false
	}
	value, ok := resolveValue(node.value)
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

func (a *EffectArena) resolvePathStoreWrite(config PathStoreWriteConfig, present bool, cursor BindingCursor, resolveValue effectValueResolver) (factapply.ResolvedPathStoreWrite, bool) {
	if !present {
		return factapply.ResolvedPathStoreWrite{}, true
	}
	target, targetOK := a.terms.evalPath(config.Target, cursor)
	value, valueOK := resolveValue(config.Value)
	var source pathdom.Path
	sourceOK := true
	if config.HasSourcePath {
		source, sourceOK = a.terms.evalPath(config.SourcePath, cursor)
	}
	if !targetOK || !valueOK || !sourceOK {
		return factapply.ResolvedPathStoreWrite{}, false
	}
	return factapply.ResolvedPathStoreWrite{
		Target: target, Value: value, SourcePath: source, HasSourcePath: config.HasSourcePath, SuppressProof: config.SuppressProof,
	}, true
}

func (a *EffectArena) resolvePathStoreObject(config PathStoreObjectConfig, cursor BindingCursor, resolveValue effectValueResolver) (factapply.ResolvedPathStoreObject, bool) {
	object := factapply.ResolvedPathStoreObject{
		Heaps:     make([]factapply.ResolvedPathStoreHeapObject, len(config.Heaps)),
		Entries:   make([]factapply.ResolvedPathStoreWrite, len(config.Entries)),
		ListFloor: config.ListFloor,
	}
	for heapIndex, heapConfig := range config.Heaps {
		root, ok := resolveValue(heapConfig.Root)
		if !ok {
			return factapply.ResolvedPathStoreObject{}, false
		}
		heap := factapply.ResolvedPathStoreHeapObject{Root: root, Members: make([]factapply.ResolvedPathStoreHeapMember, len(heapConfig.Members)), StableShape: heapConfig.StableShape}
		for memberIndex, memberConfig := range heapConfig.Members {
			value, ok := resolveValue(memberConfig.Value)
			if !ok {
				return factapply.ResolvedPathStoreObject{}, false
			}
			member := factapply.ResolvedPathStoreHeapMember{Suffix: append([]segment.Segment(nil), memberConfig.Suffix...), Value: value, HasExpected: memberConfig.HasExpected}
			if memberConfig.HasExpected {
				member.Expected, ok = resolveValue(memberConfig.Expected)
				if !ok {
					return factapply.ResolvedPathStoreObject{}, false
				}
			}
			heap.Members[memberIndex] = member
		}
		object.Heaps[heapIndex] = heap
	}
	for entryIndex, entryConfig := range config.Entries {
		target, targetOK := a.terms.evalPath(entryConfig.Target, cursor)
		value, valueOK := resolveValue(entryConfig.Value)
		if !targetOK || !valueOK {
			return factapply.ResolvedPathStoreObject{}, false
		}
		entry := factapply.ResolvedPathStoreWrite{Target: target, Value: value, HasSourcePath: entryConfig.HasSourcePath, SuppressProof: entryConfig.SuppressProof, HasExpected: entryConfig.HasExpected}
		if entryConfig.HasSourcePath {
			entry.SourcePath, targetOK = a.terms.evalPath(entryConfig.SourcePath, cursor)
			if !targetOK {
				return factapply.ResolvedPathStoreObject{}, false
			}
		}
		if entryConfig.HasExpected {
			entry.Expected, valueOK = resolveValue(entryConfig.Expected)
			if !valueOK {
				return factapply.ResolvedPathStoreObject{}, false
			}
		}
		object.Entries[entryIndex] = entry
	}
	return object, true
}

func (a *EffectArena) resolveInvalidation(config InvalidatePathConfig, cursor BindingCursor, resolveValue effectValueResolver) (ResolvedPathInvalidation, bool) {
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
		key, keyOK := resolveValue(config.Precise.Key)
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
	allocation := ResolvedAllocationTemplate{
		Site: op.Site(), Template: op, Result: result,
		identities: cloneAllocationIdentityMap(a.terms.allocations[target.allocation].identities),
	}
	return ResolvedEffectTarget{kind: effectTargetAllocation, allocation: allocation}, true
}

func cloneAllocationIdentityMap(input map[signature.AllocationTemplateID]identity.Term) map[signature.AllocationTemplateID]identity.Term {
	if len(input) == 0 {
		return nil
	}
	out := make(map[signature.AllocationTemplateID]identity.Term, len(input))
	for name, term := range input {
		out[name] = term
	}
	return out
}
