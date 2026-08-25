package program

import (
	"testing"

	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

func TestAllocationBirthIsOneExactProducerReadAndOneExactWrite(t *testing.T) {
	program := AllocationBirth()
	if problem, ok := program.Check(); !ok {
		t.Fatalf("allocation birth declaration: %+v", problem)
	}
	if len(program.Joins) != 1 || len(program.Fold.Inputs) != 1 || len(program.Fold.Outputs) != 1 || program.Carry == nil || program.Carry.Input != 0 || program.Carry.Mode != ruleprogram.CarryIdentity {
		t.Fatalf("allocation birth shape joins=%d inputs=%d outputs=%d carry=%v", len(program.Joins), len(program.Fold.Inputs), len(program.Fold.Outputs), program.Carry)
	}
}

// TestAllocationBirthConsumesTheAllocationLocalSuccessor keeps the birth
// consumer behind Value allocation's local-stage publication cut. It must not
// share the entry snapshot that the allocation producer reads.
func TestAllocationBirthConsumesTheAllocationLocalSuccessor(t *testing.T) {
	issues := RuleIssues()
	if len(issues) != 1 {
		t.Fatalf("allocation birth issues = %d, want 1", len(issues))
	}
	issue := issues[0]
	if issue.Occurrence != "occurrence/allocation" || issue.Requirement != "program-requirement/unrestricted" || issue.Form != programissuance.FormLocalSuccessor {
		t.Fatalf("allocation birth issue = %+v, want allocation local-successor", issue)
	}
}
