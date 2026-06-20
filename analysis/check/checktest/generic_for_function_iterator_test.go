package checktest

import (
	"testing"
)

// TestGenericForStatelessFunctionIteratorTypesLoopVariableAsString proves a
// generic-for loop over a stateless function iterator types the loop variable
// from the iterator function's result. gmatch returns fun(): string?, so w is
// string inside the loop body; assigning it to a number annotation is a type
// error.
func TestGenericForStatelessFunctionIteratorRejectsNumberAnnotation(t *testing.T) {
	result := Check(`
local s: string = "hello world"
for w in s:gmatch("%a+") do
	local n: number = w
end
`, WithStdlib())
	if len(result.Diagnostics) == 0 {
		t.Fatalf("diagnostics = %#v, want a type error for assigning string loop variable to number", result.Diagnostics)
	}
}

// TestGenericForStatelessFunctionIteratorAcceptsStringAnnotation proves the same
// loop variable is the iterator function's non-nil first result: assigning it to
// a string annotation checks clean.
func TestGenericForStatelessFunctionIteratorAcceptsStringAnnotation(t *testing.T) {
	result := Check(`
local s: string = "hello world"
for w in s:gmatch("%a+") do
	local ok: string = w
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for assigning string loop variable to string", result.Diagnostics)
	}
}
