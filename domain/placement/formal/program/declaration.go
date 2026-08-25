// Package program owns Placement Formal's callback-free rule declaration.
//
// The hot implementation already fixes the judgment: Call is read exactly,
// mounted actual Value facts are selected, and Placement is selected and
// reduced into one routed Placement output. This package records that shape
// without importing an engine slot, a callback, a route planner, or a second
// fallback judgment. The member names below are intentionally local until
// the owning Call/Value/Placement catalogs publish the FT-25 seams.
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
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// The family identities. These are cold declaration keys, not runtime
// handles, and remain private to this package until the member surfaces are
// published by their owning domains.
const (
	AxisKey   schema.Key = "placement"
	OutputKey schema.Key = "placement/facts"
	RuleKey   schema.Key = "placement-formal"

	RuleRole    = "rule/placement/formal"
	OperandRole = "operand/placement/formal"
)

// The axis references are kept local so this child package does not add to a
// shared schema registry. Call owns the candidate directory; Value and
// Placement own the dependent relations that consume that foreign provider.
const (
	callAxisKey      schema.Key = "call"
	valueAxisKey     schema.Key = "value"
	placementAxisKey schema.Key = AxisKey
)

// The member seams Formal names. Placement owns the route relation and its
// three projections; Call owns the mounted-call candidate directory and its
// fact key; Value owns the actual member set, its parent row and its
// coordinate. Naming each through its own axis package keeps every
// declaration law able to see which owner published the row.
const (
	CallMountedCandidates schema.Key = calldomain.MountedCallCandidates
	CallMountedFacts      schema.Key = calldomain.MountedCallFacts
	CallMountedFactKey    schema.Key = calldomain.MountedCallFactKey

	FormalRoutes    schema.Key = placementdomain.FormalRoutes
	FormalRouteKey  schema.Key = placementdomain.FormalRouteKey
	FormalRouteTag  schema.Key = placementdomain.FormalRouteTag
	FormalRouteDest schema.Key = placementdomain.FormalRouteDestination
	FormalReducer   schema.Key = placementdomain.FormalReducer
)

func axisReference(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

func denominatorReference(key schema.Key) ruleprogram.DenominatorRef {
	return ruleprogram.DenominatorRef{Surface: schema.SurfaceKindDenominator, Key: key}
}

// RuleIssues is Formal's mounted call-effect issuance geometry. A fresh slice
// keeps one caller's inspection or mutation from changing the next RuleEntry.
func RuleIssues() []rule.Issuance {
	return []rule.Issuance{{
		Occurrence:  "occurrence/call",
		Requirement: "program-requirement/unrestricted",
		Form:        "program-form/call-effect",
	}}
}

// RuleEntry is the declaration-only identity of the mounted Formal consumer.
// Its Program is the canonical reducer judgment; no alternate plan is
// authored for opaque, absent, or compensation cases.
func RuleEntry() rule.Spec {
	return rule.Spec{
		Key:      RuleKey,
		Writes:   AxisKey,
		Owner:    AxisKey,
		Issues:   RuleIssues(),
		Lane:     rule.LaneMounted,
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole)},
		Program:  Formal(),
	}
}

// StructureSpecs contributes only Formal's semantic rule and operand roles.
// Placement's factor role remains owned by Placement's axis declaration.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(RuleRole, OperandRole)
}

// Formal returns the immutable cold declaration for the Formal J/WR family.
//
// Join 0 reads the mounted Call fact exactly. Join 1 is Value's canonical
// mounted-call member set: the parent directory identifies the call and the
// Summary read delivers its authored actuals in one closed vector. The
// member's ordinal is the correlation, so this join must not add a second
// selection-tag projection. Join 2 selects Placement route facts using the
// candidate and both preceding results, and carries the route's owner-issued
// tag beside its selected cell for the routed reducer.
//
// The route tag is not a second authority: it is the Placement-owned
// projection the route relation already declares, and the selected read must
// name it so the family ABI can pair the tag with the cell. The explicit
// RouteJoin on the output is the sole destination source. A routed predecessor
// already transports the untouched Placement state; an identity CarryDecl
// would publish the same state through a second authority and is therefore
// absent.
func Formal() ruleprogram.Program {
	callAxis := axisReference(callAxisKey)
	valueAxis := axisReference(valueAxisKey)
	placementAxis := axisReference(placementAxisKey)
	placementDenominator := denominatorReference("coordinates/placement")

	return ruleprogram.Program{
		OperandRole: vocabulary.RoleKey(OperandRole),
		Candidate: member.AxisRelationCandidate(member.RelationRef{
			Axis:   callAxis,
			Member: CallMountedCandidates,
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
				Sources: []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation: member.RelationRef{
					Axis:   valueAxis,
					Member: valuedomain.MountedCallActualMembers,
				},
				Key: member.ProjectionRef{
					Axis:   valueAxis,
					Member: valuedomain.MountedCallActualKey,
				},
				Parent: member.RelationRef{
					Axis:   valueAxis,
					Member: valuedomain.MountedCallParents,
				},
				Read: ruleprogram.ReadDecl{
					// Every read of this rule is bound to the one input port
					// the candidate arrives on. The port is what fixes the
					// point a read observes, and all three of these observe
					// the mounted call the candidate names: a Value read bound
					// to a second port would observe a point at which the
					// mounted actual's fact is not yet published, and answer
					// the Factor default in place of it.
					Input:      0,
					Axis:       ruleprogram.AxisRef(valueAxis),
					Form:       ruleprogram.Summary,
					PointBound: ruleprogram.PointBoundSelf,
					Contract: ruleprogram.ReadContract{
						Order:          ruleprogram.OrderCanonical,
						Sparse:         ruleprogram.SparseDefault,
						OnOpaque:       ruleprogram.OnOpaquePropagateAuthenticated,
						Multiplicity:   ruleprogram.MultiplicityMany,
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
					Member: FormalRoutes,
				},
				Key: member.ProjectionRef{
					Axis:   placementAxis,
					Member: FormalRouteKey,
				},
				Predicate: member.ProjectionRef{
					Axis:   placementAxis,
					Member: FormalRouteTag,
				},
				Read: ruleprogram.ReadDecl{
					Input: 0,
					Axis:  ruleprogram.AxisRef(placementAxis),
					Form:  ruleprogram.Selected,
					// Resolved through Placement's own route directory at this
					// Input, not a transported occurrence, the same as the
					// actual vector above.
					PointBound: ruleprogram.PointBoundSelf,
					Contract: ruleprogram.ReadContract{
						Order:          ruleprogram.OrderCanonical,
						Sparse:         ruleprogram.SparseDefault,
						OnOpaque:       ruleprogram.OnOpaqueRefuse,
						Multiplicity:   ruleprogram.MultiplicityOne,
						DenominatorRef: placementDenominator,
					},
				},
			},
		},
		Fold: ruleprogram.FoldDecl{
			Reducer: member.ReducerRef{
				Axis:   placementAxis,
				Member: FormalReducer,
			},
			Inputs: []ruleprogram.JoinRef{2},
			Outputs: []ruleprogram.OutputDecl{{
				Column: axis.OutputRef{
					Axis: placementAxis,
					Key:  OutputKey,
				},
				Destination: member.ProjectionRef{
					Axis:   placementAxis,
					Member: FormalRouteDest,
				},
				Mode:             ruleprogram.ModeRoute,
				ValueSlot:        0,
				RouteJoin:        2,
				RouteJoinPresent: true,
			}},
		},
	}
}
