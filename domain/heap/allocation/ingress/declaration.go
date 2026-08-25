package ingress

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

// RuleEntry is Heap allocation-ingress's semantic callback-free Rule
// declaration. The composition derives and binds its generated engine slot
// from this Program; this package owns no engine fragment, registration, or
// hot rule.
func RuleEntry() rule.Spec {
	heapAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "heap"}
	return rule.Spec{
		Key:    "heap-ingress",
		Writes: "heap",
		Owner:  "heap",
		Issues: []rule.Issuance{
			{Occurrence: "occurrence/allocation", Requirement: "program-requirement/unrestricted", Form: "program-form/base-none"},
		},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/heap/allocation-ingress",
		Roles:    []schema.Key{"semantic/operand/heap/allocation-ingress"},
		Program: program.Program{
			OperandRole: "semantic/operand/heap/allocation-ingress",
			Candidate:   member.AxisRelationCandidate(member.RelationRef{Axis: heapAxis, Member: heapdomain.IngressSeeds}),
			Fold: program.FoldDecl{
				Reducer: member.ReducerRef{Axis: heapAxis, Member: heapdomain.IngressReducer},
				Outputs: []program.OutputDecl{{
					Column:      axis.OutputRef{Axis: heapAxis, Key: "heap/facts"},
					Destination: member.ProjectionRef{Axis: heapAxis, Member: heapdomain.IngressCoordinate},
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
	return vocabulary.RoleSpecs("rule/heap/allocation-ingress", "operand/heap/allocation-ingress")
}
