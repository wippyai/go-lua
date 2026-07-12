package projectsummary

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// ResolvedEffectSummaryResolver returns the canonical inactive lowering seam
// for a frozen relation. It emits only the existing Summary.NormalReturnFacts
// schema; caller State application remains owned by the call adapter.
func ResolvedEffectSummaryResolver(reg *axis.Registry) transformer.EffectSummaryResolver {
	return func(effects []transformer.ResolvedEffect) (summary.Summary, bool) {
		return LowerResolvedEffects(reg, effects)
	}
}

// LowerResolvedEffects lowers the proven boundary-table mutation slice. The
// complete ordered sequence is admitted before any fragment is built. Anything
// outside the slice rejects the whole row:
//   - standalone/precise/subtree invalidation;
//   - append candidates or overlapping mutation tables;
//   - key/value paths, concrete identities, boundary-lossy products, maybe-absent
//     products, literals, or non-scalar products.
//
// The restrictions are intentionally stronger than soundness alone. They make
// the existing PathInvalidations + DynamicIndexFacts call-boundary lanes exact
// with the concrete N3/N4 transaction without adding another effect schema.
func LowerResolvedEffects(reg *axis.Registry, effects []transformer.ResolvedEffect) (summary.Summary, bool) {
	if reg == nil || len(effects) == 0 {
		return summary.Summary{}, false
	}
	if effects[0].Kind == transformer.EffectAllocationTemplate {
		return lowerResolvedAllocations(reg, effects)
	}
	mutations := make([]transformer.ResolvedIndexMutation, len(effects))
	for i, effect := range effects {
		if effect.Kind != transformer.EffectIndexMutation {
			return summary.Summary{}, false
		}
		mutation := effect.Mutation
		if !safeResolvedBoundaryMutation(reg, mutation) {
			return summary.Summary{}, false
		}
		for j := 0; j < i; j++ {
			if mutation.Table.Overlaps(mutations[j].Table) {
				return summary.Summary{}, false
			}
		}
		mutations[i] = mutation
	}

	var facts callboundary.NormalReturnFacts
	for _, mutation := range mutations {
		// Append through the storage descriptor rather than assembling parallel
		// lane state. Summary.Normalize below supplies the canonical per-lane
		// lattice normalization/deduplication.
		fragment := callboundary.NormalReturnFacts{
			PathInvalidations: []callboundary.PathInvalidationFact{{
				Path: mutation.Table, PreserveStructuralWitness: true,
			}},
			DynamicIndexFacts: []callboundary.DynamicIndexFact{{
				Table: mutation.Table,
				Site:  dynamicindex.SiteForPoint(int(mutation.Site.Ordinal)),
				Value: dynamicindex.Fact{
					KeyPresence: product.PresenceOf(mutation.Key),
					KeyValue:    portableBoundaryValue(reg, mutation.Key),
					Value:       portableBoundaryValue(reg, mutation.Value),
					Admission:   mutation.Admission,
				},
			}},
		}
		facts = facts.Append(fragment)
	}
	out := summary.NormalizeOwned(reg, summary.Summary{NormalReturnFacts: facts})
	if out.NormalReturnFacts.Empty() {
		return summary.Summary{}, false
	}
	return out, true
}

func lowerResolvedAllocations(reg *axis.Registry, effects []transformer.ResolvedEffect) (summary.Summary, bool) {
	objects := make(map[identity.ID]heapidentity.TableObject, len(effects))
	fresh := make([]summary.FreshHeapAllocation, 0, len(effects))
	ks := keyspace.New()
	for _, effect := range effects {
		if effect.Kind != transformer.EffectAllocationTemplate {
			return summary.Summary{}, false
		}
		allocation := effect.Allocation
		materialized, exact := effectlowering.MaterializeStaticAllocation(
			reg, nil, ks, cfg.Point(allocation.Site.Ordinal), allocation.Template.Template(),
		)
		if !exact || !product.Equal(reg, materialized.Result, allocation.Result) {
			return summary.Summary{}, false
		}
		id, ok := product.Get(reg, allocation.Result, identity.Key).ID()
		if !ok || id == (identity.ID{}) {
			return summary.Summary{}, false
		}
		for objectID, object := range materialized.Objects {
			if _, duplicate := objects[objectID]; duplicate {
				return summary.Summary{}, false
			}
			objects[objectID] = object
			placementValue, ok := materialized.Placements[objectID]
			if !ok {
				return summary.Summary{}, false
			}
			fresh = append(fresh, summary.FreshHeapAllocation{ID: objectID, Placement: placementValue})
		}
	}
	out := summary.NormalizeOwned(reg, summary.Summary{
		HeapTableObjects: objects, FreshHeapAllocations: fresh, HeapKeySpace: ks,
	})
	if len(out.HeapTableObjects) != len(effects) || len(out.FreshHeapAllocations) != len(effects) {
		return summary.Summary{}, false
	}
	return out, true
}

func safeResolvedBoundaryMutation(reg *axis.Registry, mutation transformer.ResolvedIndexMutation) bool {
	if mutation.AppendCandidate || mutation.Site.Owner == 0 || mutation.Site.Ordinal == 0 ||
		mutation.Admission == dynamicindex.AdmissionBottom ||
		mutation.Readback != factflow.DynamicIndexReadbackKeyAndValue ||
		mutation.Invalidation.Scope != transformer.InvalidationScopeDescendants ||
		!mutation.Invalidation.PreserveStructuralWitness ||
		!mutation.Invalidation.PreserveDynamicValueMemberships ||
		mutation.Invalidation.Precise != nil ||
		mutation.Table.IsEmpty() || !mutation.Invalidation.Target.Equal(mutation.Table) ||
		!boundaryEffectPath(mutation.Table) ||
		!mutation.KeyPath.IsEmpty() || !mutation.ValuePath.IsEmpty() {
		return false
	}
	return exactPortablePresentScalar(reg, mutation.Key) && exactPortablePresentScalar(reg, mutation.Value)
}

func boundaryEffectPath(path pathdom.Path) bool {
	return path.IsPlaceholder() || callboundary.IsConcreteSymbolPath(path)
}

func exactPortablePresentScalar(reg *axis.Registry, value product.Value) bool {
	if _, hasConcreteIdentity := product.Get(reg, value, identity.Key).ID(); hasConcreteIdentity ||
		!presence.Equal(product.PresenceOf(value), presence.Present()) ||
		!product.Equal(reg, value, portableBoundaryValue(reg, value)) {
		return false
	}
	t, ok := typevalue.TypeOf(reg, value)
	if !ok {
		return false
	}
	switch t.Kind() {
	case kind.Boolean, kind.Number, kind.Integer, kind.String:
		_, literal := t.(*typ.Literal)
		return !literal
	default:
		return false
	}
}
