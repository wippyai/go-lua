// Package program owns the runtime-kind rule's callback-free declaration.
//
// It names Value's sealed runtime-kind candidate, the two exact own-axis reads
// of the observed and compared values, the one exact foreign Call read the
// occurrence that candidate names addresses, and the exact Value publication at
// the coordinate Value already issued. It contains no engine slot, runtime
// callback, or compatibility path; the judgment itself stays in
// domain/value/runtimekind.
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

// The runtime-kind family identities. These are declaration keys, not runtime
// handles: composition resolves them against the sealed axis/member surfaces.
const (
	AxisKey     schema.Key = "value"
	OutputKey   schema.Key = "value/facts"
	RuleKey     schema.Key = "value-runtime-kind-call"
	RuleRole               = "rule/value/runtime-kind-call"
	OperandRole            = "operand/value/runtime-kind-call"

	callAxisKey schema.Key = "call"

	// Value owns the runtime-kind directory, the two values it reads, the
	// coordinate it publishes at, and the occurrence each row names. Call owns
	// the fact relation this rule reads and the key that addresses it; these
	// aliases keep the foreign side explicit.
	RuntimeKindCallCandidates  schema.Key = valuedomain.RuntimeKindCallCandidates
	RuntimeKindSubjects        schema.Key = valuedomain.RuntimeKindSubjects
	RuntimeKindComparisons     schema.Key = valuedomain.RuntimeKindComparisons
	RuntimeKindSubjectKey      schema.Key = valuedomain.RuntimeKindSubjectKey
	RuntimeKindComparisonKey   schema.Key = valuedomain.RuntimeKindComparisonKey
	RuntimeKindWriteCoordinate schema.Key = valuedomain.RuntimeKindWriteCoordinate
	RuntimeKindCallOccurrence  schema.Key = valuedomain.RuntimeKindCallOccurrence
	RuntimeKindCallSites       schema.Key = calldomain.RuntimeKindCallSites
	RuntimeKindCallSiteKey     schema.Key = calldomain.RuntimeKindCallSiteKey
	Reducer                    schema.Key = valuedomain.RuntimeKindCallReducer
)

func axisReference(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

// RuleIssues is this rule's mounted issuance geometry.
//
// The runtime-kind projection consumes the sealed geometry of a strict unary
// plain call and writes the call result: Value seals a candidate for exactly
// that geometry with a finite result slot, so the subscription requires it. A
// method, nullary, multi-argument, tail-expanded, or result-discarding call
// issues nothing here.
//
// The guarded arm of the same call is its own occurrence family, and Value
// seals a candidate for every row of it, so the rule subscribes to that family
// too. The arm is reached along its route predecessor, which is where the
// narrowed subject Value is carried.
func RuleIssues() []rule.Issuance {
	return []rule.Issuance{
		{
			Occurrence:  "occurrence/call",
			Requirement: "program-requirement/call-result",
			Form:        "program-form/call-summary",
		},
		{
			Occurrence:  "occurrence/operation-predicate-refinement",
			Requirement: "program-requirement/unrestricted",
			Form:        "program-form/local-predecessor",
		},
	}
}

// RuleEntry is the canonical callback-free runtime-kind rule declaration. The
// family is installed through the generated RuleFamily seam; this value is what
// Program composition consumes.
func RuleEntry() rule.Spec {
	return rule.Spec{
		Key:      RuleKey,
		Writes:   AxisKey,
		Owner:    AxisKey,
		Issues:   RuleIssues(),
		Lane:     rule.LaneMounted,
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole)},
		Program:  RuntimeKindCallResult(),
	}
}

// StructureSpecs contributes this rule's own semantic roles. The Value factor
// role is the axis owner's declaration and is therefore not re-authored here.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(RuleRole, OperandRole)
}

// RuntimeKindCallResult returns the immutable runtime-kind rule declaration.
//
// The candidate is Value's sealed runtime-kind row. Join 0 is the one exact
// foreign Call read: Call's mounted-call directory enumerates the same sites
// under its own order, addressed by the occurrence the candidate names, which
// is that call's own for the plain arm and the interpreted call's for the
// guarded one. Joins 1 and 2 are the own-axis reads of the observed and
// compared values, each addressed by the candidate's own coordinate. The
// publication is exact at the candidate's own write coordinate, so Value's
// relation owner projects it, and the identity carry keeps the predecessor
// world of that coordinate.
func RuntimeKindCallResult() ruleprogram.Program {
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
			Member: RuntimeKindCallCandidates,
		}),
		Joins: []ruleprogram.JoinDecl{
			{
				Sources:         []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation:        member.RelationRef{Axis: callAxis, Member: RuntimeKindCallSites},
				Key:             member.ProjectionRef{Axis: callAxis, Member: RuntimeKindCallSiteKey},
				AddressIdentity: member.ProjectionRef{Axis: valueAxis, Member: RuntimeKindCallOccurrence},
				Read: ruleprogram.ReadDecl{
					PointBound: ruleprogram.PointBound,
					Input:      0,
					Axis:       ruleprogram.AxisRef(callAxis),
					Form:       ruleprogram.Exact,
					Contract:   exact,
				},
			},
			{
				Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation: member.RelationRef{Axis: valueAxis, Member: RuntimeKindSubjects},
				Key:      member.ProjectionRef{Axis: valueAxis, Member: RuntimeKindSubjectKey},
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
				Relation: member.RelationRef{Axis: valueAxis, Member: RuntimeKindComparisons},
				Key:      member.ProjectionRef{Axis: valueAxis, Member: RuntimeKindComparisonKey},
				Read: ruleprogram.ReadDecl{
					PointBound: ruleprogram.PointBound,
					Input:      0,
					Axis:       ruleprogram.AxisRef(valueAxis),
					Form:       ruleprogram.Exact,
					Contract:   exact,
				},
			},
		},
		Fold: ruleprogram.FoldDecl{
			Reducer: member.ReducerRef{Axis: valueAxis, Member: Reducer},
			Inputs:  []ruleprogram.JoinRef{0, 1, 2},
			Outputs: []ruleprogram.OutputDecl{{
				Column: axis.OutputRef{
					Axis: valueAxis,
					Key:  OutputKey,
				},
				Destination: member.ProjectionRef{Axis: valueAxis, Member: RuntimeKindWriteCoordinate},
				Mode:        ruleprogram.ModeExact,
				ValueSlot:   0,
			}},
		},
		Carry: &ruleprogram.CarryDecl{Input: 0, Mode: ruleprogram.CarryIdentity},
	}
}
