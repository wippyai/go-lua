// Package program owns the Heap closed-allocation rule's callback-free
// declaration.
//
// It names the constructor directory the rule draws candidates from, the Heap
// world it extends, the Value operands it folds, and the allocation transition
// its carry applies. The fold itself stays in the rule package; this is the
// cold half, and it is what Program composition consumes.
package program

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// The closed-allocation family identities. These are declaration keys, not
// runtime handles: composition resolves them against the sealed axis and
// member surfaces.
const (
	AxisKey       schema.Key = "heap"
	OutputKey     schema.Key = "heap/facts"
	RuleKey       schema.Key = "heap-closed"
	RuleRole                 = "rule/heap/allocation-closed"
	OperandRole              = "operand/heap/allocation-closed"
	TransformRole            = "transform/heap/allocation-closed"

	ClosedAllocations             schema.Key = heapdomain.ClosedAllocations
	ClosedAllocationCoordinate    schema.Key = heapdomain.ClosedAllocationCoordinate
	ClosedAllocationPredecessors  schema.Key = heapdomain.ClosedAllocationPredecessors
	ClosedAllocationPredecessor   schema.Key = heapdomain.ClosedAllocationPredecessorKey
	ClosedAllocationReducer       schema.Key = heapdomain.ClosedAllocationReducer
	ClosedAllocationCarryTransfer schema.Key = heapdomain.ClosedAllocationCarryTransform
	ClosedOperandParents          schema.Key = valuedomain.ClosedOperandParents
	ClosedOperandCells            schema.Key = valuedomain.ClosedOperandCells
	ClosedOperandKey              schema.Key = valuedomain.ClosedOperandKey
)

func axisReference(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

func denominatorReference(key schema.Key) ruleprogram.DenominatorRef {
	return ruleprogram.DenominatorRef{Surface: schema.SurfaceKindDenominator, Key: key}
}

// RuleEntry is the canonical callback-free closed-allocation declaration.
func RuleEntry() rule.Spec {
	return rule.Spec{
		Key:    RuleKey,
		Writes: AxisKey,
		Owner:  AxisKey,
		Issues: []rule.Issuance{
			{Occurrence: "occurrence/allocation-closed", Requirement: "program-requirement/unrestricted", Form: "program-form/local-successor"},
		},
		Lane:     rule.LaneMounted,
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole), vocabulary.RoleKey(TransformRole)},
		Program:  ClosedAllocation(),
	}
}

// StructureSpecs contributes this rule's semantic roles.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(RuleRole, OperandRole, TransformRole)
}

// ClosedAllocation returns the immutable closed-allocation rule declaration.
//
// Join 0 is the Heap world this constructor extends, read exactly at the
// coordinate it writes. Join 1 is the constructor's operands: the whole vector
// of Value cells it consumes, spanned by the key vector Value publishes for
// this constructor.
//
// The span is a restatement, not a decision. WHICH coordinates a constructor
// reads is a fact in Value's own numbering - a coordinate's dense key is the
// position Value assigned it, and Heap, upstream of it, holds no index into it
// - so the vector is published by the Value row addressed by this
// constructor's own occurrence, and the read declares which directory it comes
// from rather than resolving members itself. A selection here would make this
// rule supply the operands, which means re-deriving the vector beside the one
// that already exists.
//
// Both joins are fold arguments: the fold concludes the one Heap world this
// constructor denotes from the world it extends and the values it applies. The
// carry is the owner-issued allocation transition, because a constructor
// publishes at a coordinate whose prior fact is its own predecessor.
func ClosedAllocation() ruleprogram.Program {
	heapAxis := axisReference(AxisKey)
	valueAxis := axisReference("value")
	valueDenominator := denominatorReference("coordinates/value")
	return ruleprogram.Program{
		OperandRole: vocabulary.RoleKey(OperandRole),
		Candidate: member.AxisRelationCandidate(member.RelationRef{
			Axis:   heapAxis,
			Member: ClosedAllocations,
		}),
		Joins: []ruleprogram.JoinDecl{
			{
				Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation: member.RelationRef{Axis: heapAxis, Member: ClosedAllocationPredecessors},
				Key:      member.ProjectionRef{Axis: heapAxis, Member: ClosedAllocationPredecessor},
				Read: ruleprogram.ReadDecl{
					Input:      0,
					Axis:       ruleprogram.AxisRef(heapAxis),
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
				Sources:   []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation:  member.RelationRef{Axis: valueAxis, Member: ClosedOperandCells},
				Key:       member.ProjectionRef{Axis: valueAxis, Member: ClosedOperandKey},
				KeyVector: member.RelationRef{Axis: valueAxis, Member: ClosedOperandParents},
				Read: ruleprogram.ReadDecl{
					Input: 1,
					Axis:  ruleprogram.AxisRef(valueAxis),
					Form:  ruleprogram.Summary,
					// The vector resolves through Value's own published span
					// at this Input rather than a transported occurrence, so
					// Input 1's slot shares the candidate's own point.
					PointBound: ruleprogram.PointBoundSelf,
					Contract: ruleprogram.ReadContract{
						Order:          ruleprogram.OrderCanonical,
						Sparse:         ruleprogram.SparseExplicit,
						OnOpaque:       ruleprogram.OnOpaqueRefuse,
						Multiplicity:   ruleprogram.MultiplicityMany,
						DenominatorRef: valueDenominator,
					},
				},
			},
		},
		Fold: ruleprogram.FoldDecl{
			Reducer: member.ReducerRef{Axis: heapAxis, Member: ClosedAllocationReducer},
			Inputs:  []ruleprogram.JoinRef{0, 1},
			Outputs: []ruleprogram.OutputDecl{{
				Column:      axis.OutputRef{Axis: heapAxis, Key: OutputKey},
				Destination: member.ProjectionRef{Axis: heapAxis, Member: ClosedAllocationCoordinate},
				Mode:        ruleprogram.ModeExact,
				ValueSlot:   0,
			}},
		},
		Carry: &ruleprogram.CarryDecl{
			Input:     0,
			Mode:      ruleprogram.CarryTransform,
			Transform: member.CarryTransformRef{Axis: heapAxis, Member: ClosedAllocationCarryTransfer},
		},
	}
}
