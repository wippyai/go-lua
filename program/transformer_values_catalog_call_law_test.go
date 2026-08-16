package program_test

import (
	"testing"

	programlower "github.com/wippyai/go-lua/program/lower"
)

func TestTransformerValuesCatalogTwoCallsSharingValuesRemainDistinct(t *testing.T) {
	published, err := programlower.Lower(programlower.Source{Name: "transformer-values-shared-calls.lua", Text: []byte(`
local function f(a, b)
  sink(a, b)
  sink(a, b)
end
f(1, 2)
`)})
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}
	input := published.TransformerInput()
	seen := make(map[string]struct{})
	for index := 0; index < input.CallCount(); index++ {
		call, callOK := input.CallAt(index)
		if !callOK {
			t.Fatalf("CallAt(%d) unavailable", index)
		}
		values, valuesOK := call.Values()
		if !valuesOK {
			continue
		}
		callValuesID := values.ContextID()
		if _, exists := seen[callValuesID.String()]; exists {
			t.Fatal("two Calls sharing one Values row reused the CallValues semantic ID")
		}
		seen[callValuesID.String()] = struct{}{}
	}
}
