// Package program owns Placement fresh birth's callback-free Rule Program.
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
	RuleKey     schema.Key = "placement-fresh-birth"
	RuleRole               = "rule/placement/fresh-birth"
	OperandRole            = "operand/placement/fresh-birth"
)

const (
	freshCandidates  schema.Key = "value/fresh-result/candidates"
	freshFacts       schema.Key = "value/fresh-result/facts"
	freshFactKey     schema.Key = "value/fresh-result/fact-key"
	birthDestination schema.Key = "placement/fresh-birth/destination"
	birthReducer     schema.Key = "placement/fresh-birth/reducer"
	placementFacts   schema.Key = "placement/facts"
)

func axisRef(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

func candidateProvider() member.RelationRef {
	return member.RelationRef{Axis: axisRef("value"), Member: freshCandidates}
}

// RuleEntry is Link-scoped because Value's fresh-result directory supplies
// the occurrence inventory. There is no Program issuance row and no Heap-wide
// fallback seed.
func RuleEntry() rule.Spec {
	return rule.Spec{
		Key: RuleKey, Writes: "placement", Owner: "placement", Lane: rule.LaneLink,
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole)},
		Program:  FreshBirth(),
	}
}

func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(RuleRole, OperandRole)
}

// FreshBirth reads the canonical Value cell whose presence proves that the
// fresh candidate materialized and writes Placement's explicit Stack default
// at that candidate's owner-issued Heap key.
func FreshBirth() ruleprogram.Program {
	valueAxis := axisRef("value")
	placementAxis := axisRef("placement")
	return ruleprogram.Program{
		OperandRole: vocabulary.RoleKey(OperandRole),
		Candidate:   member.AxisRelationCandidate(candidateProvider()),
		Joins: []ruleprogram.JoinDecl{{
			Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
			Relation: member.RelationRef{Axis: valueAxis, Member: freshFacts},
			Key:      member.ProjectionRef{Axis: valueAxis, Member: freshFactKey},
			Read: ruleprogram.ReadDecl{
				Input: 0, Axis: ruleprogram.AxisRef(valueAxis), Form: ruleprogram.Exact,
				PointBound: ruleprogram.PointBound,
				Contract: ruleprogram.ReadContract{
					Order: ruleprogram.OrderCanonical, Sparse: ruleprogram.SparseExplicit,
					OnOpaque:     ruleprogram.OnOpaquePropagateAuthenticated,
					Multiplicity: ruleprogram.MultiplicityOne,
				},
			},
		}},
		Fold: ruleprogram.FoldDecl{
			Reducer: member.ReducerRef{Axis: placementAxis, Member: birthReducer},
			Inputs:  []ruleprogram.JoinRef{0},
			Outputs: []ruleprogram.OutputDecl{{
				Column:      axis.OutputRef{Axis: placementAxis, Key: placementFacts},
				Destination: member.ProjectionRef{Axis: placementAxis, Member: birthDestination},
				Mode:        ruleprogram.ModeExact, ValueSlot: 0,
			}},
		},
		Carry: &ruleprogram.CarryDecl{Input: 0, Mode: ruleprogram.CarryIdentity},
	}
}
