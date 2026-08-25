// Package program owns the module-load call result's callback-free rule
// declaration.
//
// It names Value's sealed module-load candidate, the exact own-axis read of
// the actual that call applies, the one exact foreign Call read that
// candidate's occurrence addresses, and the exact Value publication at the
// call-result coordinate Value already issued. It contains no engine slot,
// runtime callback, or compatibility path; the judgment itself stays in
// domain/value/moduleload.
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
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// The module-load family identities. These are declaration keys, not runtime
// handles: composition resolves them against the sealed axis/member surfaces.
const (
	AxisKey     schema.Key = "value"
	OutputKey   schema.Key = "value/facts"
	RuleKey     schema.Key = "value-callresult-moduleload"
	RuleRole               = "rule/value/callresult-moduleload"
	OperandRole            = "operand/value/callresult-moduleload"

	callAxisKey schema.Key = "call"

	// Value owns the module-load directory, the actual it reads, and the
	// call-result coordinate it publishes at. Call owns the fact relation this
	// rule reads and the key that addresses it; these aliases keep the foreign
	// side explicit.
	ModuleLoadCallCandidates   schema.Key = valuedomain.ModuleLoadCallCandidates
	ModuleLoadArguments        schema.Key = valuedomain.ModuleLoadArguments
	ModuleLoadArgumentKey      schema.Key = valuedomain.ModuleLoadArgumentKey
	ModuleLoadResultCoordinate schema.Key = valuedomain.ModuleLoadResultCoordinate
	ModuleLoadCallSites        schema.Key = calldomain.ModuleLoadCallSites
	ModuleLoadCallSiteKey      schema.Key = calldomain.ModuleLoadCallSiteKey
	Reducer                    schema.Key = valuedomain.ModuleLoadCallReducer
)

func axisReference(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

// RuleIssues is this rule's mounted issuance geometry: one call occurrence
// whose result the module-load requirement names.
func RuleIssues() []rule.Issuance {
	return []rule.Issuance{{
		Occurrence:  "occurrence/call",
		Requirement: "program-requirement/module-load-call-result",
		Form:        "program-form/call-summary",
	}}
}

// RuleEntry is the canonical callback-free module-load rule declaration. The
// family is installed through the generated RuleFamily seam; this value is
// what Program composition consumes.
func RuleEntry() rule.Spec {
	return rule.Spec{
		Key:      RuleKey,
		Writes:   AxisKey,
		Owner:    AxisKey,
		Issues:   RuleIssues(),
		Lane:     rule.LaneMounted,
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole)},
		Program:  ModuleLoadCallResult(),
	}
}

// StructureSpecs contributes this rule's own semantic roles. The Value factor
// role is the axis owner's declaration and is therefore not re-authored here.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(RuleRole, OperandRole)
}

// ModuleLoadCallResult returns the immutable module-load rule declaration.
//
// The candidate is Value's sealed module-load row. Join 0 is the own-axis read
// of the actual the scoped loader is applied to, addressed by the candidate's
// own argument coordinate. Join 1 is the one exact foreign Call read: Call's
// mounted-call directory enumerates the same sites under its own order, and
// the occurrence both directories are addressed by resolves the row. The
// publication is exact at the candidate's own call-result coordinate, so
// Value's relation owner projects it, and the identity carry keeps the
// predecessor world of that coordinate.
func ModuleLoadCallResult() ruleprogram.Program {
	valueAxis := axisReference(AxisKey)
	callAxis := axisReference(callAxisKey)
	exact := ruleprogram.ReadContract{
		Order:        ruleprogram.OrderCanonical,
		Sparse:       ruleprogram.SparseExplicit,
		OnOpaque:     ruleprogram.OnOpaqueRefuse,
		Multiplicity: ruleprogram.MultiplicityOne,
	}
	return ruleprogram.Program{
		OperandRole: vocabulary.RoleKey(OperandRole),
		Candidate: member.AxisRelationCandidate(member.RelationRef{
			Axis:   valueAxis,
			Member: ModuleLoadCallCandidates,
		}),
		Joins: []ruleprogram.JoinDecl{
			{
				Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation: member.RelationRef{Axis: valueAxis, Member: ModuleLoadArguments},
				Key:      member.ProjectionRef{Axis: valueAxis, Member: ModuleLoadArgumentKey},
				Read: ruleprogram.ReadDecl{
					PointBound: ruleprogram.PointBound,
					Input:      0,
					Axis:       ruleprogram.AxisRef(valueAxis),
					Form:       ruleprogram.Exact,
					Contract:   exact,
				},
			},
			{
				Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation: member.RelationRef{Axis: callAxis, Member: ModuleLoadCallSites},
				Key:      member.ProjectionRef{Axis: callAxis, Member: ModuleLoadCallSiteKey},
				Read: ruleprogram.ReadDecl{
					PointBound: ruleprogram.PointBound,
					Input:      0,
					Axis:       ruleprogram.AxisRef(callAxis),
					Form:       ruleprogram.Exact,
					Contract:   exact,
				},
			},
		},
		Fold: ruleprogram.FoldDecl{
			Reducer: member.ReducerRef{Axis: valueAxis, Member: Reducer},
			Inputs:  []ruleprogram.JoinRef{0, 1},
			Outputs: []ruleprogram.OutputDecl{{
				Column: axis.OutputRef{
					Axis: valueAxis,
					Key:  OutputKey,
				},
				Destination: member.ProjectionRef{Axis: valueAxis, Member: ModuleLoadResultCoordinate},
				Mode:        ruleprogram.ModeExact,
				ValueSlot:   0,
			}},
		},
		Carry: &ruleprogram.CarryDecl{Input: 0, Mode: ruleprogram.CarryIdentity},
	}
}
