package source

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	packdomain "github.com/wippyai/go-lua/domain/pack"
)

// RuleEntry is Pack source's semantic callback-free Rule declaration. The
// composition derives and binds its generated engine slot from this Program;
// this package owns no engine fragment, registration, or hot rule.
func RuleEntry() rule.Spec {
	packAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "pack"}
	return rule.Spec{
		Key:    "pack-source",
		Writes: "pack",
		Owner:  "pack",
		Issues: []rule.Issuance{
			{Occurrence: "occurrence/values", Requirement: "program-requirement/unrestricted", Form: "program-form/base-none-allow-empty"},
			{Occurrence: "occurrence/call", Requirement: "program-requirement/unrestricted", Form: "program-form/base-none"},
		},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/pack/source",
		Roles:    []schema.Key{"semantic/operand/pack/source"},
		Program: program.Program{
			OperandRole: "semantic/operand/pack/source",
			Candidate:   member.AxisRelationCandidate(member.RelationRef{Axis: packAxis, Member: packdomain.SourceSeeds}),
			Fold: program.FoldDecl{
				Reducer: member.ReducerRef{Axis: packAxis, Member: packdomain.SourceReducer},
				Outputs: []program.OutputDecl{{
					Column:      axis.OutputRef{Axis: packAxis, Key: "pack/facts"},
					Destination: member.ProjectionRef{Axis: packAxis, Member: packdomain.SourceCoordinate},
					Mode:        program.ModeExact,
					ValueSlot:   0,
				}},
			},
		},
	}
}

// StructureSpecs is this package's contribution to the analyzer's semantic
// role vocabulary: the two roles its rule is identified by.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("rule/pack/source", "operand/pack/source")
}
