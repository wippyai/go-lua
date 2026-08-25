// Package program owns the result-alias call result's callback-free rule
// declaration.
//
// It names Value's sealed result-zero candidate, the one exact foreign Call
// read that candidate's occurrence addresses, the selection of mounted actuals
// the operations that fact names alias the first result to, and the exact
// Value publication at the call-result coordinate Value already issued. It
// contains no engine slot, runtime callback, or compatibility path; the
// judgment itself stays in domain/value/resultalias.
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

// The result-alias family identities. These are declaration keys, not runtime
// handles: composition resolves them against the sealed axis/member surfaces.
const (
	AxisKey     schema.Key = "value"
	OutputKey   schema.Key = "value/facts"
	RuleKey     schema.Key = "value-callresult-resultalias"
	RuleRole               = "rule/value/callresult-resultalias"
	OperandRole            = "operand/value/callresult-resultalias"

	callAxisKey schema.Key = "call"

	// Value owns the result-zero directory, the alias route set it selects
	// over, and the call-result coordinate it publishes at. Call owns the fact
	// relation this rule reads and the key that addresses it; these aliases
	// keep the foreign side explicit.
	MountedCallResultSlotCandidates schema.Key = valuedomain.MountedCallResultSlotCandidates
	MountedCallResultSlotCoordinate schema.Key = valuedomain.MountedCallResultSlotCoordinate
	ResultAliasRoutes               schema.Key = valuedomain.ResultAliasRoutes
	ResultAliasRouteKey             schema.Key = valuedomain.ResultAliasRouteKey
	ResultAliasRouteTag             schema.Key = valuedomain.ResultAliasRouteTag
	CallResultSlotSites             schema.Key = calldomain.CallResultSlotSites
	CallResultSlotSiteKey           schema.Key = calldomain.CallResultSlotSiteKey
	Reducer                         schema.Key = valuedomain.ResultAliasReducer
)

func axisReference(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

// RuleIssues is this rule's mounted issuance geometry.
//
// Value seals exactly one ResultAlias operand per mounted call that owns a
// fixed valued result-zero slot, so the subscription names that requirement
// rather than the whole call family: a call with no valued result slot has no
// operand to resolve, and a placement for it is a construction the owner
// cannot answer.
func RuleIssues() []rule.Issuance {
	return []rule.Issuance{{
		Occurrence:  "occurrence/call",
		Requirement: "program-requirement/call-result-slot",
		Form:        "program-form/call-summary",
	}}
}

// RuleEntry is the canonical callback-free result-alias rule declaration. The
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
		Program:  ResultAliasCallResult(),
	}
}

// StructureSpecs contributes this rule's own semantic roles. The Value factor
// role is the axis owner's declaration and is therefore not re-authored here.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(RuleRole, OperandRole)
}

// ResultAliasCallResult returns the immutable result-alias rule declaration.
//
// The candidate is Value's sealed result-zero slot. Join 0 is the one exact
// foreign Call read: Call's own mounted-call directory enumerates the same
// sites under its own order, and the occurrence both directories are addressed
// by resolves the row. Join 1 is the alias route set derived from that
// candidate and that fact, observed at the mounted actual coordinates the
// selected Target operations name. The publication is exact at the candidate's
// own call-result coordinate, so Value's relation owner projects it, and the
// identity carry keeps the predecessor world of that coordinate.
func ResultAliasCallResult() ruleprogram.Program {
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
				Relation:  member.RelationRef{Axis: valueAxis, Member: ResultAliasRoutes},
				Key:       member.ProjectionRef{Axis: valueAxis, Member: ResultAliasRouteKey},
				Predicate: member.ProjectionRef{Axis: valueAxis, Member: ResultAliasRouteTag},
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
