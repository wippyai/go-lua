// Package program owns Placement containment's callback-free cold declaration.
// The route relation is intentionally named here before the dependent-relation
// issuer is wired: the current shared ABI can seal Complete/Selected geometry,
// while FT-25 still owns vector argument and one-shot relation materialization.
package program

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

const (
	AxisKey     schema.Key = "placement"
	OutputKey   schema.Key = "placement/facts"
	RuleKey     schema.Key = "placement-containment"
	RuleRole               = "rule/placement/containment"
	OperandRole            = "operand/placement/containment"

	heapAxisKey schema.Key = "heap"

	ContainmentCandidates       schema.Key = "placement/containment/candidates"
	ContainmentPlacementSummary schema.Key = "placement/containment/placement-summary"
	ContainmentPlacementKey     schema.Key = "placement/containment/placement-summary-coordinate"
	ContainmentHeapSummary      schema.Key = "heap/containment/heap-summary"
	ContainmentHeapKey          schema.Key = "heap/containment/heap-summary-coordinate"
	ContainmentRoutes           schema.Key = "placement/containment/routes"
	ContainmentRouteKey         schema.Key = "placement/containment/route-key"
	ContainmentRouteSelection   schema.Key = "placement/containment/route-selection"
	ContainmentRouteTag         schema.Key = "placement/containment/route-tag"
	ContainmentRouteDestination schema.Key = "placement/containment/route-destination"
	ContainmentReducer          schema.Key = "placement/containment/reducer"
)

func RuleEntry() rule.Spec {
	return rule.Spec{
		Key: RuleKey, Writes: AxisKey, Owner: AxisKey, Lane: rule.LaneMountedPoint,
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole)},
		Program:  PlacementContainment(),
	}
}

func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(RuleRole, OperandRole)
}

func axisReference(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

func denominatorReference(key schema.Key) ruleprogram.DenominatorRef {
	return ruleprogram.DenominatorRef{Surface: schema.SurfaceKindDenominator, Key: key}
}

// PlacementContainment returns the sealed three-join geometry.  The first two joins
// are complete owner vectors; the third is the single selected/routed
// relation.  No route or Unknown value is fabricated by this declaration.
func PlacementContainment() ruleprogram.Program {
	placementAxis := axisReference(AxisKey)
	heapAxis := axisReference(heapAxisKey)
	candidate := member.RelationRef{Axis: placementAxis, Member: ContainmentCandidates}
	return ruleprogram.Program{
		OperandRole: vocabulary.RoleKey(OperandRole),
		Candidate:   member.AxisRelationCandidate(candidate),
		Joins: []ruleprogram.JoinDecl{
			{
				Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation: member.RelationRef{Axis: placementAxis, Member: ContainmentPlacementSummary},
				Key:      member.ProjectionRef{Axis: placementAxis, Member: ContainmentPlacementKey},
				Read: ruleprogram.ReadDecl{
					Input: 0, Axis: ruleprogram.AxisRef(placementAxis), Form: ruleprogram.Complete,
					PointBound: ruleprogram.PointBound,
					Contract: ruleprogram.ReadContract{
						Order: ruleprogram.OrderCanonical, Sparse: ruleprogram.SparseDense,
						OnOpaque: ruleprogram.OnOpaqueRefuse, Multiplicity: ruleprogram.MultiplicityMany,
						DenominatorRef: denominatorReference("coordinates/placement"),
					},
				},
			},
			{
				Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation: member.RelationRef{Axis: heapAxis, Member: ContainmentHeapSummary},
				Key:      member.ProjectionRef{Axis: heapAxis, Member: ContainmentHeapKey},
				Read: ruleprogram.ReadDecl{
					Input: 0, Axis: ruleprogram.AxisRef(heapAxis), Form: ruleprogram.Complete,
					PointBound: ruleprogram.PointBound,
					Contract: ruleprogram.ReadContract{
						Order: ruleprogram.OrderCanonical, Sparse: ruleprogram.SparseDense,
						OnOpaque: ruleprogram.OnOpaqueRefuse, Multiplicity: ruleprogram.MultiplicityMany,
						DenominatorRef: denominatorReference("coordinates/heap"),
					},
				},
			},
			{
				Sources: []ruleprogram.SourceRef{
					ruleprogram.CandidateSource(), ruleprogram.PriorSource(0), ruleprogram.PriorSource(1),
				},
				Relation:  member.RelationRef{Axis: placementAxis, Member: ContainmentRoutes},
				Key:       member.ProjectionRef{Axis: placementAxis, Member: ContainmentRouteKey},
				Selection: member.SelectionRef{Axis: placementAxis, Member: ContainmentRouteSelection},
				Read: ruleprogram.ReadDecl{
					Input: 0, Axis: ruleprogram.AxisRef(placementAxis), Form: ruleprogram.Selected,
					PointBound: ruleprogram.PointBound,
					Contract: ruleprogram.ReadContract{
						Order: ruleprogram.OrderByTag, Sparse: ruleprogram.SparseExplicit,
						OnOpaque: ruleprogram.OnOpaqueRefuse, Multiplicity: ruleprogram.MultiplicityOne,
						DenominatorRef: denominatorReference("coordinates/placement"),
					},
				},
			},
		},
		Fold: ruleprogram.FoldDecl{
			Reducer: member.ReducerRef{Axis: placementAxis, Member: ContainmentReducer},
			Inputs:  []ruleprogram.JoinRef{0, 1, 2},
			Outputs: []ruleprogram.OutputDecl{{
				Column:      axis.OutputRef{Axis: placementAxis, Key: OutputKey},
				Destination: member.ProjectionRef{Axis: placementAxis, Member: ContainmentRouteDestination},
				Mode:        ruleprogram.ModeRoute, ValueSlot: 0, RouteJoin: 2, RouteJoinPresent: true,
			}},
		},
	}
}
