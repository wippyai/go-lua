package transfer

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// RuleEntry is Value transfer's semantic callback-free Rule declaration. The
// generic composition derives and binds its generated engine slot from this
// Program; this package owns no engine fragment, registration, or hot rule.
func RuleEntry() rule.Spec {
	valueAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "value"}
	return rule.Spec{
		Key:      "value-transfer",
		Writes:   "value",
		Owner:    "value",
		Issues:   valuedomain.StorageTransferRuleIssues(),
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/value/storage-transfer",
		Roles:    []schema.Key{"semantic/operand/value/storage-transfer"},
		Program: program.Program{
			OperandRole: "semantic/operand/value/storage-transfer",
			Candidate:   member.AxisRelationCandidate(member.RelationRef{Axis: valueAxis, Member: valuedomain.StorageTransferCandidates}),
			Joins: []program.JoinDecl{{
				Sources:  []program.SourceRef{program.CandidateSource()},
				Relation: member.RelationRef{Axis: valueAxis, Member: valuedomain.StorageTransferSources},
				Key:      member.ProjectionRef{Axis: valueAxis, Member: valuedomain.StorageTransferSourceKey},
				Read: program.ReadDecl{
					PointBound: program.PointBound,
					Input:      0,
					Axis:       program.AxisRef(valueAxis),
					Form:       program.Exact,
					Contract: program.ReadContract{
						Order: program.OrderCanonical, Sparse: program.SparseExplicit,
						OnOpaque: program.OnOpaqueRefuse, Multiplicity: program.MultiplicityOne,
					},
				},
			}},
			Fold: program.FoldDecl{
				Reducer: member.ReducerRef{Axis: valueAxis, Member: valuedomain.IdentityReducer},
				Inputs:  []program.JoinRef{0},
				Outputs: []program.OutputDecl{{
					Column:      axis.OutputRef{Axis: valueAxis, Key: "value/facts"},
					Destination: member.ProjectionRef{Axis: valueAxis, Member: valuedomain.StorageTransferTarget},
					Mode:        program.ModeExact,
					ValueSlot:   0,
				}},
			},
			Carry: &program.CarryDecl{Input: 0, Mode: program.CarryIdentity},
		},
	}
}

// StructureSpecs is this package's contribution to the analyzer's semantic
// role vocabulary: the two roles its rule is identified by. A role is
// declared where it is used, so the row and the reference that names it are one
// package's statement.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("rule/value/storage-transfer", "operand/value/storage-transfer")
}
