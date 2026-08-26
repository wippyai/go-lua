package source

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

// RuleEntry is Value source's semantic callback-free Rule declaration. The
// composition derives and binds its generated engine slot from this Program;
// this package owns no engine fragment, registration, or hot rule.
func RuleEntry() rule.Spec {
	valueAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "value"}
	return rule.Spec{
		Key:    "value-source",
		Writes: "value",
		Owner:  "value",
		Issues: []rule.Issuance{
			{Occurrence: "occurrence/value-source", Requirement: "program-requirement/unrestricted", Form: "program-form/base-none"},
			{Occurrence: "occurrence/formal-entry", Requirement: "program-requirement/unrestricted", Form: "program-form/base-none"},
			{Occurrence: "occurrence/global-entry", Requirement: "program-requirement/unrestricted", Form: "program-form/base-none"},
		},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/value/source",
		Roles:    []schema.Key{"semantic/operand/value/source"},
		Program: program.Program{
			OperandRole: "semantic/operand/value/source",
			Candidate:   member.AxisRelationCandidate(member.RelationRef{Axis: valueAxis, Member: valuedomain.SourceSeeds}),
			Fold: program.FoldDecl{
				Reducer: member.ReducerRef{Axis: valueAxis, Member: valuedomain.SourceReducer},
				Outputs: []program.OutputDecl{{
					Column:      axis.OutputRef{Axis: valueAxis, Key: "value/facts"},
					Destination: member.ProjectionRef{Axis: valueAxis, Member: valuedomain.SourceCoordinate},
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
	return vocabulary.RoleSpecs("rule/value/source", "operand/value/source")
}
