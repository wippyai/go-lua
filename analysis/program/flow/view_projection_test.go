package flow_test

import (
	"testing"

	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func lowerBranchConditionLaw(t *testing.T, name, text string) *program.Program {
	t.Helper()
	result, err := lualower.Lower(lualower.Source{Name: name, Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// branchConditionBinary returns the exact Binary term used as the condition of
// the single authored Branch.
func branchConditionBinary(t *testing.T, lowered *program.Program) keyspace.Term {
	t.Helper()
	branches := lowered.Flow().Authored().Control().Branches()
	if branches.Count() != 1 {
		t.Fatalf("fixture published %d Branches", branches.Count())
	}
	branch, branchOK := branches.At(0)
	if !branchOK {
		t.Fatal("Branch view is not live")
	}
	_, condition, _, _, rowOK := branches.Get(branch)
	if !rowOK || keyspace.TermFamily(condition) != keyspace.FamilyBinary {
		t.Fatalf("Branch condition is not a Binary: family=%v ok=%t", keyspace.TermFamily(condition), rowOK)
	}
	return condition
}

// TestBinaryBranchConditionSealsComparisonOnlyForComparisons proves that a
// truth-tested arithmetic condition is an executable primitive carrying the
// exact Branch arms and no operand comparison, while an order condition still
// seals its comparison against the same Branch.
func TestBinaryBranchConditionSealsComparisonOnlyForComparisons(t *testing.T) {
	arithmetic := lowerBranchConditionLaw(t, "branch-condition-arithmetic.lua", `local cap = 3
if cap + 2 then
    return 1
end
return 0`)
	condition := branchConditionBinary(t, arithmetic)
	primitives := arithmetic.Flow().BinaryPrimitives()
	primitive, primitiveOK := primitives.Primitive(condition)
	if !primitiveOK {
		t.Fatal("truth-tested arithmetic Branch condition is not an executable primitive")
	}
	if source, sourceOK := primitive.Source(); !sourceOK || source != condition {
		t.Fatalf("primitive source disagrees with the Branch condition: %v/%t", source, sourceOK)
	}
	bucketed := false
	for index := 0; index < primitives.Arithmetic().Count(); index++ {
		term, termOK := primitives.Arithmetic().At(index)
		if termOK && term == condition {
			bucketed = true
			break
		}
	}
	if !bucketed {
		t.Fatal("truth-tested arithmetic Branch condition left the arithmetic bucket")
	}
	if _, comparisonOK := primitive.Comparison(); comparisonOK {
		t.Fatal("truth-tested arithmetic Branch condition sealed an operand comparison")
	}

	order := lowerBranchConditionLaw(t, "branch-condition-order.lua", `local cap = 3
if cap > 5 then
    return 1
end
return 0`)
	orderCondition := branchConditionBinary(t, order)
	orderPrimitive, orderPrimitiveOK := order.Flow().BinaryPrimitives().Primitive(orderCondition)
	if !orderPrimitiveOK {
		t.Fatal("order Branch condition is not an executable primitive")
	}
	comparison, comparisonOK := orderPrimitive.Comparison()
	if !comparisonOK {
		t.Fatal("order Branch condition lost its comparison")
	}
	branches := order.Flow().Authored().Control().Branches()
	branch, branchOK := branches.At(0)
	_, _, whenTrue, whenFalse, rowOK := branches.Get(branch)
	if !branchOK || !rowOK || comparison.Branch != branch || comparison.TrueBody != whenTrue || comparison.FalseBody != whenFalse {
		t.Fatalf("order comparison arms disagree with the authored Branch: %+v", comparison)
	}
}
