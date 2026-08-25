// Package program owns Placement Publication Escape's callback-free rule
// declaration.
//
// Effect's mounted call is the candidate. Its publication batch remains
// Effect-owned data reached by dependent relations. The existing hot judgment
// reads the Call gate exactly, selects Value receipt subjects, then selects and
// reduces Placement routes. This package records that dependency geometry and
// its strict read contracts while leaving the future member catalogs and
// runtime RouteMember binding to their owning seams.
package program

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	effectdomain "github.com/wippyai/go-lua/domain/effect"
)

// The family identities. They are local cold keys and do not mutate the root
// Placement, Effect, Call, or Value catalogs.
const (
	AxisKey   schema.Key = "placement"
	OutputKey schema.Key = "placement/facts"
	RuleKey   schema.Key = "placement-publication-escape"

	RuleRole    = "rule/placement/publication-escape"
	OperandRole = "operand/placement/publication-escape"
)

const (
	callAxisKey      schema.Key = "call"
	valueAxisKey     schema.Key = "value"
	placementAxisKey schema.Key = AxisKey
	effectAxisKey    schema.Key = "effect"
)

// Effect's canonical mounted-call row is the foreign provider for all three
// dependent relations. The publication batch is data reached from that row,
// never a second candidate directory. Call's exact fact is shared with the
// Formal family by spelling; the Value and Placement relations are Publication
// Escape's own future foreign-provider members. No root catalog entry is
// authored here.
const (
	EffectMountedCallCandidates schema.Key = effectdomain.MountedEffectCallCandidates
	CallMountedFacts            schema.Key = "call/mounted-call/facts"
	CallMountedFactKey          schema.Key = "call/mounted-call/fact-key"

	PublicationSources         schema.Key = "effect/mounted-publication/sources"
	PublicationSourceKey       schema.Key = "effect/mounted-publication/source-value-coordinate"
	PublicationSourceTag       schema.Key = "effect/mounted-publication/source-tag"
	PublicationSourceSelection schema.Key = "effect/publication-escape/source-selection"
	PublicationRoutes          schema.Key = "placement/publication-escape/routes"
	PublicationRouteKey        schema.Key = "placement/publication-escape/route-key"
	PublicationRouteSelection  schema.Key = "placement/publication-escape/route-selection"
	PublicationRouteDest       schema.Key = "placement/publication-escape/route-destination"
	PublicationReducer         schema.Key = "placement/publication-escape/reducer"
	// PublicationRouteDestination is the descriptive alias retained for
	// callers that name the deferred destination by its full role.
	PublicationRouteDestination schema.Key = PublicationRouteDest
)

func axisReference(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

func denominatorReference(key schema.Key) ruleprogram.DenominatorRef {
	return ruleprogram.DenominatorRef{Surface: schema.SurfaceKindDenominator, Key: key}
}

// RuleIssues returns the one mounted call-effect issuance form. The result is
// fresh so a caller cannot mutate a later declaration through shared storage.
func RuleIssues() []rule.Issuance {
	return []rule.Issuance{{
		Occurrence:  "occurrence/call",
		Requirement: "program-requirement/unrestricted",
		Form:        "program-form/call-effect",
	}}
}

// RuleEntry is the declaration-only identity of the mounted Publication
// Escape consumer.
func RuleEntry() rule.Spec {
	return rule.Spec{
		Key:      RuleKey,
		Writes:   AxisKey,
		Owner:    AxisKey,
		Issues:   RuleIssues(),
		Lane:     rule.LaneMounted,
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole)},
		Program:  PublicationEscape(),
	}
}

// StructureSpecs contributes only this family's semantic rule and operand
// roles; the Placement factor remains owned by the Placement axis package.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(RuleRole, OperandRole)
}

