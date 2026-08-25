// Package program owns the interprocedural call site's callback-free rule declaration.
//
// It names the sealed Effect mounted-call candidate, the one exact foreign
// Call read that candidate's occurrence addresses, and the exact Effect
// publication at the candidate's own Root. It contains no engine slot, runtime
// callback, or compatibility path; the judgment itself stays in
// domain/effect/callsite, which both readings of a call target share.
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
	effectdomain "github.com/wippyai/go-lua/domain/effect"
)

// The the interprocedural call site family identities. These are declaration keys, not runtime handles:
// composition resolves them against the sealed axis/member surfaces.
const (
	AxisKey     schema.Key = "effect"
	OutputKey   schema.Key = "effect/facts"
	RuleKey     schema.Key = "effect-body"
	RuleRole               = "rule/effect/callsite-body"
	OperandRole            = "operand/effect/callsite-body"

	callAxisKey schema.Key = "call"

	// Effect owns the mounted-call directory and the Root every site
	// publishes at. Call owns the fact relation this rule reads and the key
	// that addresses it; these aliases keep the foreign side explicit.
	MountedEffectCallCandidates schema.Key = effectdomain.MountedEffectCallCandidates
	MountedEffectCallCoordinate schema.Key = effectdomain.MountedEffectCallCoordinate
	MountedEffectCallSites      schema.Key = calldomain.MountedEffectCallSites
	MountedEffectCallSiteKey    schema.Key = calldomain.MountedEffectCallSiteKey
	BodyRoutes                  schema.Key = effectdomain.BodyRoutes
	BodyRouteKey                schema.Key = effectdomain.BodyRouteKey
	BodyRouteTag                schema.Key = effectdomain.BodyRouteTag
	BodyRouteSelection          schema.Key = effectdomain.BodyRouteSelection
	Reducer                     schema.Key = effectdomain.BodyCallEffectReducer
)

func axisReference(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

// RuleIssues is this rule's mounted issuance geometry: one call occurrence,
// published as the call's own effect.
func RuleIssues() []rule.Issuance {
	return []rule.Issuance{
		{Occurrence: "occurrence/call", Requirement: "program-requirement/unrestricted", Form: "program-form/call-effect"},
	}
}

// RuleEntry is the canonical callback-free the interprocedural call site rule declaration. The family is
// installed through the generated RuleFamily seam; this value is what Program
// composition consumes.
func RuleEntry() rule.Spec {
	return rule.Spec{
		Key:      RuleKey,
		Writes:   AxisKey,
		Owner:    AxisKey,
		Issues:   RuleIssues(),
		Lane:     rule.LaneMounted,
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole)},
		Program:  BodyCallEffect(),
	}
}

// StructureSpecs contributes this rule's own semantic roles. The Effect factor
// role is the axis owner's declaration and is therefore not re-authored here.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(RuleRole, OperandRole)
}

// BodyCallEffect returns the immutable the interprocedural call site rule declaration.
//
// The candidate is Effect's canonical mounted call site. Join 0 is the one
// exact foreign Call read: Call's own mounted-call directory enumerates the
// same sites under its own order, and the occurrence both directories are
// addressed by resolves the row. The publication is exact at the candidate's
// own Root, so Effect's relation owner projects it and the row carries no
// predecessor world.
func BodyCallEffect() ruleprogram.Program {
	effectAxis := axisReference(AxisKey)
	callAxis := axisReference(callAxisKey)
	return ruleprogram.Program{
		OperandRole: vocabulary.RoleKey(OperandRole),
		Candidate: member.AxisRelationCandidate(member.RelationRef{
			Axis:   effectAxis,
			Member: MountedEffectCallCandidates,
		}),
		Joins: []ruleprogram.JoinDecl{
			{
				Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation: member.RelationRef{Axis: callAxis, Member: MountedEffectCallSites},
				Key:      member.ProjectionRef{Axis: callAxis, Member: MountedEffectCallSiteKey},
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
				Relation:  member.RelationRef{Axis: effectAxis, Member: BodyRoutes},
				Key:       member.ProjectionRef{Axis: effectAxis, Member: BodyRouteKey},
				Predicate: member.ProjectionRef{Axis: effectAxis, Member: BodyRouteTag},
				Selection: member.SelectionRef{Axis: effectAxis, Member: BodyRouteSelection},
				Read: ruleprogram.ReadDecl{
					PointBound: ruleprogram.PointBound,
					Input:      0,
					Axis:       ruleprogram.AxisRef(effectAxis),
					Form:       ruleprogram.Selected,
					Contract: ruleprogram.ReadContract{
						Order:          ruleprogram.OrderByTag,
						Sparse:         ruleprogram.SparseExplicit,
						OnOpaque:       ruleprogram.OnOpaqueRefuse,
						Multiplicity:   ruleprogram.MultiplicityOne,
						DenominatorRef: ruleprogram.DenominatorRef{Surface: schema.SurfaceKindDenominator, Key: "coordinates/effect"},
					},
				},
			},
		},
		Fold: ruleprogram.FoldDecl{
			Reducer: member.ReducerRef{Axis: effectAxis, Member: Reducer},
			Inputs:  []ruleprogram.JoinRef{1},
			Outputs: []ruleprogram.OutputDecl{{
				Column: axis.OutputRef{
					Axis: effectAxis,
					Key:  OutputKey,
				},
				Destination: member.ProjectionRef{Axis: effectAxis, Member: MountedEffectCallCoordinate},
				Mode:        ruleprogram.ModeExact,
				ValueSlot:   0,
			}},
		},
	}
}
