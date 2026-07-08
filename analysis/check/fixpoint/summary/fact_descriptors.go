package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

// SummarySlotOps is the summary family's per-kind behavior payload. It mirrors
// the storage-level fields of summaryLane: every lane owns emptiness, clone, and
// normalize behavior, and non-slot lanes additionally own their lattice
// operations (equal, lessOrEq, join, widen). Slot lanes (Returns,
// ParamObligations, NormalReturnParams, NormalReturnParamConditions) leave the
// lattice hooks nil because their equal/join/widen are arity-aware and driven
// inline by summary_lattice.go rather than per-lane.
type SummarySlotOps struct {
	slot           bool
	empty          func(Summary) bool
	assignClone    func(src Summary, dst *Summary)
	normalizeOwned func(reg *axis.Registry, s *Summary)
	equal          func(reg *axis.Registry, a, b Summary, normalized bool) bool
	lessOrEq       func(reg *axis.Registry, a, b Summary) bool
	assignJoin     func(reg *axis.Registry, a, b Summary, out *Summary)
	assignWiden    func(reg *axis.Registry, prev, next Summary, out *Summary)
}

// Slot reports whether the lane is an arity-indexed slot lane whose lattice ops
// are driven inline rather than per-lane.
func (o SummarySlotOps) Slot() bool { return o.slot }

// deriveSummaryLane rebuilds the summaryLane a descriptor describes.
func deriveSummaryLane(d callboundary.BoundaryFactDescriptor[SummarySlotOps]) summaryLane {
	return summaryLane{
		fieldName:      string(d.Kind),
		slot:           d.Ops.slot,
		empty:          d.Ops.empty,
		assignClone:    d.Ops.assignClone,
		normalizeOwned: d.Ops.normalizeOwned,
		equal:          d.Ops.equal,
		lessOrEq:       d.Ops.lessOrEq,
		assignJoin:     d.Ops.assignJoin,
		assignWiden:    d.Ops.assignWiden,
	}
}

func summarySlotDescriptor(fieldName string, wireRef []string, ops SummarySlotOps) callboundary.BoundaryFactDescriptor[SummarySlotOps] {
	return callboundary.BoundaryFactDescriptor[SummarySlotOps]{
		Kind:    callboundary.BoundaryFactKind(fieldName),
		WireRef: wireRef,
		Ops:     ops,
	}
}