// PublicationEscape returns the immutable cold declaration for the WR family.
//
// Join 0 is the exact foreign Call fact gated by Effect's canonical mounted
// call candidate. Join 1 selects Value receipt subjects/contexts from Effect's
// dependent publication data and the Call result. Join 2 selects the Placement route facts from all
// preceding evidence. As in Formal, the route join has no independently
// authored Predicate: FT-25 RouteMember carries the selected tag and deferred
// strong destination as one owner-issued pair. The output names Join 2
// explicitly, and the identity carry preserves the input batch judgment.
func PublicationEscape() ruleprogram.Program {
	effectAxis := axisReference(effectAxisKey)
	callAxis := axisReference(callAxisKey)
	valueAxis := axisReference(valueAxisKey)
	placementAxis := axisReference(placementAxisKey)

	return ruleprogram.Program{
		OperandRole: vocabulary.RoleKey(OperandRole),
		Candidate: member.AxisRelationCandidate(member.RelationRef{
			Axis:   effectAxis,
			Member: EffectMountedCallCandidates,
		}),
		Joins: []ruleprogram.JoinDecl{
			{
				Sources: []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation: member.RelationRef{
					Axis:   callAxis,
					Member: CallMountedFacts,
				},
				Key: member.ProjectionRef{
					Axis:   callAxis,
					Member: CallMountedFactKey,
				},
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
				Relation: member.RelationRef{
					Axis:   effectAxis,
					Member: PublicationSources,
				},
				Key: member.ProjectionRef{
					Axis:   effectAxis,
					Member: PublicationSourceKey,
				},
				Predicate: member.ProjectionRef{
					Axis:   effectAxis,
					Member: PublicationSourceTag,
				},
				Selection: member.SelectionRef{Axis: effectAxis, Member: PublicationSourceSelection},
				Read: ruleprogram.ReadDecl{
					Input:      0,
					Axis:       ruleprogram.AxisRef(valueAxis),
					Form:       ruleprogram.Selected,
					PointBound: ruleprogram.PointBound,
					Contract: ruleprogram.ReadContract{
						Order:          ruleprogram.OrderCanonical,
						Sparse:         ruleprogram.SparseExplicit,
						OnOpaque:       ruleprogram.OnOpaqueRefuse,
						Multiplicity:   ruleprogram.MultiplicityOne,
						DenominatorRef: denominatorReference("coordinates/value"),
					},
				},
			},
			{
				Sources: []ruleprogram.SourceRef{
					ruleprogram.CandidateSource(),
					ruleprogram.PriorSource(0),
					ruleprogram.PriorSource(1),
				},
				Relation: member.RelationRef{
					Axis:   placementAxis,
					Member: PublicationRoutes,
				},
				Key: member.ProjectionRef{
					Axis:   placementAxis,
					Member: PublicationRouteKey,
				},
				// RouteMember supplies the paired tag and destination; a second
				// route predicate would be a duplicate route plan.
				Selection: member.SelectionRef{Axis: placementAxis, Member: PublicationRouteSelection},
				Read: ruleprogram.ReadDecl{
					Input:      0,
					Axis:       ruleprogram.AxisRef(placementAxis),
					Form:       ruleprogram.Selected,
					PointBound: ruleprogram.PointBound,
					Contract: ruleprogram.ReadContract{
						Order:          ruleprogram.OrderCanonical,
						Sparse:         ruleprogram.SparseExplicit,
						OnOpaque:       ruleprogram.OnOpaqueRefuse,
						Multiplicity:   ruleprogram.MultiplicityOne,
						DenominatorRef: denominatorReference("coordinates/placement"),
					},
				},
			},
		},
		Fold: ruleprogram.FoldDecl{
			Reducer: member.ReducerRef{
				Axis:   placementAxis,
				Member: PublicationReducer,
			},
			Inputs: []ruleprogram.JoinRef{2},
			Outputs: []ruleprogram.OutputDecl{{
				Column: axis.OutputRef{
					Axis: placementAxis,
					Key:  OutputKey,
				},
				Destination: member.ProjectionRef{
					Axis:   placementAxis,
					Member: PublicationRouteDest,
				},
				Mode:             ruleprogram.ModeRoute,
				ValueSlot:        0,
				RouteJoin:        2,
				RouteJoinPresent: true,
			}},
		},
		Carry: &ruleprogram.CarryDecl{
			Input: 0,
			Mode:  ruleprogram.CarryIdentity,
		},
	}
}
