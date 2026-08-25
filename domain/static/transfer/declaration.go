package transfer

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// RuleEntry is Static transfer's semantic callback-free Rule declaration.
// The operand and destination stay Value's sealed StorageTransfer; this
// Program reads and writes only Static TypeFacts at those coordinates.
func RuleEntry() rule.Spec {
	valueAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "value"}
	staticAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "static-type"}
	return rule.Spec{
		Key:      "static-transfer",
		Writes:   "static-type",
		Owner:    "static-type",
		Issues:   valuedomain.StorageTransferRuleIssues(),
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/static/transfer",
		Roles:    []schema.Key{"semantic/operand/value/storage-transfer"},
		Program: program.Program{
			OperandRole: "semantic/operand/value/storage-transfer",
			Candidate:   member.AxisRelationCandidate(member.RelationRef{Axis: valueAxis, Member: valuedomain.StorageTransferCandidates}),
			Joins: []program.JoinDecl{{
				Sources:  []program.SourceRef{program.CandidateSource()},
				Relation: member.RelationRef{Axis: staticAxis, Member: staticdomain.TypeFactSources},
				Key:      member.ProjectionRef{Axis: staticAxis, Member: staticdomain.TypeFactSourceKey},
				Read: program.ReadDecl{
					PointBound: program.PointBound,
					Input:      0,
					Axis:       program.AxisRef(staticAxis),
					Form:       program.Exact,
					Contract: program.ReadContract{
						Order: program.OrderCanonical, Sparse: program.SparseExplicit,
						OnOpaque: program.OnOpaqueRefuse, Multiplicity: program.MultiplicityOne,
					},
				},
			}},
			Fold: program.FoldDecl{
				Reducer: member.ReducerRef{Axis: staticAxis, Member: staticdomain.IdentityTypeFactReducer},
				Inputs:  []program.JoinRef{0},
				Outputs: []program.OutputDecl{{
					Column:      axis.OutputRef{Axis: staticAxis, Key: "static-type/facts"},
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
// role vocabulary: the rule identity. The operand family is Value's existing
// StorageTransfer role.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("rule/static/transfer")
}
