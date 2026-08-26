package owner

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/domain/value"
)

// This file is the value factor's query half. The axis beside it declares the
// coordinate space and binds the principal that writes it; this declares the
// one family that space is read out of, and the fold, freeze, and equality the
// family's answers are produced by. Both halves are stated by this package, so
// what the value-summary family answers is value's own statement rather than a
// fold a composition assembles out of the domain's parts.
//
// The fold itself is not restated here. It is the summary observation the value
// domain already owns; this declares which of the domain's folds answers the
// family, under which identities, and against which principal.

// SummaryQueryImplementation is the sealed implementation the value-summary
// family's answers are materialized and read through.
type SummaryQueryImplementation = engine.SummaryQueryImplementation[value.Value, value.ValueSummaryObservation]

// SummaryQueryFragment is the value-summary family's cold half: the query slot
// its answers are published in, the read form its fold runs over, and the
// identity its results are frozen under. All three are recorded by one
// declaration, so the rows a family folds, the slot it answers into, and the
// contract its answers travel under cannot be paired twice.
type SummaryQueryFragment struct {
	slot    *engine.QuerySlot[value.ValueSummaryObservation]
	read    engine.SchemaReadForm[value.Value]
	freezer identity.SemanticKey
}

// Available reports whether this fragment holds every half of the declaration.
func (fragment *SummaryQueryFragment) Available() bool {
	return fragment != nil && fragment.slot != nil && fragment.freezer.Available()
}

// QuerySpec is this package's value-summary query declaration. The family
// reads the value axis, folds coordinatewise, and is answered by the summary
// observation this domain owns.
func QuerySpec() query.Spec {
	return query.Spec{
		Family:   value.SummaryResultFamily,
		Semantic: "semantic/query/value-summary",
		Codec:    "semantic/query-result/value-summary",
		// The summary fold composes coordinatewise, so the family may be
		// answered over disjoint fragments of its subject and joined. The
		// contract that discharges that claim is the value schema's own
		// coordinatewise summary role.
		Fold:       query.FoldDistributive,
		Contract:   "semantic/factor/value/summary-coordinatewise",
		Subjects:   []schema.Key{"value"},
		Population: query.PopulationSelectedPoint,
		Projection: query.ProjectionSummary,
	}
}

// DeclareQuery opens the value-summary query slot against the open schema.
func DeclareQuery(builder *engine.SchemaBuilder, context query.Declaration) (*SummaryQueryFragment, bool) {
	cell, cellOK := context.Subjects.At("value")
	declared, declaredOK := axis.Payload[*SchemaFragment](cell)
	if !cellOK || !declaredOK {
		return nil, false
	}
	read := declared.FoldSummaryRead()
	if read.Schema() != nil {
		return nil, false
	}
	slot, slotOK := engine.NewQuerySlot[value.ValueSummaryObservation](builder, engine.SchemaQuerySpec{Semantic: context.Semantic, Freezer: context.Freezer, Population: context.Population})
	if !slotOK || !engine.SchemaQueryRead(slot, read) {
		return nil, false
	}
	fragment := &SummaryQueryFragment{slot: slot, read: read, freezer: context.Freezer}
	return fragment, fragment.Available()
}

// BindQuery installs the value-summary fold on the bound principal.
func BindQuery(_ *engine.SchemaBinding, context query.Binding[*SummaryQueryFragment]) bool {
	cell, cellOK := context.Subjects.At("value")
	owner, ownerOK := axis.Payload[*HotOwner](cell)
	if !cellOK || !ownerOK || !context.Fragment.Available() || context.Fragment.read.Schema() == nil {
		return false
	}
	return BindSummaryQuery(owner, context.Fragment.slot, context.Fragment.read, summaryQuerySpec(owner.Schema(), context.Fragment.freezer))
}

// RecoverQuery recovers the sealed value-summary implementation.
func RecoverQuery(binding *engine.SchemaBinding, context query.Sealed[*SummaryQueryFragment]) (*SummaryQueryImplementation, bool) {
	if !context.Fragment.Available() {
		return nil, false
	}
	return engine.SummaryQueryImplementationAt[value.Value, value.ValueSummaryObservation](binding, context.Fragment.slot)
}

// summaryQuerySpec is the family's hot half against one Link's sealed value
// schema: the summary fold the domain already states, and the result contract
// its answers are frozen, cloned, compared, and fingerprinted under. Every term
// is the domain's own function; what is authored here is which of them answers
// this family.
func summaryQuerySpec(schema *value.Schema, freezer identity.SemanticKey) engine.HotSummaryQuerySpec[value.Value, value.ValueSummaryObservation] {
	return engine.HotSummaryQuerySpec[value.Value, value.ValueSummaryObservation]{
		Fold: engine.QueryFold[engine.OrderedCells[value.Value], value.ValueSummaryObservation]{
			Begin:          func() value.ValueSummaryObservation { return value.BeginValueSummary(schema) },
			BorrowIssued:   true,
			TransferResult: true,
			Accumulate: func(result value.ValueSummaryObservation, cells engine.OrderedCells[value.Value]) (value.ValueSummaryObservation, bool) {
				return value.AccumulateValueSummaryRows(schema, result, cells.Count(), cells.At)
			},
		},
		Result: engine.FrozenResult[value.ValueSummaryObservation]{
			Semantic: freezer,
			Freeze:   value.CloneValueSummary, Clone: value.CloneValueSummary,
			Equal: func(left, right value.ValueSummaryObservation) bool {
				return value.EqualValueSummary(schema, left, right)
			},
			Fingerprint: func(observation value.ValueSummaryObservation) uint64 {
				return value.FingerprintValueSummary(schema, observation)
			},
			Present: func(observation value.ValueSummaryObservation) bool {
				return observation.Rows != 0
			},
		},
	}
}
