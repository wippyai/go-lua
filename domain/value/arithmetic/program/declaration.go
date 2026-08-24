// Package program owns Value binary arithmetic's callback-free Rule Program
// declaration. The candidate and coordinate projections are Value-owned
// members; this package states only the two exact reads, the fold, output, and
// identity carry that compose them.
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
	AxisKey   schema.Key = "value"
	OutputKey schema.Key = "value/facts"

	RuleKey     schema.Key = "value-binary-arithmetic"
	RuleRole               = "rule/value/binary-arithmetic"
	OperandRole            = "operand/value/binary-arithmetic"

	BinaryArithmeticCandidates schema.Key = "value/binary-arithmetic/candidates"
	BinaryArithmeticSources    schema.Key = "value/binary-arithmetic/sources"
	BinaryArithmeticLeft       schema.Key = "value/binary-arithmetic/left"
	BinaryArithmeticRight      schema.Key = "value/binary-arithmetic/right"
	BinaryArithmeticWrite      schema.Key = "value/binary-arithmetic/write"
	BinaryArithmeticReducer    schema.Key = "value/binary-arithmetic/reducer"
)

// RuleIssues is Value arithmetic's mounted occurrence issuance. The rule
// package owns no engine registration; composition consumes this declaration
// and its issuance rows directly.
func RuleIssues() []rule.Issuance {
	return []rule.Issuance{{
		Occurrence:  "occurrence/binary-arithmetic",
		Requirement: "program-requirement/unrestricted",
		Form:        "program-form/computation",
	}}
}

// RuleEntry is the child-package rule declaration consumed by composition.
// Its Program is the sole source of arithmetic join/fold geometry.
func RuleEntry() rule.Spec {
	return rule.Spec{
		Key:      RuleKey,
		Writes:   AxisKey,
		Owner:    AxisKey,
		Issues:   RuleIssues(),
		Lane:     rule.LaneMounted,
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole)},
		Program:  BinaryArithmetic(),
	}
}

// StructureSpecs contributes arithmetic's rule and operand semantic roles to
// the structural vocabulary registry.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(RuleRole, OperandRole)
}

func axisReference(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

// BinaryArithmetic returns Value's complete arithmetic Program:
//
//	candidate
//	    -> exact Value at the candidate's left coordinate
//	    -> exact Value at the candidate's right coordinate
//	    -> one Value write at the candidate's write coordinate
//
// Both joins consume the same predecessor input. Join ordinal and owner-issued
// Unit distinguish the two reads; no arithmetic-specific denominator or
// candidate order is introduced here.
func BinaryArithmetic() ruleprogram.Program {
	valueAxis := axisReference(AxisKey)
	provider := member.RelationRef{Axis: valueAxis, Member: BinaryArithmeticCandidates}
	return ruleprogram.Program{
		OperandRole: vocabulary.RoleKey(OperandRole),
		Candidate:   ruleprogram.AxisRelationCandidate(provider),
		Joins: []ruleprogram.JoinDecl{
			{
				Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation: member.RelationRef{Axis: valueAxis, Member: BinaryArithmeticSources},
				Key:      member.ProjectionRef{Axis: valueAxis, Member: BinaryArithmeticLeft},
				Read: ruleprogram.ReadDecl{
					PointBound: ruleprogram.PointBound,
					Input:      0,
					Axis:       ruleprogram.AxisRef(valueAxis),
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
				Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation: member.RelationRef{Axis: valueAxis, Member: BinaryArithmeticSources},
				Key:      member.ProjectionRef{Axis: valueAxis, Member: BinaryArithmeticRight},
				Read: ruleprogram.ReadDecl{
					PointBound: ruleprogram.PointBound,
					Input:      0,
					Axis:       ruleprogram.AxisRef(valueAxis),
					Form:       ruleprogram.Exact,
					Contract: ruleprogram.ReadContract{
						Order:        ruleprogram.OrderCanonical,
						Sparse:       ruleprogram.SparseExplicit,
						OnOpaque:     ruleprogram.OnOpaqueRefuse,
						Multiplicity: ruleprogram.MultiplicityOne,
					},
				},
			},
		},
		Fold: ruleprogram.FoldDecl{
			Reducer: member.ReducerRef{Axis: valueAxis, Member: BinaryArithmeticReducer},
			Inputs:  []ruleprogram.JoinRef{0, 1},
			Outputs: []ruleprogram.OutputDecl{{
				Column:      axis.OutputRef{Axis: valueAxis, Key: OutputKey},
				Destination: member.ProjectionRef{Axis: valueAxis, Member: BinaryArithmeticWrite},
				Mode:        ruleprogram.ModeExact,
				ValueSlot:   0,
			}},
		},
		Carry: &ruleprogram.CarryDecl{Input: 0, Mode: ruleprogram.CarryIdentity},
	}
}

// Arithmetic is a short declaration alias for callers that name the rule by
// its family rather than its operand type.
func Arithmetic() ruleprogram.Program { return BinaryArithmetic() }
