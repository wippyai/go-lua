package owner

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/domain/effect/factor"
)

// This file is the effect factor's query half. The axis beside it declares the
// coordinate space and binds the principal that writes it; this declares the
// one family that space is read out of, and the fold, freeze, and equality the
// family's answers are produced by. Both halves are stated by this package, so
// what the effect-exact family answers is effect's own statement rather than a
// fold a composition assembles out of the domain's parts.
//
// The family is exact: it is answered from the one row its subject holds, so it
// admits no split of that subject and its declared fold says so.

// ExactQueryImplementation is the sealed implementation the effect-exact
// family's answers are materialized and read through.
type ExactQueryImplementation = engine.ExactQueryImplementation[factor.Value, factor.EffectObservation]

// ExactQueryFragment is the effect-exact family's cold half: the query slot its
// answers are published in, the read form its fold runs over, and the identity
// its results are frozen under. All three are recorded by one declaration, so
// the rows a family folds, the slot it answers into, and the contract its
// answers travel under cannot be paired twice.
type ExactQueryFragment struct {
	slot    *engine.QuerySlot[factor.EffectObservation]
	read    engine.SchemaReadForm[factor.Value]
	freezer identity.SemanticKey
}

// Available reports whether this fragment holds every half of the declaration.
func (fragment *ExactQueryFragment) Available() bool {
	return fragment != nil && fragment.slot != nil && fragment.freezer.Available()
}

// QuerySpec is this package's effect-exact query declaration. The family reads
// the effect axis, admits no split of its subject, and is answered by the
// effect observation this domain owns.
func QuerySpec() query.Spec {
	return query.Spec{
		Family:   "effect-exact",
		Semantic: "semantic/query/effect-exact",
		Codec:    "semantic/query-result/effect-exact",
		// An exact read admits no split at all, so the obligation the fold rests
		// on is the exact read the family is answered by and the contract it
		// names is its own query identity.
		Fold:       query.FoldGeneral,
		Contract:   "semantic/query/effect-exact",
		Subjects:   []schema.Key{"effect"},
		Population: query.PopulationSelectedPoint,
		Projection: query.ProjectionExact,
	}
}

// DeclareQuery opens the effect-exact query slot against the open schema.
func DeclareQuery(builder *engine.SchemaBuilder, context query.Declaration) (*ExactQueryFragment, bool) {
	cell, cellOK := context.Subjects.At("effect")
	declared, declaredOK := axis.Payload[*SchemaFragment](cell)
	if !cellOK || !declaredOK {
		return nil, false
	}
	read := declared.ExactRead()
	if read.Schema() != nil {
		return nil, false
	}
	slot, slotOK := engine.NewQuerySlot[factor.EffectObservation](builder, engine.SchemaQuerySpec{Semantic: context.Semantic, Freezer: context.Freezer})
	if !slotOK || !engine.SchemaQueryRead(slot, read) {
		return nil, false
	}
	fragment := &ExactQueryFragment{slot: slot, read: read, freezer: context.Freezer}
	return fragment, fragment.Available()
}

// BindQuery installs the effect-exact fold on the bound principal.
func BindQuery(_ *engine.SchemaBinding, context query.Binding[*ExactQueryFragment]) bool {
	cell, cellOK := context.Subjects.At("effect")
	owner, ownerOK := axis.Payload[*HotOwner](cell)
	if !cellOK || !ownerOK || !context.Fragment.Available() || context.Fragment.read.Schema() == nil {
		return false
	}
	return BindExactQuery(owner, context.Fragment.slot, exactQuerySpec(owner.Algebra(), context.Fragment.freezer))
}

// RecoverQuery recovers the sealed effect-exact implementation.
func RecoverQuery(binding *engine.SchemaBinding, context query.Sealed[*ExactQueryFragment]) (*ExactQueryImplementation, bool) {
	if !context.Fragment.Available() {
		return nil, false
	}
	return engine.ExactQueryImplementationAt[factor.Value, factor.EffectObservation](binding, context.Fragment.slot)
}

// exactQuerySpec is the family's hot half against one Link's sealed effect
// algebra: the exact fold the domain already states, and the result contract
// its answers are frozen, cloned, compared, and fingerprinted under. Every term
// is the domain's own function; what is authored here is which of them answers
// this family, and that the read it folds is exactly one row.
func exactQuerySpec(algebra *factor.Algebra, freezer identity.SemanticKey) engine.HotExactQuerySpec[factor.Value, factor.EffectObservation] {
	return engine.HotExactQuerySpec[factor.Value, factor.EffectObservation]{
		Fold: engine.QueryFold[engine.OrderedCells[factor.Value], factor.EffectObservation]{
			Begin:          func() factor.EffectObservation { return factor.BeginEffect(algebra) },
			BorrowIssued:   true,
			TransferResult: true,
			Accumulate: func(result factor.EffectObservation, cells engine.OrderedCells[factor.Value]) (factor.EffectObservation, bool) {
				if cells.Count() != 1 {
					return factor.EffectObservation{}, false
				}
				value, present, available := cells.At(0)
				return factor.AccumulateEffect(algebra, result, value, present, available)
			},
		},
		Result: engine.FrozenResult[factor.EffectObservation]{
			Semantic: freezer,
			Freeze:   factor.CloneEffect, Clone: factor.CloneEffect,
			Equal: factor.EqualEffect, Fingerprint: factor.FingerprintEffect,
			Present: func(observation factor.EffectObservation) bool {
				return observation.Rows != 0
			},
		},
	}
}
