// Package program owns the Heap empty-allocation rule's callback-free
// declaration.
//
// It names the constructor directory the rule draws candidates from, the Heap
// predecessor world it folds over, and the allocation transition its carry
// applies. The fold itself stays in the rule package; this is the cold half,
// and it is what Program composition consumes.
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
	heapdomain "github.com/wippyai/go-lua/domain/heap"
)

// The empty-allocation family identities. These are declaration keys, not
// runtime handles: composition resolves them against the sealed axis and
// member surfaces.
const (
	AxisKey     schema.Key = "heap"
	OutputKey   schema.Key = "heap/facts"
	RuleKey     schema.Key = "heap-empty"
	RuleRole               = "rule/heap/allocation-empty"
	OperandRole            = "operand/heap/allocation-empty"
)

func axisReference(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

// RuleEntry is the canonical callback-free empty-allocation declaration.
func RuleEntry() rule.Spec {
	return rule.Spec{
		Key:    RuleKey,
		Writes: AxisKey,
		Owner:  AxisKey,
		Issues: []rule.Issuance{
			{Occurrence: "occurrence/allocation-empty", Requirement: "program-requirement/unrestricted", Form: "program-form/local-finish"},
		},
		Lane:     rule.LaneMounted,
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole)},
		Program:  EmptyAllocation(),
	}
}

// EmptyAllocation is the empty-allocation Program: the constructor directory
// the rule draws candidates from, the predecessor world it extends, and the
// allocation transition its carry applies.
//
// The one join is the fold's only argument: the fold concludes the one Heap
// world an empty constructor denotes from the world it extends. The carry is
// the owner-issued allocation transition, because a constructor publishes at a
// coordinate whose prior fact is its own predecessor.
func EmptyAllocation() ruleprogram.Program {
	heapAxis := axisReference(AxisKey)
	return ruleprogram.Program{
		OperandRole: vocabulary.RoleKey(OperandRole),
		Candidate: member.AxisRelationCandidate(member.RelationRef{
			Axis:   heapAxis,
			Member: heapdomain.EmptyAllocations,
		}),
		Joins: []ruleprogram.JoinDecl{{
			Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
			Relation: member.RelationRef{Axis: heapAxis, Member: heapdomain.EmptyAllocationPredecessors},
			Key:      member.ProjectionRef{Axis: heapAxis, Member: heapdomain.EmptyAllocationPredecessorKey},
			Read: ruleprogram.ReadDecl{
				PointBound: ruleprogram.PointBound,
				Input:      0,
				Axis:       ruleprogram.AxisRef(heapAxis),
				Form:       ruleprogram.Exact,
				Contract: ruleprogram.ReadContract{
					Order:        ruleprogram.OrderCanonical,
					Sparse:       ruleprogram.SparseExplicit,
					OnOpaque:     ruleprogram.OnOpaqueRefuse,
					Multiplicity: ruleprogram.MultiplicityOne,
				},
			},
		}},
		Fold: ruleprogram.FoldDecl{
			Reducer: member.ReducerRef{Axis: heapAxis, Member: heapdomain.EmptyAllocationReducer},
			Inputs:  []ruleprogram.JoinRef{0},
			Outputs: []ruleprogram.OutputDecl{{
				Column:      axis.OutputRef{Axis: heapAxis, Key: OutputKey},
				Destination: member.ProjectionRef{Axis: heapAxis, Member: heapdomain.EmptyAllocationCoordinate},
				Mode:        ruleprogram.ModeExact,
				ValueSlot:   0,
			}},
		},
		Carry: &ruleprogram.CarryDecl{
			Input:     0,
			Mode:      ruleprogram.CarryTransform,
			Transform: member.CarryTransformRef{Axis: heapAxis, Member: heapdomain.EmptyAllocationCarryTransform},
			Output:    algebra.ScalarSource(algebra.NewSlotSource(0, 0)),
		},
	}
}

// StructureSpecs is this rule's contribution to the analyzer's semantic role
// vocabulary: the two roles it is identified by. A role is declared where it is
// used, so the row and the reference that names it are one package's statement.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(RuleRole, OperandRole)
}
