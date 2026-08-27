// Package program owns the Value fresh-result rule's callback-free
// declaration: the mounted call it is issued at, the exact Call fact read that
// decides which arms the call admits, the derived route set over that call's
// fresh results, and the routed publication that carries each destination's
// image through its own recency transition.
package program

import (
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
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

// The fresh-result family identities. These are declaration keys, not runtime
// handles: composition resolves them against the sealed axis/member surfaces.
const (
	AxisKey     schema.Key = "value"
	OutputKey   schema.Key = "value/facts"
	RuleKey     schema.Key = "value-callresult-freshresult"
	RuleRole               = "rule/value/callresult-freshresult"
	OperandRole            = "operand/value/callresult-freshresult"

	MountedCallCandidates       schema.Key = calldomain.MountedCallCandidates
	MountedCallFacts            schema.Key = calldomain.MountedCallFacts
	MountedCallFactKey          schema.Key = calldomain.MountedCallFactKey
	FreshResultRoutes           schema.Key = valuedomain.FreshResultRoutes
	FreshResultRouteKey         schema.Key = valuedomain.FreshResultRouteKey
	FreshResultRouteTag         schema.Key = valuedomain.FreshResultRouteTag
	FreshResultRouteSelection   schema.Key = valuedomain.FreshResultRouteSelection
	FreshResultRouteDestination schema.Key = valuedomain.FreshResultRouteDestination
	FreshResultReducer          schema.Key = valuedomain.FreshResultReducer
	FreshResultRouteCarry       schema.Key = valuedomain.FreshResultRouteCarryTransform
)

func axisReference(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

func denominatorReference(key schema.Key) ruleprogram.DenominatorRef {
	return ruleprogram.DenominatorRef{Surface: schema.SurfaceKindDenominator, Key: key}
}

// RuleEntry is the canonical callback-free fresh-result rule declaration.
//
// It is issued at occurrence/call on the call-effect stage. A fresh result is
// created by a mounted call and what it is worth depends on which Target
// operation that call's own fact admits, so the rule has to observe a fact call
// dispatch published - and the stage graph is where that dependency is stated.
func RuleEntry() rule.Spec {
	return rule.Spec{
		Key:      RuleKey,
		Writes:   AxisKey,
		Owner:    AxisKey,
		Issues:   []rule.Issuance{{Occurrence: "occurrence/call", Requirement: "program-requirement/unrestricted", Form: "program-form/call-effect"}},
		Lane:     rule.LaneMounted,
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole)},
		Program:  FreshResult(),
	}
}

// StructureSpecs contributes the fresh-result rule's semantic roles.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(RuleRole, OperandRole)
}

// FreshResult returns the immutable fresh-result rule declaration.
//
// Join 0 is the one exact Call fact read, keyed at the coordinate Call already
// projects for the mounted-call candidate: it is the fact that says which
// Target operations this call site may reach. Join 1 is the dependent route
// relation, which consumes the candidate and that fact and performs one
// selected read over the Value denominator at every coordinate the call
// publishes a fresh result at.
//
// Only join 1 is a fold argument beside the call fact. The carry is a
// transform rather than the identity, because every root admitted at a
// destination has just been created: a reference that was Recent for one of
// them is Recent no longer, and that transition is issued by the route row
// rather than by the call.
func FreshResult() ruleprogram.Program {
	callAxis := axisReference("call")
	valueAxis := axisReference(AxisKey)
	valueDenominator := denominatorReference("coordinates/value")
	return ruleprogram.Program{
		OperandRole: vocabulary.RoleKey(OperandRole),
		Candidate: member.AxisRelationCandidate(member.RelationRef{
			Axis:   callAxis,
			Member: MountedCallCandidates,
		}),
		Joins: []ruleprogram.JoinDecl{
			{
				Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation: member.RelationRef{Axis: callAxis, Member: MountedCallFacts},
				Key:      member.ProjectionRef{Axis: callAxis, Member: MountedCallFactKey},
				Read: ruleprogram.ReadDecl{
					Input:      0,
					Axis:       ruleprogram.AxisRef(callAxis),
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
				Sources: []ruleprogram.SourceRef{
					ruleprogram.CandidateSource(),
					ruleprogram.PriorSource(0),
				},
				Relation:  member.RelationRef{Axis: valueAxis, Member: FreshResultRoutes},
				Key:       member.ProjectionRef{Axis: valueAxis, Member: FreshResultRouteKey},
				Predicate: member.ProjectionRef{Axis: valueAxis, Member: FreshResultRouteTag},
				Selection: member.SelectionRef{Axis: valueAxis, Member: FreshResultRouteSelection},
				Read: ruleprogram.ReadDecl{
					Input: 1,
					Axis:  ruleprogram.AxisRef(valueAxis),
					Form:  ruleprogram.Selected,
					// Resolved through Value's own route surface at this Input,
					// not a transported occurrence, so Input 1's slot shares the
					// candidate's own point.
					PointBound: ruleprogram.PointBoundSelf,
					Contract: ruleprogram.ReadContract{
						Order:          ruleprogram.OrderCanonical,
						Sparse:         ruleprogram.SparseDefault,
						OnOpaque:       ruleprogram.OnOpaqueRefuse,
						Multiplicity:   ruleprogram.MultiplicityOne,
						DenominatorRef: valueDenominator,
					},
				},
			},
		},
		Fold: ruleprogram.FoldDecl{
			Reducer: member.ReducerRef{
				Axis:   valueAxis,
				Member: FreshResultReducer,
			},
			Inputs: []ruleprogram.JoinRef{0, 1},
			Outputs: []ruleprogram.OutputDecl{
				{
					Column: axis.OutputRef{
						Axis: valueAxis,
						Key:  OutputKey,
					},
					Destination: member.ProjectionRef{
						Axis:   valueAxis,
						Member: FreshResultRouteDestination,
					},
					Mode:             ruleprogram.ModeRoute,
					ValueSlot:        0,
					RouteJoin:        1,
					RouteJoinPresent: true,
				},
			},
		},
		Carry: &ruleprogram.CarryDecl{
			Input:     1,
			Mode:      ruleprogram.CarryTransform,
			Transform: member.CarryTransformRef{Axis: valueAxis, Member: FreshResultRouteCarry},
			Output:    algebra.ScalarSource(algebra.NewSlotSource(0, 0)),
		},
	}
}
