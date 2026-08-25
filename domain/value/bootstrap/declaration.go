package bootstrap

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

// RuleEntry is Value bootstrap's semantic callback-free Rule declaration. Its
// candidate relation is the Link-global directory of sealed Host global
// binding receipts, so the occurrences this rule admits are that directory
// rather than an artifact's rows; the composition derives and binds the
// generated engine slot from this Program, and this package owns no engine
// fragment, registration, or hot rule.
func RuleEntry() rule.Spec {
	valueAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "value"}
	return rule.Spec{
		Key:      "value-bootstrap",
		Writes:   "value",
		Owner:    "value",
		Lane:     rule.LaneLink,
		Semantic: "semantic/rule/value/host-global-bootstrap",
		Roles:    []schema.Key{"semantic/operand/value/host-global-bootstrap"},
		Program: program.Program{
			OperandRole: "semantic/operand/value/host-global-bootstrap",
			Candidate:   member.AxisRelationCandidate(member.RelationRef{Axis: valueAxis, Member: valuedomain.GlobalBootstrapResults}),
			Fold: program.FoldDecl{
				Reducer: member.ReducerRef{Axis: valueAxis, Member: valuedomain.GlobalBootstrapReducer},
				Outputs: []program.OutputDecl{{
					Column:      axis.OutputRef{Axis: valueAxis, Key: "value/facts"},
					Destination: member.ProjectionRef{Axis: valueAxis, Member: valuedomain.GlobalBootstrapCoordinate},
					Mode:        program.ModeExact,
					ValueSlot:   0,
				}},
			},
		},
	}
}

// StructureSpecs is this package's contribution to the analyzer's semantic
// role vocabulary: the two roles its rule is identified by. A role is
// declared where it is used, so the row and the reference that names it are one
// package's statement.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("rule/value/host-global-bootstrap", "operand/value/host-global-bootstrap")
}
