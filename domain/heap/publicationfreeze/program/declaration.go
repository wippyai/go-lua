// Package program owns the Heap publication-freeze rule's callback-free
// declaration.
//
// It names Effect's mounted-call candidate, the exact foreign Call fact read
// that candidate's occurrence addresses, the selected Value coordinates this
// call's authored subjects resolve to, the dependent Heap route relation, and
// the routed Heap publication. It contains no engine slot, runtime callback,
// route planner, or compatibility path. The judgment itself stays in
// domain/heap: heap.PublicationFreezeFact is the one domain reducer named by
// the rule's member contribution.
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
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// The publication-freeze family identities. These are declaration keys, not
// runtime handles: composition resolves them against the sealed axis and
// member surfaces.
const (
	AxisKey     schema.Key = "heap"
	OutputKey   schema.Key = "heap/facts"
	RuleKey     schema.Key = "heap-publication-freeze"
	RuleRole               = "rule/heap/publication-freeze"
	OperandRole            = "operand/heap/publication-freeze"

	// Effect owns the mounted-call directory this rule's candidate is drawn
	// from. Call owns the fact relation and the key that addresses it. Value
	// owns the subject coordinates. Heap owns the route relation, its three
	// projections, and the reducer. Naming each foreign member through its own
	// axis package keeps every declaration law able to see which owner
	// published each row.
	MountedCallCandidates    schema.Key = calldomain.MountedCallCandidates
	MountedCallFacts         schema.Key = calldomain.MountedCallFacts
	MountedCallFactKey       schema.Key = calldomain.MountedCallFactKey
	MountedCallActualMembers schema.Key = valuedomain.MountedCallActualMembers
	MountedCallActualKey     schema.Key = valuedomain.MountedCallActualKey
	MountedCallParents       schema.Key = valuedomain.MountedCallParents

	PublicationFreezeRoutes           schema.Key = heapdomain.PublicationFreezeRoutes
	PublicationFreezeRouteKey         schema.Key = heapdomain.PublicationFreezeRouteKey
	PublicationFreezeRouteTag         schema.Key = heapdomain.PublicationFreezeRouteTag
	PublicationFreezeRouteDestination schema.Key = heapdomain.PublicationFreezeRouteDestination
	PublicationFreezeReducer          schema.Key = heapdomain.PublicationFreezeReducer
)

func axisReference(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

func denominatorReference(key schema.Key) ruleprogram.DenominatorRef {
	return ruleprogram.DenominatorRef{Surface: schema.SurfaceKindDenominator, Key: key}
}

// RuleEntry is the canonical callback-free publication-freeze rule
// declaration. The family is installed through the generated RuleFamily seam;
// this value is what Program composition consumes.
func RuleEntry() rule.Spec {
	return rule.Spec{
		Key:      RuleKey,
		Writes:   AxisKey,
		Owner:    AxisKey,
		Issues:   []rule.Issuance{{Occurrence: "occurrence/call", Requirement: "program-requirement/unrestricted", Form: "program-form/call-effect"}},
		Lane:     rule.LaneMounted,
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole)},
		Program:  PublicationFreeze(),
	}
}

// StructureSpecs contributes the publication-freeze consumer's rule and
// operand semantic roles.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(RuleRole, OperandRole)
}

