// Package program owns Value presence-refinement's callback-free Rule
// Program declaration. The candidate and coordinate projections are
// Value-owned members; this package states only the one exact read, the
// fold, output, and identity carry that compose them.
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

	RuleKey     schema.Key = "value-presence-refinement"
	RuleRole               = "rule/value/presence-refinement"
	OperandRole            = "operand/value/presence-refinement"

	PresenceRefinementCandidates schema.Key = "value/presence-refinement/candidates"
	PresenceRefinementSources    schema.Key = "value/presence-refinement/sources"
	PresenceRefinementSourceKey  schema.Key = "value/presence-refinement/source-key"
	PresenceRefinementWrite      schema.Key = "value/presence-refinement/write"
	PresenceRefinementReducer    schema.Key = "value/presence-refinement/reducer"
)

// RuleIssues is Value presence-refinement's mounted occurrence issuance. The
// rule package owns no engine registration; composition consumes this
// declaration and its issuance rows directly.
func RuleIssues() []rule.Issuance {
	return []rule.Issuance{{
		Occurrence:  "occurrence/binary-presence-refinement",
		Requirement: "program-requirement/unrestricted",
		Form:        "program-form/local-predecessor",
	}}
}

// RuleEntry is the child-package rule declaration consumed by composition.
// Its Program is the sole source of refinement join/fold geometry.
func RuleEntry() rule.Spec {
	return rule.Spec{
		Key:      RuleKey,
		Writes:   AxisKey,
		Owner:    AxisKey,
		Issues:   RuleIssues(),
		Lane:     rule.LaneMounted,
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole)},
		Program:  PresenceRefinement(),
	}
}

// StructureSpecs contributes refinement's rule and operand semantic roles to
// the structural vocabulary registry.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(RuleRole, OperandRole)
}

func axisReference(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

// PresenceRefinement returns Value's complete refinement Program:
//
//	candidate
//	    -> exact Value at the candidate's guarded-read coordinate
//	    -> one Value write at the candidate's narrowed-write coordinate
//
// Every other coordinate the arm-entry environment already carries passes
// through unaffected via the declared identity carry; only the guarded
// coordinate is folded.
func PresenceRefinement() ruleprogram.Program {
	valueAxis := axisReference(AxisKey)
	provider := member.RelationRef{Axis: valueAxis, Member: PresenceRefinementCandidates}
	return ruleprogram.Program{
		OperandRole: vocabulary.RoleKey(OperandRole),
		Candidate:   member.AxisRelationCandidate(provider),
		Joins: []ruleprogram.JoinDecl{{
			Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
			Relation: member.RelationRef{Axis: valueAxis, Member: PresenceRefinementSources},
			Key:      member.ProjectionRef{Axis: valueAxis, Member: PresenceRefinementSourceKey},
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
		}},
		Fold: ruleprogram.FoldDecl{
			Reducer: member.ReducerRef{Axis: valueAxis, Member: PresenceRefinementReducer},
			Inputs:  []ruleprogram.JoinRef{0},
			Outputs: []ruleprogram.OutputDecl{{
				Column:      axis.OutputRef{Axis: valueAxis, Key: OutputKey},
				Destination: member.ProjectionRef{Axis: valueAxis, Member: PresenceRefinementWrite},
				Mode:        ruleprogram.ModeExact,
				ValueSlot:   0,
			}},
		},
		Carry: &ruleprogram.CarryDecl{Input: 0, Mode: ruleprogram.CarryIdentity},
	}
}
