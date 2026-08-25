// Package program owns Call dispatch's callback-free rule declaration.
package program

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	calldomain "github.com/wippyai/go-lua/domain/call"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

const (
	AxisKey     schema.Key = "call"
	OutputKey   schema.Key = "call/facts"
	RuleKey     schema.Key = "call-dispatch"
	RuleRole               = "rule/call/dispatch"
	OperandRole            = "operand/call/dispatch"

	MountedCallCandidates    schema.Key = calldomain.MountedCallCandidates
	MountedCallParents       schema.Key = valuedomain.MountedCallParents
	MountedCallCalleeKey     schema.Key = valuedomain.MountedCallCalleeKey
	DispatchRoutes           schema.Key = calldomain.DispatchRoutes
	DispatchRouteKey         schema.Key = calldomain.DispatchRouteKey
	DispatchRouteTag         schema.Key = calldomain.DispatchRouteTag
	DispatchRouteDestination schema.Key = calldomain.DispatchRouteDestination
	DispatchReducer          schema.Key = calldomain.DispatchReducer
)

func axisReference(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

func denominatorReference(key schema.Key) ruleprogram.DenominatorRef {
	return ruleprogram.DenominatorRef{Surface: schema.SurfaceKindDenominator, Key: key}
}

func RuleEntry() rule.Spec {
	return rule.Spec{
		Key:      RuleKey,
		Writes:   AxisKey,
		Owner:    AxisKey,
		Issues:   []rule.Issuance{{Occurrence: "occurrence/call", Requirement: "program-requirement/unrestricted", Form: "program-form/call-dispatch"}},
		Lane:     rule.LaneMounted,
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole)},
		Program:  Dispatch(),
	}
}

func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(RuleRole, OperandRole)
}

// Dispatch declares two joins. The first reads Value's owner-issued callee
// coordinate. The second derives Call's bounded route relation from that fact
// and selects Call's own destination cell. The fold receives only the route
// selection and publishes one Call alternative; the lattice join combines
// exact targets and the explicit opaque/top disposition.
func Dispatch() ruleprogram.Program {
	callAxis := axisReference(AxisKey)
	valueAxis := axisReference("value")
	callDenominator := denominatorReference("coordinates/call")
	return ruleprogram.Program{
		OperandRole: vocabulary.RoleKey(OperandRole),
		Candidate: member.AxisRelationCandidate(member.RelationRef{
			Axis:   callAxis,
			Member: MountedCallCandidates,
		}),
		Joins: []ruleprogram.JoinDecl{
			{
				Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation: member.RelationRef{Axis: valueAxis, Member: MountedCallParents},
				Key:      member.ProjectionRef{Axis: valueAxis, Member: MountedCallCalleeKey},
				Read: ruleprogram.ReadDecl{
					Input:      0,
					Axis:       ruleprogram.AxisRef(valueAxis),
					Form:       ruleprogram.Exact,
					PointBound: ruleprogram.PointBound,
					Contract: ruleprogram.ReadContract{
						Order:        ruleprogram.OrderCanonical,
						Sparse:       ruleprogram.SparseExplicit,
						OnOpaque:     ruleprogram.OnOpaqueRefuse,
						Multiplicity: ruleprogram.MultiplicityOne,
					},
				},
			},
			{
				Sources:   []ruleprogram.SourceRef{ruleprogram.CandidateSource(), ruleprogram.PriorSource(0)},
				Relation:  member.RelationRef{Axis: callAxis, Member: DispatchRoutes},
				Key:       member.ProjectionRef{Axis: callAxis, Member: DispatchRouteKey},
				Predicate: member.ProjectionRef{Axis: callAxis, Member: DispatchRouteTag},
				Read: ruleprogram.ReadDecl{
					Input:      0,
					Axis:       ruleprogram.AxisRef(callAxis),
					Form:       ruleprogram.Selected,
					PointBound: ruleprogram.PointBound,
					Contract: ruleprogram.ReadContract{
						Order:          ruleprogram.OrderByTag,
						Sparse:         ruleprogram.SparseExplicit,
						OnOpaque:       ruleprogram.OnOpaqueRefuse,
						Multiplicity:   ruleprogram.MultiplicityOne,
						DenominatorRef: callDenominator,
					},
				},
			},
		},
		Fold: ruleprogram.FoldDecl{
			Reducer: member.ReducerRef{Axis: callAxis, Member: DispatchReducer},
			Inputs:  []ruleprogram.JoinRef{1},
			Outputs: []ruleprogram.OutputDecl{{
				Column:           axis.OutputRef{Axis: callAxis, Key: OutputKey},
				Destination:      member.ProjectionRef{Axis: callAxis, Member: DispatchRouteDestination},
				Mode:             ruleprogram.ModeRoute,
				ValueSlot:        0,
				RouteJoin:        1,
				RouteJoinPresent: true,
			}},
		},
	}
}
