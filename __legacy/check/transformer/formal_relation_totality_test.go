package transformer

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
)

func TestFormalRelationFreezeRejectsIncompletePathStoreShape(t *testing.T) {
	reg := standard.Registry()
	program := formalPathReplacementTestProgram(t, nil, typevalue.LiteralString(reg, "value"))
	code := program.bodies[0].relation.code
	step := code.nodes[1].steps[0]
	code.effects.nodes[step.effect].pathStoreObject.ListFloor = 1
	program.formalTemplate = nil

	_, err := freezeFormalRelationTemplate(program)
	if err == nil || !strings.Contains(err.Error(), "EffectPathStore shape assignment=true static=false heaps=0 entries=0 list-floor=1 has no complete formal factor transaction") {
		t.Fatalf("incomplete PathStore freeze error = %v", err)
	}
}

func TestFormalRelationFreezeRejectsUnknownBoundaryKind(t *testing.T) {
	reg := standard.Registry()
	program := formalPathReplacementTestProgram(t, nil, typevalue.LiteralString(reg, "value"))
	program.bodies[0].relation.code.nodes[1].steps[0].kind = boundaryStepKind(255)
	program.formalTemplate = nil

	_, err := freezeFormalRelationTemplate(program)
	if err == nil || !strings.Contains(err.Error(), "boundary kind 255 is outside the sealed Step vocabulary") {
		t.Fatalf("unknown boundary freeze error = %v", err)
	}
}
