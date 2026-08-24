package bootstrap

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
)

// RuleEntry is Heap bootstrap's semantic callback-free Rule declaration. Its
// candidate relation is the Link-global directory of sealed bootstrap roots,
// so the occurrences this rule admits are that directory rather than an
// artifact's rows; the composition derives and binds the generated engine slot
// from this Program, and this package owns no engine fragment, registration,
// or hot rule.
func RuleEntry() rule.Spec {
	heapAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "heap"}
	return rule.Spec{
		Key:      "heap-bootstrap",
		Writes:   "heap",
		Owner:    "heap",
		Lane:     rule.LaneLink,
		Semantic: "semantic/rule/heap/host-bootstrap",
		Roles:    []schema.Key{"semantic/operand/heap/host-bootstrap"},
		Program: program.Program{
			OperandRole: "semantic/operand/heap/host-bootstrap",
			Candidate:   member.AxisRelationCandidate(member.RelationRef{Axis: heapAxis, Member: heapdomain.BootRoots}),
			Fold: program.FoldDecl{
				Reducer: member.ReducerRef{Axis: heapAxis, Member: heapdomain.BootReducer},
				Outputs: []program.OutputDecl{{
					Column:      axis.OutputRef{Axis: heapAxis, Key: "heap/facts"},
					Destination: member.ProjectionRef{Axis: heapAxis, Member: heapdomain.BootCoordinate},
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
	return vocabulary.RoleSpecs("rule/heap/host-bootstrap", "operand/heap/host-bootstrap")
}
