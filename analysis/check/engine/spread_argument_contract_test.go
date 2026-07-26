package engine

import (
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/ir/wir"
)

// stringParameterSignature is the contract a caller's argument list is decided
// against: two required parameters, both string.
func stringParameterSignature() callableShape {
	return callableShape{Params: []string{"string", "string"}, Required: 2}
}

// spreadCallOperands is the operand shape a call with a trailing expansion
// carries: the positions the list names, and the spread marker that states the
// expansion has no proven count.
func spreadCallOperands(arguments ...string) directCallOperands {
	operands := directCallOperands{callee: []byte("path/need"), display: "need", spread: true}
	for _, argument := range arguments {
		operands.arguments = append(operands.arguments, []byte(argument))
	}
	return operands
}

func argumentDiagnostic(t *testing.T, result equation.TransactionResult) string {
	t.Helper()
	if len(result.Closure.Diagnostics) != 1 {
		t.Fatalf("expected exactly one diagnostic, got %d", len(result.Closure.Diagnostics))
	}
	message, _ := diagnosticMessage(result.Closure.Diagnostics[0].Value)
	return message
}

// TestSpreadDecidesThePositionsItsListNames pins the sound half of the spread
// rule: the leading result of the expansion lands at the position its own
// argument occupies, so a value refuting that parameter refutes the call.
func TestSpreadDecidesThePositionsItsListNames(t *testing.T) {
	partition := callResultPartition(t,
		equation.Fact{Key: "value/temp/1/op-00000001", Value: []byte("scalar/number/1")},
	)
	result, refuted := spreadArgumentRefutation(equation.BoundEquation{}, spreadCallOperands("temp/1"), stringParameterSignature(), partition)
	if !refuted {
		t.Fatal("a number landing at a string parameter was accepted through a spread")
	}
	if message := argumentDiagnostic(t, result); message != "argument 1 is 1, not string" {
		t.Fatalf("diagnostic = %q", message)
	}
}

// TestSpreadKeepsThePositionsAheadOfItExact keeps the ordinary arguments of a
// spread call under their own contracts: only the final position expands.
func TestSpreadKeepsThePositionsAheadOfItExact(t *testing.T) {
	partition := callResultPartition(t,
		equation.Fact{Key: "value/temp/1/op-00000001", Value: []byte(`scalar/string/"head"`)},
		equation.Fact{Key: "value/temp/2/op-00000001", Value: []byte("scalar/number/1")},
	)
	result, refuted := spreadArgumentRefutation(equation.BoundEquation{}, spreadCallOperands("temp/1", "temp/2"), stringParameterSignature(), partition)
	if !refuted {
		t.Fatal("the position ahead of the expansion was not decided")
	}
	if message := argumentDiagnostic(t, result); message != "argument 2 is 1, not string" {
		t.Fatalf("diagnostic = %q", message)
	}
}

// TestSpreadClaimsNoArity is the fail-closed half. An expansion may supply any
// number of values, including none, so an argument list longer than the
// parameter list proves no count and the positions past the contract end the
// walk undecided.
func TestSpreadClaimsNoArity(t *testing.T) {
	partition := callResultPartition(t,
		equation.Fact{Key: "value/temp/1/op-00000001", Value: []byte(`scalar/string/"a"`)},
		equation.Fact{Key: "value/temp/2/op-00000001", Value: []byte(`scalar/string/"b"`)},
		equation.Fact{Key: "value/temp/3/op-00000001", Value: []byte("scalar/number/1")},
	)
	if _, refuted := spreadArgumentRefutation(equation.BoundEquation{}, spreadCallOperands("temp/1", "temp/2", "temp/3"), stringParameterSignature(), partition); refuted {
		t.Fatal("an argument past the parameter list was decided through a spread")
	}
	short := spreadCallOperands("temp/1")
	if _, refuted := spreadArgumentRefutation(equation.BoundEquation{}, short, stringParameterSignature(), partition); refuted {
		t.Fatal("a list shorter than the required count was refuted through a spread")
	}
}

