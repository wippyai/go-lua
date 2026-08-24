// Package program owns Value binary equality's callback-free Rule Program
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

	RuleKey     schema.Key = "value-binary-equality"
	RuleRole               = "rule/value/binary-equality"
	OperandRole            = "operand/value/binary-equality"

	BinaryEqualityCandidates schema.Key = "value/binary-equality/candidates"
	BinaryEqualitySources    schema.Key = "value/binary-equality/sources"
	BinaryEqualityLeft       schema.Key = "value/binary-equality/left"
	BinaryEqualityRight      schema.Key = "value/binary-equality/right"
	BinaryEqualityWrite      schema.Key = "value/binary-equality/write"
	BinaryEqualityReducer    schema.Key = "value/binary-equality/reducer"
)

// RuleIssues is Value equality's mounted occurrence issuance. The rule
// package owns no engine registration; composition consumes this declaration
// and its issuance rows directly.
func RuleIssues() []rule.Issuance {
	return []rule.Issuance{{
		Occurrence:  "occurrence/binary-equality",
		Requirement: "program-requirement/unrestricted",
		Form:        "program-form/computation",
	}}
}

// RuleEntry is the child-package rule declaration consumed by composition.
// Its Program is the sole source of equality join/fold geometry.
func RuleEntry() rule.Spec {
	return rule.Spec{
		Key:      RuleKey,
		Writes:   AxisKey,
		Owner:    AxisKey,
		Issues:   RuleIssues(),
		Lane:     rule.LaneMounted,
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole)},
		Program:  BinaryEquality(),
	}
}

// StructureSpecs contributes equality's rule and operand semantic roles to
// the structural vocabulary registry.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(RuleRole, OperandRole)
}

func axisReference(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

// BinaryEquality returns Value's complete equality Program:
//
//	candidate
//	    -> exact Value at the candidate's left coordinate
//	    -> exact Value at the candidate's right coordinate
//	    -> one Value write at the candidate's write coordinate
//
// Both joins consume the same predecessor input. Join ordinal and
// owner-issued Unit distinguish the two reads; no equality-specific
// denominator or candidate order is introduced here.
func BinaryEquality() ruleprogram.Program {
	valueAxis := axisReference(AxisKey)
	provider := member.RelationRef{Axis: valueAxis, Member: BinaryEqualityCandidates}
	return ruleprogram.Program{
		OperandRole: vocabulary.RoleKey(OperandRole),
		Candidate:   member.AxisRelationCandidate(provider),
		Joins: []ruleprogram.JoinDecl{
			{
				Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation: member.RelationRef{Axis: valueAxis, Member: BinaryEqualitySources},
				Key:      member.ProjectionRef{Axis: valueAxis, Member: BinaryEqualityLeft},
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
				Relation: member.RelationRef{Axis: valueAxis, Member: BinaryEqualitySources},
				Key:      member.ProjectionRef{Axis: valueAxis, Member: BinaryEqualityRight},
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
			Reducer: member.ReducerRef{Axis: valueAxis, Member: BinaryEqualityReducer},
			Inputs:  []ruleprogram.JoinRef{0, 1},
			Outputs: []ruleprogram.OutputDecl{{
				Column:      axis.OutputRef{Axis: valueAxis, Key: OutputKey},
				Destination: member.ProjectionRef{Axis: valueAxis, Member: BinaryEqualityWrite},
				Mode:        ruleprogram.ModeExact,
				ValueSlot:   0,
			}},
		},
		Carry: &ruleprogram.CarryDecl{Input: 0, Mode: ruleprogram.CarryIdentity},
	}
}

// Equality is a short declaration alias for callers that name the rule by
// its family rather than its operand type.
func Equality() ruleprogram.Program { return BinaryEquality() }
