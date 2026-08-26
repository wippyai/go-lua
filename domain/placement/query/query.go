package query

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	queryschema "github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	placementsuspension "github.com/wippyai/go-lua/domain/placement/suspension"
)

// StructureSpecs contributes the Placement query family's own semantic roles.
// The factor owner contributes only the Placement axis roles; this vertical
// owns the query declaration and its result contract.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("query/placement-summary", "query-result/placement-summary")
}

// SummaryQueryFragment is the Placement query vertical's cold half. The
// placement class, complete Heap vector, and suspension evidence are declared
// as three typed subject reads; no producer-owned evidence is retained by the
// Placement factor owner.
type SummaryQueryFragment struct {
	slot          *engine.QuerySlot[placementdomain.PlacementSummaryObservation]
	placementRead engine.SchemaReadForm[placementdomain.Fact]
	heapRead      engine.SchemaReadForm[heapdomain.Value]
	evidenceRead  engine.SchemaReadForm[placementsuspension.Evidence]
	freezer       identity.SemanticKey
}

func (fragment *SummaryQueryFragment) Available() bool {
	return fragment != nil && fragment.slot != nil && fragment.freezer.Available()
}

// QuerySpec is the Placement query vertical's declaration. It is general
// because containment evidence depends on the complete Heap vector, and its
// suspension projection is admitted through the evidence axis subject.
func QuerySpec() queryschema.Spec {
	return queryschema.Spec{
		Family:     placementdomain.SummaryResultFamily,
		Semantic:   "semantic/query/placement-summary",
		Codec:      "semantic/query-result/placement-summary",
		Fold:       queryschema.FoldGeneral,
		Contract:   "semantic/query/placement-summary",
		Subjects:   []schema.Key{"placement", "heap", "placement-suspension-evidence"},
		Population: queryschema.PopulationSelectedPoint,
		Projection: queryschema.ProjectionExact,
	}
}

func DeclareQuery(builder *engine.SchemaBuilder, context queryschema.Declaration) (*SummaryQueryFragment, bool) {
	placementCell, placementCellOK := context.Subjects.At("placement")
	placementFragment, placementOK := axis.Payload[*placementowner.SchemaFragment](placementCell)
	heapCell, heapCellOK := context.Subjects.At("heap")
	heapFragment, heapOK := axis.Payload[*heapowner.SchemaFragment](heapCell)
	evidenceCell, evidenceCellOK := context.Subjects.At("placement-suspension-evidence")
	evidenceFragment, evidenceOK := axis.Payload[*placementsuspension.EvidenceFactorFragment](evidenceCell)
	if !placementCellOK || !placementOK || !heapCellOK || !heapOK || !evidenceCellOK || !evidenceOK {
		return nil, false
	}
	placementRead := placementFragment.FoldSummaryRead()
	heapRead := heapFragment.SummaryRead()
	evidenceRead := evidenceFragment.SummaryRead()
	if placementRead.Schema() != nil || heapRead.Schema() != nil || evidenceRead.Schema() != nil {
		return nil, false
	}
	slot, slotOK := engine.NewQuerySlot[placementdomain.PlacementSummaryObservation](builder, engine.SchemaQuerySpec{Semantic: context.Semantic, Freezer: context.Freezer, Population: context.Population})
	if !slotOK || !engine.SchemaQueryRead(slot, placementRead) || !engine.SchemaQueryRead(slot, heapRead) || !engine.SchemaQueryRead(slot, evidenceRead) {
		return nil, false
	}
	fragment := &SummaryQueryFragment{slot: slot, placementRead: placementRead, heapRead: heapRead, evidenceRead: evidenceRead, freezer: context.Freezer}
	return fragment, fragment.Available()
}

func BindQuery(binding *engine.SchemaBinding, context queryschema.Binding[*SummaryQueryFragment]) bool {
	placementCell, placementCellOK := context.Subjects.At("placement")
	placementOwner, placementOK := axis.Payload[*placementowner.HotOwner](placementCell)
	heapCell, heapCellOK := context.Subjects.At("heap")
	heapOwner, heapOK := axis.Payload[*heapowner.HotOwner](heapCell)
	evidenceCell, evidenceCellOK := context.Subjects.At("placement-suspension-evidence")
	evidenceOwner, evidenceOK := axis.Payload[*placementsuspension.EvidenceOwner](evidenceCell)
	if !placementCellOK || !placementOK || !heapCellOK || !heapOK || !evidenceCellOK || !evidenceOK || !context.Fragment.Available() || binding == nil ||
		!placementOwner.MatchesBinding(binding) || !heapOwner.MatchesBinding(binding) || !evidenceOwner.MatchesBinding(binding) ||
		context.Fragment.placementRead.Schema() == nil || context.Fragment.heapRead.Schema() == nil || context.Fragment.evidenceRead.Schema() == nil {
		return false
	}
	placementSchema := placementOwner.Schema()
	heapSchema := heapOwner.Schema()
	evidenceSchema := evidenceOwner.Schema()
	if !placementSchema.Valid() || !heapSchema.Valid() || !evidenceSchema.Valid() || placementSchema.Heap() != heapSchema || placementSchema != evidenceSchema {
		return false
	}
	return engine.BindHeterogeneousQuery(binding, context.Fragment.slot, summaryQuerySpec(placementOwner, heapOwner, evidenceOwner, context.Fragment.freezer))
}