// TestPositionalContractEndsWhereTheParameterListDoes pins the shared decision
// the exact and spread walks both consume: a position the signature does not
// state yields no refutation and stops the walk.
func TestPositionalContractEndsWhereTheParameterListDoes(t *testing.T) {
	partition := callResultPartition(t,
		equation.Fact{Key: "value/temp/9/op-00000001", Value: []byte("scalar/number/1")},
	)
	_, refuted, ended := positionalArgumentContract(equation.BoundEquation{}, stringParameterSignature(), true, 0, 2, []byte("temp/9"), partition)
	if refuted || !ended {
		t.Fatalf("position past the contract = (refuted %v, ended %v), want (false, true)", refuted, ended)
	}
	_, refuted, ended = positionalArgumentContract(equation.BoundEquation{}, stringParameterSignature(), true, 0, 0, []byte("temp/missing"), partition)
	if refuted || ended {
		t.Fatalf("position with no published value = (refuted %v, ended %v), want (false, false)", refuted, ended)
	}
}

// concatExpression is the operation shape the expression kernel reads for a
// two-operand concatenation.
func concatExpression(result string, terms ...string) equation.BoundEquation {
	operands := []equation.BoundOperand{
		{Role: "result", Value: []byte(result)},
		{Role: "kind", Value: []byte(strconv.Itoa(int(wir.OpConcat)))},
		{Role: "operator", Value: []byte("0")},
	}
	for index, term := range terms {
		operands = append(operands, equation.BoundOperand{Role: "value-" + strconv.Itoa(index + 100000000)[1:], Value: []byte(term)})
	}
	return equation.BoundEquation{Target: equation.Coordinate{Name: "op-00000002"}, Operands: operands}
}

func publishedValue(t *testing.T, result equation.TransactionResult, term string) string {
	t.Helper()
	for _, fact := range result.Closure.Values {
		if fact.Key == "value/"+term+"/op-00000002" {
			return string(fact.Value)
		}
	}
	t.Fatalf("no value published for %s", term)
	return ""
}

// TestConcatCarriesAnOperandBoundaryIntoItsResult pins the operator's own
// contribution: it validates nothing an operand owed, so a result built from an
// unvalidated operand carries that boundary rather than stating top. The
// operand here is a call result, which no path-rooted relay reaches.
func TestConcatCarriesAnOperandBoundaryIntoItsResult(t *testing.T) {
	partition := callResultPartition(t,
		equation.Fact{Key: `value/temp/1/op-00000001`, Value: []byte(`scalar/string/"e: "`)},
		equation.Fact{Key: "value/temp/2/op-00000001", Value: []byte(`scalar/claim/claim-kind/3/"any"`)},
	)
	result, err := expressionKernel(concatExpression("temp/3", "temp/1", "temp/2"), partition)
	if err != nil {
		t.Fatalf("concat kernel: %v", err)
	}
	if value := publishedValue(t, result, "temp/3"); !isUnvalidatedAnyValue([]byte(value)) {
		t.Fatalf("concat result = %q, want the operand's any boundary", value)
	}
}

// TestConcatOfProvenOperandsStaysConcrete keeps the change confined to the
// boundary: operands the kernel can evaluate still produce their exact string.
func TestConcatOfProvenOperandsStaysConcrete(t *testing.T) {
	partition := callResultPartition(t,
		equation.Fact{Key: `value/temp/1/op-00000001`, Value: []byte(`scalar/string/"e: "`)},
		equation.Fact{Key: "value/temp/2/op-00000001", Value: []byte("scalar/number/1")},
	)
	result, err := expressionKernel(concatExpression("temp/3", "temp/1", "temp/2"), partition)
	if err != nil {
		t.Fatalf("concat kernel: %v", err)
	}
	if value := publishedValue(t, result, "temp/3"); value != `scalar/string/"e: 1"` {
		t.Fatalf("concat result = %q, want the evaluated string", value)
	}
}

// TestConcatOfAnUnresolvedOperandStaysTop is the control: an operand the kernel
// cannot evaluate has published no boundary, and absence of information is not
// a claim.
func TestConcatOfAnUnresolvedOperandStaysTop(t *testing.T) {
	partition := callResultPartition(t,
		equation.Fact{Key: `value/temp/1/op-00000001`, Value: []byte(`scalar/string/"e: "`)},
		equation.Fact{Key: "value/temp/2/op-00000001", Value: []byte("scalar/top")},
	)
	result, err := expressionKernel(concatExpression("temp/3", "temp/1", "temp/2"), partition)
	if err != nil {
		t.Fatalf("concat kernel: %v", err)
	}
	if value := publishedValue(t, result, "temp/3"); value != "scalar/top" {
		t.Fatalf("concat result = %q, want scalar/top", value)
	}
}
