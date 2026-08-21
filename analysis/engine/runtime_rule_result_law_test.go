package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func resultReplayLawExecution[V, O any](bound *boundRuleMember[V, O], epoch identity.Generation, row int) *ruleExecution {
	execution := &ruleExecution{owner: bound, epoch: epoch}
	execution.active.Open(epoch)
	execution.product = &productSession{
		execution: execution,
		ready:     true,
		current:   row,
		values:    make([]productRow, row+1),
		live:      true,
	}
	return execution
}

// TestRuleResultCannotReplayAcrossExecutionBoundaries proves an opaque result
// is a capability for exactly the Product row that issued it. Retaining the
// value cannot turn it into a second publication authority for another row,
// member, execution, or epoch.
func TestRuleResultCannotReplayAcrossExecutionBoundaries(t *testing.T) {
	staged := 0
	output := outputAccess[uint64]{
		stage: func(*ruleExecution, identity.Generation, int, uint64) bool {
			staged++
			return true
		},
		noCandidate: func(*ruleExecution, identity.Generation, int) bool { return true },
	}
	first := &boundRuleMember[uint64, struct{}]{output: output}
	second := &boundRuleMember[uint64, struct{}]{output: output}
	epoch := identity.Generation(1)
	firstExecution := resultReplayLawExecution(first, epoch, 0)
	result := RuleResult[uint64]{execution: firstExecution, epoch: epoch, row: 0, kind: ruleResultStaged, value: 7}
	if !first.settleRuleResult(firstExecution, epoch, 0, result) || staged != 1 {
		t.Fatal("issuing row could not settle its own result")
	}

	firstExecution.product.current = 1
	firstExecution.product.values = append(firstExecution.product.values, productRow{})
	if first.settleRuleResult(firstExecution, epoch, 1, result) {
		t.Fatal("result replayed into another Product row")
	}

	otherExecution := resultReplayLawExecution(first, epoch, 0)
	if first.settleRuleResult(otherExecution, epoch, 0, result) {
		t.Fatal("result replayed into another execution")
	}

	otherMemberExecution := resultReplayLawExecution(second, epoch, 0)
	if second.settleRuleResult(otherMemberExecution, epoch, 0, result) {
		t.Fatal("result replayed into another bound member")
	}

	firstExecution.product.current = 0
	firstExecution.active.Open(epoch.Next())
	if first.settleRuleResult(firstExecution, epoch.Next(), 0, result) {
		t.Fatal("result replayed into a later execution epoch")
	}
	if staged != 1 {
		t.Fatalf("replayed results staged %d writes", staged-1)
	}
}
