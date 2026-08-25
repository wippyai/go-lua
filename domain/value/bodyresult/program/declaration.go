// Package program owns the body-result call result's callback-free rule
// declaration.
//
// It names Value's sealed result-zero candidate, the one exact foreign Call
// read that candidate's occurrence addresses, the selection of first return
// members the bodies that fact dispatches to publish, and the exact Value
// publication at the call-result coordinate Value already issued. It contains
// no engine slot, runtime callback, or compatibility path; the judgment itself
// stays in domain/value/bodyresult.
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

// The body-result family identities. These are declaration keys, not runtime
// handles: composition resolves them against the sealed axis/member surfaces.
const (
	AxisKey     schema.Key = "value"
	OutputKey   schema.Key = "value/facts"
	RuleKey     schema.Key = "value-callresult-body"
	RuleRole               = "rule/value/callresult-body"
	OperandRole            = "operand/value/callresult-body"

	callAxisKey schema.Key = "call"

	// Value owns the result-zero directory, the return route set it selects
	// over, and the call-result coordinate it publishes at. Call owns the fact
	// relation this rule reads and the key that addresses it; these aliases
	// keep the foreign side explicit.
	MountedCallResultSlotCandidates schema.Key = valuedomain.MountedCallResultSlotCandidates
	MountedCallResultSlotCoordinate schema.Key = valuedomain.MountedCallResultSlotCoordinate
	BodyReturnRoutes                schema.Key = valuedomain.BodyReturnRoutes
	BodyReturnRouteKey              schema.Key = valuedomain.BodyReturnRouteKey
	BodyReturnRouteTag              schema.Key = valuedomain.BodyReturnRouteTag
	BodyReturnRouteSelection        schema.Key = valuedomain.BodyReturnRouteSelection
	CallResultSlotSites             schema.Key = calldomain.CallResultSlotSites
	CallResultSlotSiteKey           schema.Key = calldomain.CallResultSlotSiteKey
	Reducer                         schema.Key = valuedomain.BodyResultReducer
)

func axisReference(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

// RuleIssues is this rule's mounted issuance geometry: one call occurrence
// that owns a fixed valued result-zero slot.
func RuleIssues() []rule.Issuance {
	return []rule.Issuance{{
		Occurrence:  "occurrence/call",
		Requirement: "program-requirement/call-result-slot",
		Form:        "program-form/call-summary",
	}}
}

// RuleEntry is the canonical callback-free body-result rule declaration. The
// family is installed through the generated RuleFamily seam; this value is
// what Program composition consumes.
func RuleEntry() rule.Spec {
	return rule.Spec{
		Key:      RuleKey,
		Writes:   AxisKey,
		Owner:    AxisKey,
		Issues:   RuleIssues(),
		Lane:     rule.LaneMounted,
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole)},
		Program:  BodyCallResult(),
	}
}

// StructureSpecs contributes this rule's own semantic roles. The Value factor
// role is the axis owner's declaration and is therefore not re-authored here.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(RuleRole, OperandRole)
}

// BodyCallResult returns the immutable body-result rule declaration.
//
// The candidate is Value's sealed result-zero slot. Join 0 is the one exact
// foreign Call read: Call's own mounted-call directory enumerates the same
// sites under its own order, and the occurrence both directories are addressed
// by resolves the row. Join 1 is the return route set derived from that
// candidate and that fact, observed at the canonical first return member of
// every body the call reaches. The publication is exact at the candidate's own
// call-result coordinate, so Value's relation owner projects it, and the
// identity carry keeps the predecessor world of that coordinate.
func BodyCallResult() ruleprogram.Program {
	valueAxis := axisReference(AxisKey)
	callAxis := axisReference(callAxisKey)
	return ruleprogram.Program{
		OperandRole: vocabulary.RoleKey(OperandRole),
		Candidate: member.AxisRelationCandidate(member.RelationRef{
			Axis:   valueAxis,
			Member: MountedCallResultSlotCandidates,
		}),
		Joins: []ruleprogram.JoinDecl{
			{
				Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation: member.RelationRef{Axis: callAxis, Member: CallResultSlotSites},
				Key:      member.ProjectionRef{Axis: callAxis, Member: CallResultSlotSiteKey},
				Read: ruleprogram.ReadDecl{
					PointBound: ruleprogram.PointBound,
					Input:      0,
					Axis:       ruleprogram.AxisRef(callAxis),
					Form:       ruleprogram.Exact,
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
				Relation:  member.RelationRef{Axis: valueAxis, Member: BodyReturnRoutes},
				Key:       member.ProjectionRef{Axis: valueAxis, Member: BodyReturnRouteKey},
				Predicate: member.ProjectionRef{Axis: valueAxis, Member: BodyReturnRouteTag},
				Selection: member.SelectionRef{Axis: valueAxis, Member: BodyReturnRouteSelection},
				Read: ruleprogram.ReadDecl{
					PointBound: ruleprogram.PointBound,
					Input:      0,
					Axis:       ruleprogram.AxisRef(valueAxis),
					Form:       ruleprogram.Selected,
					Contract: ruleprogram.ReadContract{
						Order:          ruleprogram.OrderByTag,
						Sparse:         ruleprogram.SparseExplicit,
						OnOpaque:       ruleprogram.OnOpaqueRefuse,
						Multiplicity:   ruleprogram.MultiplicityOne,
						DenominatorRef: ruleprogram.DenominatorRef{Surface: schema.SurfaceKindDenominator, Key: "coordinates/value"},
					},
				},
			},
		},
		Fold: ruleprogram.FoldDecl{
			Reducer: member.ReducerRef{Axis: valueAxis, Member: Reducer},
			Inputs:  []ruleprogram.JoinRef{0, 1},
			Outputs: []ruleprogram.OutputDecl{{
				Column: axis.OutputRef{
					Axis: valueAxis,
					Key:  OutputKey,
				},
				Destination: member.ProjectionRef{Axis: valueAxis, Member: MountedCallResultSlotCoordinate},
				Mode:        ruleprogram.ModeExact,
				ValueSlot:   0,
			}},
		},
		Carry: &ruleprogram.CarryDecl{Input: 0, Mode: ruleprogram.CarryIdentity},
	}
}