// PublicationFreeze returns the immutable publication-freeze rule declaration.
//
// The candidate is Call's mounted call, the same directory Value's mounted
// actual member set declares its correspondence to, so join 1's vector is
// addressable at all. Join 0 is the one exact Call fact read, keyed at the
// coordinate Call already projects for that candidate. Which publications the
// call authored is Effect's, reached by the route relation through the Effect
// algebra it names as a static axis. Join 1 is the Value coordinate of every subject this call's authored
// FreezeSeal receipts name, selected and tagged by the tag Effect minted for
// each member. Join 2 is the dependent Heap route relation, which consumes the
// candidate and both prior joins and performs one selected read over the Heap
// denominator.
//
// Only join 2 is a fold argument. Joins 0 and 1 are PREREQUISITES - the
// materialization the route relation depends on - and a prerequisite is not an
// argument: naming one would hand the reducer a carrier it has no parameter
// for.
//
// The carry is identity. A routed row publishes at the members its routes
// selected, and every other coordinate of the output Factor must reach the
// next stage as the image this one was handed: without it the freeze stage
// contributes no Heap image at the coordinates it did not select, and a
// downstream judgment reads an image this rule silently dropped.
func PublicationFreeze() ruleprogram.Program {
	callAxis := axisReference("call")
	valueAxis := axisReference("value")
	heapAxis := axisReference(AxisKey)
	valueDenominator := denominatorReference("coordinates/value")
	heapDenominator := denominatorReference("coordinates/heap")
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
				// This call's ordered mounted actuals, delivered as the whole
				// vector Value publishes for the call - one cell per authored
				// actual, at Value's declared default where the actual's
				// coordinate is unwritten.
				//
				// A publication's subject is not a position in this vector and
				// is not assumed to be one: the derivation maps each authored
				// subject member to the actual ordinal that carries it, through
				// the Pack projection that owns that correspondence. A subject
				// that is no actual of this call settles as the empty valid
				// plan, which is the same answer this rule gives a subject
				// whose Value fact is not exact.
				Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation: member.RelationRef{Axis: valueAxis, Member: MountedCallActualMembers},
				Key:      member.ProjectionRef{Axis: valueAxis, Member: MountedCallActualKey},
				Parent:   member.RelationRef{Axis: valueAxis, Member: MountedCallParents},
				Read: ruleprogram.ReadDecl{
					Input:      1,
					Axis:       ruleprogram.AxisRef(valueAxis),
					Form:       ruleprogram.Summary,
					PointBound: ruleprogram.PointBoundSelf,
					Contract: ruleprogram.ReadContract{
						Order:          ruleprogram.OrderCanonical,
						Sparse:         ruleprogram.SparseDefault,
						OnOpaque:       ruleprogram.OnOpaqueRefuse,
						Multiplicity:   ruleprogram.MultiplicityMany,
						DenominatorRef: valueDenominator,
					},
				},
			},
			{
				// The Heap world at each route the relation answered. Heap's
				// declared default is Bottom and the freeze judgment publishes
				// the same empty normal image for a Bottom predecessor as for
				// an unwritten one, so this read carries the Factor default.
				Sources: []ruleprogram.SourceRef{
					ruleprogram.CandidateSource(),
					ruleprogram.PriorSource(0),
					ruleprogram.PriorSource(1),
				},
				Relation:  member.RelationRef{Axis: heapAxis, Member: PublicationFreezeRoutes},
				Key:       member.ProjectionRef{Axis: heapAxis, Member: PublicationFreezeRouteKey},
				Predicate: member.ProjectionRef{Axis: heapAxis, Member: PublicationFreezeRouteTag},
				Read: ruleprogram.ReadDecl{
					Input:      2,
					Axis:       ruleprogram.AxisRef(heapAxis),
					Form:       ruleprogram.Selected,
					PointBound: ruleprogram.PointBoundSelf,
					Contract: ruleprogram.ReadContract{
						Order:          ruleprogram.OrderCanonical,
						Sparse:         ruleprogram.SparseDefault,
						OnOpaque:       ruleprogram.OnOpaqueRefuse,
						Multiplicity:   ruleprogram.MultiplicityOne,
						DenominatorRef: heapDenominator,
					},
				},
			},
		},
		Fold: ruleprogram.FoldDecl{
			Reducer: member.ReducerRef{Axis: heapAxis, Member: PublicationFreezeReducer},
			Inputs:  []ruleprogram.JoinRef{2},
			Outputs: []ruleprogram.OutputDecl{{
				Column:           axis.OutputRef{Axis: heapAxis, Key: OutputKey},
				Destination:      member.ProjectionRef{Axis: heapAxis, Member: PublicationFreezeRouteDestination},
				Mode:             ruleprogram.ModeRoute,
				ValueSlot:        0,
				RouteJoin:        2,
				RouteJoinPresent: true,
			}},
		},
		Carry: &ruleprogram.CarryDecl{
			Input: 2,
			Mode:  ruleprogram.CarryIdentity,
		},
	}
}
