// Package program owns Value binary order's callback-free Rule Program
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

	RuleKey     schema.Key = "value-binary-order"
	RuleRole               = "rule/value/binary-order"
	OperandRole            = "operand/value/binary-order"

	BinaryOrderCandidates schema.Key = "value/binary-order/candidates"
	BinaryOrderSources    schema.Key = "value/binary-order/sources"
	BinaryOrderLeft       schema.Key = "value/binary-order/left"
	BinaryOrderRight      schema.Key = "value/binary-order/right"
	BinaryOrderWrite      schema.Key = "value/binary-order/write"
	BinaryOrderReducer    schema.Key = "value/binary-order/reducer"
)

// RuleIssues is Value order's mounted occurrence issuance. The rule package
// owns no engine registration; composition consumes this declaration and its
// issuance rows directly.
func RuleIssues() []rule.Issuance {
	return []rule.Issuance{{
		Occurrence:  "occurrence/binary-order",
		Requirement: "program-requirement/unrestricted",
		Form:        "program-form/computation",
	}}
}

// RuleEntry is the child-package rule declaration consumed by composition.
// Its Program is the sole source of order join/fold geometry.
func RuleEntry() rule.Spec {
	return rule.Spec{
		Key:      RuleKey,
		Writes:   AxisKey,
		Owner:    AxisKey,
		Issues:   RuleIssues(),
		Lane:     rule.LaneMounted,
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole)},
		Program:  BinaryOrder(),
	}
}

// StructureSpecs contributes order's rule and operand semantic roles to the
// structural vocabulary registry.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(RuleRole, OperandRole)
}

func axisReference(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

// BinaryOrder returns Value's complete order Program:
//
//	candidate
//	    -> exact Value at the candidate's left coordinate
//	    -> exact Value at the candidate's right coordinate
//	    -> one Value write at the candidate's write coordinate
//
// Both joins consume the same predecessor input. Join ordinal and owner-issued
// Unit distinguish the two reads; no order-specific denominator or candidate
// order is introduced here.
func BinaryOrder() ruleprogram.Program {
	valueAxis := axisReference(AxisKey)
	provider := member.RelationRef{Axis: valueAxis, Member: BinaryOrderCandidates}
	return ruleprogram.Program{
		OperandRole: vocabulary.RoleKey(OperandRole),
		Candidate:   provider,
		Joins: []ruleprogram.JoinDecl{
			{
				Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation: member.RelationRef{Axis: valueAxis, Member: BinaryOrderSources},
				Key:      member.ProjectionRef{Axis: valueAxis, Member: BinaryOrderLeft},
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
				Relation: member.RelationRef{Axis: valueAxis, Member: BinaryOrderSources},
				Key:      member.ProjectionRef{Axis: valueAxis, Member: BinaryOrderRight},
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
			Reducer: member.ReducerRef{Axis: valueAxis, Member: BinaryOrderReducer},
			Inputs:  []ruleprogram.JoinRef{0, 1},
			Outputs: []ruleprogram.OutputDecl{{
				Column:      axis.OutputRef{Axis: valueAxis, Key: OutputKey},
				Destination: member.ProjectionRef{Axis: valueAxis, Member: BinaryOrderWrite},
				Mode:        ruleprogram.ModeExact,
				ValueSlot:   0,
			}},
		},
		Carry: &ruleprogram.CarryDecl{Input: 0, Mode: ruleprogram.CarryIdentity},
	}
}

// Order is a short declaration alias for callers that name the rule by its
// family rather than its operand type.
func Order() ruleprogram.Program { return BinaryOrder() }
