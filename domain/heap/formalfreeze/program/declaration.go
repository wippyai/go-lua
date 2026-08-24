// Package program owns the Heap formal-freeze rule's callback-free
// declaration.
//
// It is deliberately separate from the freeze reducer/derivation package. It
// names the foreign mounted-call candidate, the exact Call fact read, the
// selected mounted-actual member set, the dependent Heap route relation, and
// the routed Heap publication. It contains no engine slot, runtime callback,
// route planner, or compatibility path. The declaration is the cold half of
// the freeze family; heap.FormalFreezeFact remains the one domain reducer
// implementation named by Heap's member contribution.
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

// The freeze family identities. These are declaration keys, not runtime
// handles: composition resolves them against the sealed axis/member surfaces.
const (
	AxisKey     schema.Key = "heap"
	OutputKey   schema.Key = "heap/facts"
	RuleKey     schema.Key = "heap-formal-freeze"
	RuleRole               = "rule/heap/formal-freeze"
	OperandRole            = "operand/heap/formal-freeze"

	// Heap owns the route relation and its two projections. Call owns the
	// mounted-call candidate directory and its key; Value owns the actual
	// member set, its coordinate and its selection tag. Naming the foreign
	// members through their own axis packages keeps every declaration law able
	// to see which owner published each row.
	FormalFreezeRoutes           schema.Key = heapdomain.FormalFreezeRoutes
	FormalFreezeRouteKey         schema.Key = heapdomain.FormalFreezeRouteKey
	FormalFreezeRouteDestination schema.Key = heapdomain.FormalFreezeRouteDestination
	FormalFreezeReducer          schema.Key = heapdomain.FormalFreezeReducer
	MountedCallCandidates        schema.Key = calldomain.MountedCallCandidates
	MountedCallFacts             schema.Key = calldomain.MountedCallFacts
	MountedCallFactKey           schema.Key = calldomain.MountedCallFactKey
	FormalFreezeActualMembers    schema.Key = valuedomain.FormalFreezeActualMembers
	FormalFreezeActualKey        schema.Key = valuedomain.FormalFreezeActualKey
	FormalFreezeActualTag        schema.Key = valuedomain.FormalFreezeActualTag
)

func axisReference(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

func denominatorReference(key schema.Key) ruleprogram.DenominatorRef {
	return ruleprogram.DenominatorRef{Surface: schema.SurfaceKindDenominator, Key: key}
}

// RuleEntry is the canonical callback-free freeze rule declaration. The freeze
// family is installed through the generated RuleFamily seam; this value is
// what Program composition consumes.
func RuleEntry() rule.Spec {
	return rule.Spec{
		Key:      RuleKey,
		Writes:   AxisKey,
		Owner:    AxisKey,
		Issues:   []rule.Issuance{{Occurrence: "occurrence/call", Requirement: "program-requirement/unrestricted", Form: "program-form/call-effect"}},
		Lane:     rule.LaneMounted,
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole)},
		Program:  FormalFreeze(),
	}
}

// StructureSpecs contributes the freeze consumer's rule and operand semantic
// roles.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(RuleRole, OperandRole)
}

// FormalFreeze returns the immutable freeze rule declaration.
//
// Join 0 is the one exact Call fact read, keyed at the coordinate Call already
// projects for the mounted-call candidate. Join 1 is that call's ordered
// mounted actuals: a selected read over the Value denominator, tagged by the
// owner-issued actual tag, so which actuals the call has is Value's published
// member set rather than geometry this rule re-walks. Join 2 is the dependent
// Heap route relation, which consumes the candidate and both prior joins and
// performs one selected read over the Heap denominator.
//
// Only join 2 is a fold argument. Joins 0 and 1 are PREREQUISITES - the
// materialization the route relation depends on - and a prerequisite is not an
// argument: naming one would hand the reducer a carrier it has no parameter
// for. The carry is identity because a routed row must preserve every
// coordinate of the output Factor its routes did not select.
func FormalFreeze() ruleprogram.Program {
	callAxis := axisReference("call")
	valueAxis := axisReference("value")
	heapAxis := axisReference(AxisKey)
	valueDenominator := denominatorReference("coordinates/value")
	heapDenominator := denominatorReference("coordinates/heap")
	return ruleprogram.Program{
		OperandRole: vocabulary.RoleKey(OperandRole),
		Candidate: member.RelationRef{
			Axis:   callAxis,
			Member: MountedCallCandidates,
		},
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
				// One cell per authored actual, at Value's declared default
				// where the actual's coordinate is unwritten: the freeze
				// judgment reads a value at every actual, and an absent one is
				// Value's default rather than a presence distinction this rule
				// would have to draw.
				Sources:   []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation:  member.RelationRef{Axis: valueAxis, Member: FormalFreezeActualMembers},
				Key:       member.ProjectionRef{Axis: valueAxis, Member: FormalFreezeActualKey},
				Predicate: member.ProjectionRef{Axis: valueAxis, Member: FormalFreezeActualTag},
				Read: ruleprogram.ReadDecl{
					Input: 1,
					Axis:  ruleprogram.AxisRef(valueAxis),
					Form:  ruleprogram.Selected,
					// The freeze judgment's own actual-member selection has
					// no predecessor point: it resolves through Value's
					// selected member set at this Input, not a transported
					// occurrence. Input 1's slot shares the candidate's own
					// point.
					PointBound: ruleprogram.PointBoundSelf,
					Contract: ruleprogram.ReadContract{
						Order:          ruleprogram.OrderByTag,
						Sparse:         ruleprogram.SparseDefault,
						OnOpaque:       ruleprogram.OnOpaqueRefuse,
						Multiplicity:   ruleprogram.MultiplicityOne,
						DenominatorRef: valueDenominator,
					},
				},
			},
			{
				// The Heap world at each route the relation answered. Heap's
				// declared default is Bottom and the freeze judgment publishes
				// the same empty normal image for a Bottom predecessor as for an
				// unwritten one, so this read carries the Factor default too.
				Sources: []ruleprogram.SourceRef{
					ruleprogram.CandidateSource(),
					ruleprogram.PriorSource(0),
					ruleprogram.PriorSource(1),
				},
				Relation: member.RelationRef{Axis: heapAxis, Member: FormalFreezeRoutes},
				Key:      member.ProjectionRef{Axis: heapAxis, Member: FormalFreezeRouteKey},
				Read: ruleprogram.ReadDecl{
					Input: 2,
					Axis:  ruleprogram.AxisRef(heapAxis),
					Form:  ruleprogram.Selected,
					// Resolved through Heap's own route/directory surface at
					// this Input, not a transported occurrence. Input 2's
					// slot shares the candidate's own point, the same as
					// Input 1.
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
			Reducer: member.ReducerRef{
				Axis:   heapAxis,
				Member: FormalFreezeReducer,
			},
			Inputs: []ruleprogram.JoinRef{2},
			Outputs: []ruleprogram.OutputDecl{
				{
					Column: axis.OutputRef{
						Axis: heapAxis,
						Key:  OutputKey,
					},
					Destination: member.ProjectionRef{
						Axis:   heapAxis,
						Member: FormalFreezeRouteDestination,
					},
					Mode:             ruleprogram.ModeRoute,
					ValueSlot:        0,
					RouteJoin:        2,
					RouteJoinPresent: true,
				},
			},
		},
		Carry: &ruleprogram.CarryDecl{
			Input: 2,
			Mode:  ruleprogram.CarryIdentity,
		},
	}
}
