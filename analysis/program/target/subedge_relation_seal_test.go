package target

import (
	"testing"

	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

func TestSealSubedgeRelationRejectsForeignCoordinates(t *testing.T) {
	valid := func() OperationSpec {
		operation := protectedSubedgeOperation("neutral-subedge-relation-seal", false, false, false)
		for index := range operation.Outcomes {
			operation.Outcomes[index].Values.Fixed = []schematype.Type{testAny}
		}
		operation.SubedgeRelation = &SubedgeRelationSpec{
			Operand: 1, Selector: 1, Subedge: 1, ResultOutcome: 0, Result: 0,
		}
		return operation
	}
	tests := []struct {
		name   string
		mutate func(*SubedgeRelationSpec)
	}{
		{"operand", func(row *SubedgeRelationSpec) { row.Operand = 2 }},
		{"subedge", func(row *SubedgeRelationSpec) { row.Subedge = 3 }},
		{"outcome", func(row *SubedgeRelationSpec) { row.ResultOutcome = 6 }},
		{"result", func(row *SubedgeRelationSpec) { row.Result = 1 }},
		{"effect", func(row *SubedgeRelationSpec) { row.EffectAliases = []uint32{0} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			operation := valid()
			test.mutate(operation.SubedgeRelation)
			if _, err := testSeal(&Spec{Operations: []OperationSpec{operation}}); err == nil {
				t.Fatal("foreign coordinate admitted")
			}
		})
	}
}