// summaryFactDescriptors is the descriptor-driven summary lane table. It
// registers one entry per Summary payload field in the same canonical order as
// summaryLanes, adding the WireRef link to the manifest OperationalEffects wire
// codec where the field lowers 1:1. Ground truth comes from the summary ->
// signature exporter (analysis/check/exportmanifest/function_signatures.go):
// ReturnPresenceRelations lowers into the ReturnPresenceRelations wire lane;
// NormalReturnFacts is a nested boundary-fact family whose wire refs are owned
// by callboundary.NormalReturnFactDescriptors; the remaining summary lanes
// (return tuples, param obligations, member-call facts, sink exposures,
// captured obligations, condition refinements, literal cases) are serialized
// through the signature return/param/postcondition encoders, not the
// OperationalEffects wire codec, so they carry a nil WireRef.
//
// fact_descriptors_test.go proves DeriveBoundaryLanes over
// this table reproduces the live summaryLanes behavior lane-for-lane
// (order, field name, slot flag, and every non-nil op) across a populated
// corpus, so the whole-summary Join/Widen/Equal/LessOrEq/Normalize drivers stay
// invariant after the flip.
var summaryFactDescriptors = func() callboundary.BoundaryFactTable[SummarySlotOps] {
	t := callboundary.BoundaryFactTable[SummarySlotOps]{
		summarySlotDescriptor("Returns", nil, SummarySlotOps{
			slot:        true,
			empty:       func(s Summary) bool { return len(s.Returns) == 0 },
			assignClone: func(src Summary, dst *Summary) { dst.Returns = cloneSlice(src.Returns) },
			normalizeOwned: func(reg *axis.Registry, s *Summary) {
				s.Returns = trimTrailingProducts(reg, s.Returns, product.Bottom(reg))
			},
		}),
		summarySlotDescriptor("ParamObligations", nil, SummarySlotOps{
			slot:        true,
			empty:       func(s Summary) bool { return len(s.ParamObligations) == 0 },
			assignClone: func(src Summary, dst *Summary) { dst.ParamObligations = cloneSlice(src.ParamObligations) },
			normalizeOwned: func(reg *axis.Registry, s *Summary) {
				s.ParamObligations = trimTrailingProducts(reg, s.ParamObligations, product.Top())
			},
		}),
		summarySlotDescriptor("ParamMemberCallObligations", nil, SummarySlotOps{
			empty: func(s Summary) bool { return len(s.ParamMemberCallObligations) == 0 },
			assignClone: func(src Summary, dst *Summary) {
				dst.ParamMemberCallObligations = cloneSlice(src.ParamMemberCallObligations)
			},
			normalizeOwned: func(_ *axis.Registry, s *Summary) {
				s.ParamMemberCallObligations = paramMemberCallObligationLane.Normalize(s.ParamMemberCallObligations)
			},
			equal: func(_ *axis.Registry, a, b Summary, _ bool) bool {
				return paramMemberCallObligationLane.Equal(a.ParamMemberCallObligations, b.ParamMemberCallObligations)
			},
			lessOrEq: func(_ *axis.Registry, a, b Summary) bool {
				return paramMemberCallObligationLane.LessOrEq(a.ParamMemberCallObligations, b.ParamMemberCallObligations)
			},
			assignJoin: func(_ *axis.Registry, a, b Summary, out *Summary) {
				out.ParamMemberCallObligations = paramMemberCallObligationLane.Join(
					a.ParamMemberCallObligations,
					b.ParamMemberCallObligations,
				)
			},
			assignWiden: func(_ *axis.Registry, prev, next Summary, out *Summary) {
				out.ParamMemberCallObligations = paramMemberCallObligationLane.Join(
					prev.ParamMemberCallObligations,
					next.ParamMemberCallObligations,
				)
			},
		}),
		summarySlotDescriptor("ParamMemberReturnSlots", nil, SummarySlotOps{
			empty:       func(s Summary) bool { return len(s.ParamMemberReturnSlots) == 0 },
			assignClone: func(src Summary, dst *Summary) { dst.ParamMemberReturnSlots = cloneSlice(src.ParamMemberReturnSlots) },
			normalizeOwned: func(_ *axis.Registry, s *Summary) {
				s.ParamMemberReturnSlots = paramMemberReturnSlotLane.Normalize(s.ParamMemberReturnSlots)
			},
			equal: func(_ *axis.Registry, a, b Summary, _ bool) bool {
				return paramMemberReturnSlotLane.Equal(a.ParamMemberReturnSlots, b.ParamMemberReturnSlots)
			},
			lessOrEq: func(_ *axis.Registry, a, b Summary) bool {
				return paramMemberReturnSlotLane.LessOrEq(a.ParamMemberReturnSlots, b.ParamMemberReturnSlots)
			},
			assignJoin: func(_ *axis.Registry, a, b Summary, out *Summary) {
				out.ParamMemberReturnSlots = paramMemberReturnSlotLane.Join(a.ParamMemberReturnSlots, b.ParamMemberReturnSlots)
			},
			assignWiden: func(_ *axis.Registry, prev, next Summary, out *Summary) {
				out.ParamMemberReturnSlots = paramMemberReturnSlotLane.Join(prev.ParamMemberReturnSlots, next.ParamMemberReturnSlots)
			},
		}),
		summarySlotDescriptor("ReturnParamPathAliases", nil, SummarySlotOps{
			empty:       func(s Summary) bool { return len(s.ReturnParamPathAliases) == 0 },
			assignClone: func(src Summary, dst *Summary) { dst.ReturnParamPathAliases = cloneSlice(src.ReturnParamPathAliases) },
			normalizeOwned: func(_ *axis.Registry, s *Summary) {
				s.ReturnParamPathAliases = returnParamPathAliasLane.Normalize(s.ReturnParamPathAliases)
			},
			equal: func(_ *axis.Registry, a, b Summary, _ bool) bool {
				return returnParamPathAliasLane.Equal(a.ReturnParamPathAliases, b.ReturnParamPathAliases)
			},
			lessOrEq: func(_ *axis.Registry, a, b Summary) bool {
				return returnParamPathAliasLane.LessOrEq(a.ReturnParamPathAliases, b.ReturnParamPathAliases)
			},
			assignJoin: func(_ *axis.Registry, a, b Summary, out *Summary) {
				out.ReturnParamPathAliases = returnParamPathAliasLane.Join(a.ReturnParamPathAliases, b.ReturnParamPathAliases)
			},
			assignWiden: func(_ *axis.Registry, prev, next Summary, out *Summary) {
				out.ReturnParamPathAliases = returnParamPathAliasLane.Join(prev.ReturnParamPathAliases, next.ReturnParamPathAliases)
			},
		}),
		summarySlotDescriptor("ParamSinkExposures", nil, SummarySlotOps{
			empty:       func(s Summary) bool { return len(s.ParamSinkExposures) == 0 },
			assignClone: func(src Summary, dst *Summary) { dst.ParamSinkExposures = cloneSlice(src.ParamSinkExposures) },
			normalizeOwned: func(reg *axis.Registry, s *Summary) {
				s.ParamSinkExposures = paramSinkExposureMap(reg).Normalize(s.ParamSinkExposures)
			},
			equal: func(reg *axis.Registry, a, b Summary, _ bool) bool {
				return paramSinkExposureMap(reg).Equal(a.ParamSinkExposures, b.ParamSinkExposures)
			},
			lessOrEq: func(reg *axis.Registry, a, b Summary) bool {
				return paramSinkExposureMap(reg).LessOrEq(a.ParamSinkExposures, b.ParamSinkExposures)
			},
			assignJoin: func(reg *axis.Registry, a, b Summary, out *Summary) {
				out.ParamSinkExposures = paramSinkExposureMap(reg).Join(a.ParamSinkExposures, b.ParamSinkExposures)
			},
			assignWiden: func(reg *axis.Registry, prev, next Summary, out *Summary) {
				out.ParamSinkExposures = paramSinkExposureMap(reg).Join(prev.ParamSinkExposures, next.ParamSinkExposures)
			},
		}),
		summarySlotDescriptor("CapturedPathObligations", nil, SummarySlotOps{
			empty:       func(s Summary) bool { return len(s.CapturedPathObligations) == 0 },
			assignClone: func(src Summary, dst *Summary) { dst.CapturedPathObligations = cloneSlice(src.CapturedPathObligations) },
			normalizeOwned: func(reg *axis.Registry, s *Summary) {
				s.CapturedPathObligations = normalizeCapturedPathObligations(reg, s.CapturedPathObligations)
			},
			equal: func(reg *axis.Registry, a, b Summary, _ bool) bool {
				return capturedPathObligationsEqual(reg, a.CapturedPathObligations, b.CapturedPathObligations)
			},
			lessOrEq: func(reg *axis.Registry, a, b Summary) bool {
				return capturedPathObligationsLessOrEq(reg, a.CapturedPathObligations, b.CapturedPathObligations)
			},
			assignJoin: func(reg *axis.Registry, a, b Summary, out *Summary) {
				out.CapturedPathObligations = combineCapturedPathObligations(
					reg,
					a.CapturedPathObligations,
					b.CapturedPathObligations,
				)
			},
			assignWiden: func(reg *axis.Registry, prev, next Summary, out *Summary) {
				out.CapturedPathObligations = combineCapturedPathObligations(
					reg,
					prev.CapturedPathObligations,
					next.CapturedPathObligations,
				)
			},
		}),
		summarySlotDescriptor("NormalReturnParams", nil, SummarySlotOps{
			slot:        true,
			empty:       func(s Summary) bool { return len(s.NormalReturnParams) == 0 },
			assignClone: func(src Summary, dst *Summary) { dst.NormalReturnParams = cloneSlice(src.NormalReturnParams) },
			normalizeOwned: func(reg *axis.Registry, s *Summary) {
				s.NormalReturnParams = trimTrailingProducts(reg, s.NormalReturnParams, product.Bottom(reg))
			},
		}),
		summarySlotDescriptor("NormalReturnParamConditions", nil, SummarySlotOps{
			slot:  true,
			empty: func(s Summary) bool { return len(s.NormalReturnParamConditions) == 0 },
			assignClone: func(src Summary, dst *Summary) {
				dst.NormalReturnParamConditions = cloneSlice(src.NormalReturnParamConditions)
			},
			normalizeOwned: func(_ *axis.Registry, s *Summary) {
				for len(s.NormalReturnParamConditions) > 0 &&
					!s.NormalReturnParamConditions[len(s.NormalReturnParamConditions)-1].IsUseful() {
					s.NormalReturnParamConditions = s.NormalReturnParamConditions[:len(s.NormalReturnParamConditions)-1]
				}
			},
		}),
		summarySlotDescriptor("NormalReturnParamEqualities", nil, SummarySlotOps{
			empty: func(s Summary) bool { return len(s.NormalReturnParamEqualities) == 0 },
			assignClone: func(src Summary, dst *Summary) {
				dst.NormalReturnParamEqualities = cloneSlice(src.NormalReturnParamEqualities)
			},
			normalizeOwned: func(_ *axis.Registry, s *Summary) {
				s.NormalReturnParamEqualities = normalizeParamEqualities(s.NormalReturnParamEqualities)
			},
			equal: func(reg *axis.Registry, a, b Summary, _ bool) bool {
				return paramEqualitiesSummaryEqual(reg, a, b)
			},
			lessOrEq: func(reg *axis.Registry, a, b Summary) bool {
				return paramEqualitiesSummaryLessOrEq(reg, a, b)
			},
			assignJoin: func(reg *axis.Registry, a, b Summary, out *Summary) {
				out.NormalReturnParamEqualities = joinParamEqualities(reg, a, b)
			},
			assignWiden: func(reg *axis.Registry, prev, next Summary, out *Summary) {
				out.NormalReturnParamEqualities = joinParamEqualities(reg, prev, next)
			},
		}),
		summarySlotDescriptor("NormalReturnFacts", nil, SummarySlotOps{
			empty:       func(s Summary) bool { return s.NormalReturnFacts.Empty() },
			assignClone: func(src Summary, dst *Summary) { dst.NormalReturnFacts = CloneNormalReturnFacts(src.NormalReturnFacts) },
			normalizeOwned: func(reg *axis.Registry, s *Summary) {
				s.NormalReturnFacts = normalizeOwnedNormalReturnFacts(reg, s.NormalReturnFacts)
			},
			equal: func(reg *axis.Registry, a, b Summary, normalized bool) bool {
				return normalReturnFactsEqualFor(reg, a.NormalReturnFacts, b.NormalReturnFacts, normalized)
			},
			lessOrEq: func(reg *axis.Registry, a, b Summary) bool {
				return normalReturnFactsLessOrEq(reg, a.NormalReturnFacts, b.NormalReturnFacts)
			},
			assignJoin: func(reg *axis.Registry, a, b Summary, out *Summary) {
				out.NormalReturnFacts = joinNormalReturnFacts(reg, a.NormalReturnFacts, b.NormalReturnFacts)
			},
			assignWiden: func(reg *axis.Registry, prev, next Summary, out *Summary) {
				out.NormalReturnFacts = widenNormalReturnFacts(reg, prev.NormalReturnFacts, next.NormalReturnFacts)
			},
		}),
		summarySlotDescriptor("HeapTableObjects", nil, SummarySlotOps{
			empty:       func(s Summary) bool { return len(s.HeapTableObjects) == 0 },
			assignClone: func(src Summary, dst *Summary) { dst.HeapTableObjects = cloneHeapTableObjects(src.HeapTableObjects) },
			normalizeOwned: func(reg *axis.Registry, s *Summary) {
				s.HeapTableObjects = normalizeOwnedHeapTableObjects(reg, s.HeapTableObjects)
			},
			equal: func(reg *axis.Registry, a, b Summary, _ bool) bool {
				return heapTableObjectsEqual(reg, a.HeapTableObjects, b.HeapTableObjects)
			},
			lessOrEq: func(reg *axis.Registry, a, b Summary) bool {
				return heapTableObjectsLessOrEq(reg, a.HeapTableObjects, b.HeapTableObjects)
			},
			assignJoin: func(reg *axis.Registry, a, b Summary, out *Summary) {
				out.HeapTableObjects, out.HeapKeySpace = joinSummaryHeapTableObjects(reg, a, b)
			},
			assignWiden: func(reg *axis.Registry, prev, next Summary, out *Summary) {
				out.HeapTableObjects, out.HeapKeySpace = widenSummaryHeapTableObjects(reg, prev, next)
			},
		}),
		summarySlotDescriptor("ReturnConditionParamRefinements", nil, SummarySlotOps{
			empty: func(s Summary) bool { return len(s.ReturnConditionParamRefinements) == 0 },
			assignClone: func(src Summary, dst *Summary) {
				dst.ReturnConditionParamRefinements = cloneReturnConditionParamRefinements(src.ReturnConditionParamRefinements)
			},
			normalizeOwned: func(reg *axis.Registry, s *Summary) {
				s.ReturnConditionParamRefinements = normalizeReturnConditionParamRefinements(
					reg,
					s.ReturnConditionParamRefinements,
				)
			},
			equal: func(reg *axis.Registry, a, b Summary, _ bool) bool {
				return returnConditionParamRefinementsEqual(
					reg,
					a.ReturnConditionParamRefinements,
					b.ReturnConditionParamRefinements,
				)
			},
			lessOrEq: func(reg *axis.Registry, a, b Summary) bool {
				return returnConditionParamRefinementsLessOrEq(
					reg,
					a.ReturnConditionParamRefinements,
					b.ReturnConditionParamRefinements,
				)
			},
			assignJoin: func(reg *axis.Registry, a, b Summary, out *Summary) {
				out.ReturnConditionParamRefinements = joinReturnConditionParamRefinements(
					reg,
					a.ReturnConditionParamRefinements,
					b.ReturnConditionParamRefinements,
				)
			},
			assignWiden: func(reg *axis.Registry, prev, next Summary, out *Summary) {
				out.ReturnConditionParamRefinements = joinReturnConditionParamRefinements(
					reg,
					prev.ReturnConditionParamRefinements,
					next.ReturnConditionParamRefinements,
				)
			},
		}),
		summarySlotDescriptor("ReturnConditionSlotRefinements", nil, SummarySlotOps{
			empty: func(s Summary) bool { return len(s.ReturnConditionSlotRefinements) == 0 },
			assignClone: func(src Summary, dst *Summary) {
				dst.ReturnConditionSlotRefinements = cloneReturnConditionSlotRefinements(src.ReturnConditionSlotRefinements)
			},
			normalizeOwned: func(reg *axis.Registry, s *Summary) {
				s.ReturnConditionSlotRefinements = normalizeReturnConditionSlotRefinements(
					reg,
					s.ReturnConditionSlotRefinements,
				)
			},
			equal: func(reg *axis.Registry, a, b Summary, _ bool) bool {
				return returnConditionSlotRefinementsEqual(
					reg,
					a.ReturnConditionSlotRefinements,
					b.ReturnConditionSlotRefinements,
				)
			},
			lessOrEq: func(reg *axis.Registry, a, b Summary) bool {
				return returnConditionSlotRefinementsLessOrEq(
					reg,
					a.ReturnConditionSlotRefinements,
					b.ReturnConditionSlotRefinements,
				)
			},
			assignJoin: func(reg *axis.Registry, a, b Summary, out *Summary) {
				out.ReturnConditionSlotRefinements = joinReturnConditionSlotRefinements(
					reg,
					a.ReturnConditionSlotRefinements,
					b.ReturnConditionSlotRefinements,
				)
			},
			assignWiden: func(reg *axis.Registry, prev, next Summary, out *Summary) {
				out.ReturnConditionSlotRefinements = joinReturnConditionSlotRefinements(
					reg,
					prev.ReturnConditionSlotRefinements,
					next.ReturnConditionSlotRefinements,
				)
			},
		}),
		summarySlotDescriptor("ReturnParamLiteralCases", nil, SummarySlotOps{
			empty: func(s Summary) bool { return len(s.ReturnParamLiteralCases) == 0 },
			assignClone: func(src Summary, dst *Summary) {
				dst.ReturnParamLiteralCases = cloneReturnParamLiteralCases(src.ReturnParamLiteralCases)
			},
			normalizeOwned: func(reg *axis.Registry, s *Summary) {
				s.ReturnParamLiteralCases = normalizeReturnParamLiteralCases(reg, s.ReturnParamLiteralCases)
			},
			equal: func(reg *axis.Registry, a, b Summary, _ bool) bool {
				return returnParamLiteralCasesEqual(reg, a.ReturnParamLiteralCases, b.ReturnParamLiteralCases)
			},
			lessOrEq: func(reg *axis.Registry, a, b Summary) bool {
				return returnParamLiteralCasesLessOrEq(reg, a.ReturnParamLiteralCases, b.ReturnParamLiteralCases)
			},
			assignJoin: func(reg *axis.Registry, a, b Summary, out *Summary) {
				out.ReturnParamLiteralCases = joinReturnParamLiteralCases(reg, a.ReturnParamLiteralCases, b.ReturnParamLiteralCases)
			},
			assignWiden: func(reg *axis.Registry, prev, next Summary, out *Summary) {
				out.ReturnParamLiteralCases = widenReturnParamLiteralCases(
					reg,
					prev.ReturnParamLiteralCases,
					next.ReturnParamLiteralCases,
				)
			},
		}),
		summarySlotDescriptor("ReturnPresenceRelations", []string{"ReturnPresenceRelations"}, SummarySlotOps{
			empty: func(s Summary) bool { return len(s.ReturnPresenceRelations) == 0 },
			assignClone: func(src Summary, dst *Summary) {
				dst.ReturnPresenceRelations = returnPresenceRelationLane.Clone(src.ReturnPresenceRelations)
			},
			normalizeOwned: func(_ *axis.Registry, s *Summary) {
				s.ReturnPresenceRelations = returnPresenceRelationLane.Normalize(s.ReturnPresenceRelations)
			},
			equal: func(_ *axis.Registry, a, b Summary, _ bool) bool {
				return returnPresenceRelationLane.Equal(a.ReturnPresenceRelations, b.ReturnPresenceRelations)
			},
			lessOrEq: func(_ *axis.Registry, a, b Summary) bool {
				return returnPresenceRelationLane.LessOrEq(a.ReturnPresenceRelations, b.ReturnPresenceRelations)
			},
			assignJoin: func(_ *axis.Registry, a, b Summary, out *Summary) {
				out.ReturnPresenceRelations = returnPresenceRelationLane.Join(a.ReturnPresenceRelations, b.ReturnPresenceRelations)
			},
			assignWiden: func(_ *axis.Registry, prev, next Summary, out *Summary) {
				out.ReturnPresenceRelations = returnPresenceRelationLane.Join(
					prev.ReturnPresenceRelations,
					next.ReturnPresenceRelations,
				)
			},
		}),
	}
	t.Validate("summary")
	return t
}()

// SummaryFactDescriptors returns the descriptor-driven summary lane table. The
// returned slice is a copy.
func SummaryFactDescriptors() callboundary.BoundaryFactTable[SummarySlotOps] {
	out := make(callboundary.BoundaryFactTable[SummarySlotOps], len(summaryFactDescriptors))
	copy(out, summaryFactDescriptors)
	return out
}

// derivedSummaryLanes is the lane slice used by summaryLanes.
func derivedSummaryLanes() []summaryLane {
	return callboundary.DeriveBoundaryLanes(summaryFactDescriptors, deriveSummaryLane)
}
