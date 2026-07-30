package engine_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
)

// hasPublished reports whether a published diagnostic of the given code carries
// a message containing want.
func hasPublished(result engine.Result, code, want string) bool {
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Code == code && strings.Contains(diagnostic.Message, want) {
			return true
		}
	}
	return false
}

func checkArith(t *testing.T, source string) engine.Result {
	t.Helper()
	result, err := engine.Check(source)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	return result
}

// TestDeclaredFormalArithmeticAssignmentContract exercises the declared-formal
// arithmetic boundary in assignment position for an uncalled exported function.
// The result type is projected from the declared parameter types, so a mismatch
// against the annotated target is proven at the declaration boundary.
func TestDeclaredFormalArithmeticAssignmentContract(t *testing.T) {
	// number + number -> number; assigning to integer is a proven mismatch.
	add := checkArith(t, `local function f(a: number, b: number): integer
    local x: integer = a + b
    return x
end
return f`)
	if !hasPublished(add, "type.assignment", "number, not integer") {
		t.Fatalf("number+number->number assigned to integer: want mismatch, got %#v", add.PublishedDiagnostics)
	}

	// integer + integer -> integer; assigning to integer is exact, no diagnostic.
	addInt := checkArith(t, `local function f(a: integer, b: integer): integer
    local x: integer = a + b
    return x
end
return f`)
	if hasPublished(addInt, "type.assignment", "not integer") {
		t.Fatalf("integer+integer->integer assigned to integer must not error: %#v", addInt.PublishedDiagnostics)
	}

	// integer * integer -> integer; assigning to number widens, no diagnostic.
	mulInt := checkArith(t, `local function f(a: integer, b: integer): number
    local x: number = a * b
    return x
end
return f`)
	if hasPublished(mulInt, "type.assignment", "not number") {
		t.Fatalf("integer*integer->integer assigned to number must not error: %#v", mulInt.PublishedDiagnostics)
	}

	// integer / integer -> number (division is always float); integer target errors.
	div := checkArith(t, `local function f(a: integer, b: integer): integer
    local x: integer = a / b
    return x
end
return f`)
	if !hasPublished(div, "type.assignment", "number, not integer") {
		t.Fatalf("integer/integer->number assigned to integer: want mismatch, got %#v", div.PublishedDiagnostics)
	}

	// integer // integer -> integer (floor division of integers stays integer).
	floorInt := checkArith(t, `local function f(a: integer, b: integer): integer
    local x: integer = a // b
    return x
end
return f`)
	if hasPublished(floorInt, "type.assignment", "not integer") {
		t.Fatalf("integer//integer->integer assigned to integer must not error: %#v", floorInt.PublishedDiagnostics)
	}

	// number // number -> number (floor division of floats stays float); integer
	// target errors. This is the memory-model floor_div soundness guard.
	floorNum := checkArith(t, `local function f(a: number, b: number): integer
    local x: integer = a // b
    return x
end
return f`)
	if !hasPublished(floorNum, "type.assignment", "number, not integer") {
		t.Fatalf("number//number->number assigned to integer: want mismatch, got %#v", floorNum.PublishedDiagnostics)
	}
}

// TestDeclaredFormalArithmeticReturnContract exercises the declared-formal
// arithmetic boundary in return position. This is the memory-model
// return_operator soundness guard.
func TestDeclaredFormalArithmeticReturnContract(t *testing.T) {
	// number // number -> number returned where integer is declared: mismatch.
	ret := checkArith(t, `local function f(a: number, b: number): integer
    return a // b
end
return f`)
	if !hasPublished(ret, "type.return.contract", "number") {
		t.Fatalf("return number//number where integer declared: want return-contract mismatch, got %#v", ret.PublishedDiagnostics)
	}

	// integer + integer -> integer returned where integer is declared: exact.
	okRet := checkArith(t, `local function f(a: integer, b: integer): integer
    return a + b
end
return f`)
	if hasPublished(okRet, "type.return.contract", "") {
		t.Fatalf("return integer+integer where integer declared must not error: %#v", okRet.PublishedDiagnostics)
	}
}

// TestDeclaredFormalArithmeticFailsOpenOnAnyOperand keeps the boundary sound:
// an any-typed operand has no projected numeric result, so the arithmetic
// assignment must not manufacture a spurious mismatch.
func TestDeclaredFormalArithmeticFailsOpenOnAnyOperand(t *testing.T) {
	result := checkArith(t, `local function f(a: any, b: number): integer
    local x: integer = a + b
    return x
end
return f`)
	if hasPublished(result, "type.assignment", "not integer") {
		t.Fatalf("arithmetic with an any operand must fail open, got %#v", result.PublishedDiagnostics)
	}
}
