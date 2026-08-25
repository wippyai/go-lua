package program

import (
	"testing"

	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

func TestFreshBirthIsOneExactProducerReadAndOneExactWrite(t *testing.T) {
	program := FreshBirth()
	if problem, ok := program.Check(); !ok {
		t.Fatalf("fresh birth declaration: %+v", problem)
	}
	if len(program.Joins) != 1 || len(program.Fold.Inputs) != 1 || len(program.Fold.Outputs) != 1 {
		t.Fatalf("fresh birth shape joins=%d inputs=%d outputs=%d", len(program.Joins), len(program.Fold.Inputs), len(program.Fold.Outputs))
	}
	if program.Carry == nil || program.Carry.Input != 0 || program.Carry.Mode != ruleprogram.CarryIdentity || program.Joins[0].Read.Form != ruleprogram.Exact {
		t.Fatal("fresh birth lost its input identity carry or exact read")
	}
}
