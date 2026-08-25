// Package program owns authored allocation birth's callback-free Rule Program.
package program

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

const (
	RuleKey     schema.Key = "placement-allocation-birth"
	RuleRole               = "rule/placement/allocation-birth"
	OperandRole            = "operand/placement/allocation-birth"
)

func axisRef(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

func candidateProvider() member.RelationRef {
	return member.RelationRef{Axis: axisRef("value"), Member: "value/allocation/candidates"}
}

func RuleIssues() []rule.Issuance {
	return []rule.Issuance{{Occurrence: "occurrence/allocation", Requirement: "program-requirement/unrestricted", Form: "program-form/local-successor"}}
}

func RuleEntry() rule.Spec {
	return rule.Spec{
		Key: RuleKey, Writes: "placement", Owner: "placement", Lane: rule.LaneMounted,
		Issues: RuleIssues(), Semantic: vocabulary.RoleKey(RuleRole),
		Roles: []schema.Key{vocabulary.RoleKey(OperandRole)}, Program: AllocationBirth(),
	}
}

func StructureSpecs() []structure.Spec { return vocabulary.RoleSpecs(RuleRole, OperandRole) }

func AllocationBirth() ruleprogram.Program {
	valueAxis := axisRef("value")
	placementAxis := axisRef("placement")
	return ruleprogram.Program{
		OperandRole: vocabulary.RoleKey(OperandRole),
		Candidate:   member.AxisRelationCandidate(candidateProvider()),
		Joins: []ruleprogram.JoinDecl{{
			Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
			Relation: member.RelationRef{Axis: valueAxis, Member: "value/allocation/facts"},
			Key:      member.ProjectionRef{Axis: valueAxis, Member: "value/allocation/fact-key"},
			Read: ruleprogram.ReadDecl{
				Input: 0, Axis: ruleprogram.AxisRef(valueAxis), Form: ruleprogram.Exact,
				PointBound: ruleprogram.PointBound,
				Contract:   ruleprogram.ReadContract{Order: ruleprogram.OrderCanonical, Sparse: ruleprogram.SparseExplicit, OnOpaque: ruleprogram.OnOpaquePropagateAuthenticated, Multiplicity: ruleprogram.MultiplicityOne},
			},
		}},
		Fold: ruleprogram.FoldDecl{
			Reducer: member.ReducerRef{Axis: placementAxis, Member: "placement/allocation-birth/reducer"},
			Inputs:  []ruleprogram.JoinRef{0},
			Outputs: []ruleprogram.OutputDecl{{
				Column:      axis.OutputRef{Axis: placementAxis, Key: "placement/facts"},
				Destination: member.ProjectionRef{Axis: placementAxis, Member: "placement/allocation-birth/destination"},
				Mode:        ruleprogram.ModeExact, ValueSlot: 0,
			}},
		},
		Carry: &ruleprogram.CarryDecl{Input: 0, Mode: ruleprogram.CarryIdentity},
	}
}