func RecoverQuery(binding *engine.SchemaBinding, context queryschema.Sealed[*SummaryQueryFragment]) (*engine.HeterogeneousQueryImplementation[placementdomain.PlacementSummaryObservation], bool) {
	if !context.Fragment.Available() {
		return nil, false
	}
	return engine.HeterogeneousQueryImplementationAt[placementdomain.PlacementSummaryObservation](binding, context.Fragment.slot)
}

func EncodeQueryAnswer(answer engine.Answer) (present bool, rows uint64, payload []byte, ok bool) {
	observation, readable := engine.AnswerValue[placementdomain.PlacementSummaryObservation](answer)
	if !readable {
		return false, 0, nil, false
	}
	return placementdomain.EncodeSummaryResult(observation)
}

func summaryQuerySpec(placementOwner *placementowner.HotOwner, heapOwner *heapowner.HotOwner, evidenceOwner *placementsuspension.EvidenceOwner, freezer identity.SemanticKey) engine.HeterogeneousQuerySpec[placementdomain.PlacementSummaryObservation] {
	schema := placementOwner.Schema()
	containmentCache := placementOwner.StaticContainmentCache()
	projections := []engine.QueryProjectionSpec[placementdomain.PlacementSummaryObservation]{
		engine.NewSummaryQueryProjection(placementOwner.FoldSummaryRead(), engine.QueryProjectionFold[placementdomain.Fact, placementdomain.PlacementSummaryObservation]{
			BorrowIssued: true,
			Accumulate: func(result placementdomain.PlacementSummaryObservation, cells engine.OrderedCells[placementdomain.Fact]) (placementdomain.PlacementSummaryObservation, bool) {
				return placementdomain.AccumulatePlacementSummaryRows(schema, result, cells.Count(), cells.At)
			},
		}),
		engine.NewSummaryQueryProjection(heapOwner.SummaryRead(), engine.QueryProjectionFold[heapdomain.Value, placementdomain.PlacementSummaryObservation]{
			BorrowIssued: true,
			Accumulate: func(result placementdomain.PlacementSummaryObservation, cells engine.OrderedCells[heapdomain.Value]) (placementdomain.PlacementSummaryObservation, bool) {
				return placementdomain.AccumulatePlacementSummaryContainmentCached(containmentCache, schema, result, cells)
			},
		}),
		engine.NewSummaryQueryProjection(evidenceOwner.SummaryRead(), engine.QueryProjectionFold[placementsuspension.Evidence, placementdomain.PlacementSummaryObservation]{
			BorrowIssued: true,
			Accumulate: func(result placementdomain.PlacementSummaryObservation, cells engine.OrderedCells[placementsuspension.Evidence]) (placementdomain.PlacementSummaryObservation, bool) {
				return placementsuspension.AccumulatePlacementSummarySuspensionRows(schema, result, cells.Count(), cells.At)
			},
		}),
	}
	return engine.HeterogeneousQuerySpec[placementdomain.PlacementSummaryObservation]{
		Begin: func() placementdomain.PlacementSummaryObservation {
			return placementdomain.BeginPlacementSummary(schema)
		},
		Projections:    projections,
		TransferResult: true,
		Result: engine.FrozenResult[placementdomain.PlacementSummaryObservation]{
			Semantic: freezer,
			Freeze:   placementdomain.ClonePlacementSummary,
			Clone:    placementdomain.ClonePlacementSummary,
			Equal: func(left, right placementdomain.PlacementSummaryObservation) bool {
				return placementdomain.EqualPlacementSummary(schema, left, right)
			},
			Fingerprint: func(observation placementdomain.PlacementSummaryObservation) uint64 {
				return placementdomain.FingerprintPlacementSummary(schema, observation)
			},
			Present: func(observation placementdomain.PlacementSummaryObservation) bool { return observation.Rows != 0 },
		},
	}
}
