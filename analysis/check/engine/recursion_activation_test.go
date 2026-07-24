package engine

import "testing"

func TestRecursiveLexicalTableCallActivatesDemand(t *testing.T) {
	result, err := Check(`
local function walk(n)
  if n == 0 then return { value = 0 } end
  local node = { value = 1 }
  return walk(0)
end
local result = walk(1)
local value: number = result.value

return value`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("recursive demand emitted diagnostics: %#v", result.Diagnostics)
	}
	if len(result.Values) != 1 || result.Values[0].Key != "value" || string(result.Values[0].Value) != "0" {
		t.Fatalf("recursive table call values = %#v, want value=0", result.Values)
	}
	if len(result.Outcomes) != 2 || result.Outcomes[0].Key != "return/0" || string(result.Outcomes[0].Value) != "0" || result.Outcomes[1].Key != "return/arity" || string(result.Outcomes[1].Value) != "1" {
		t.Fatalf("recursive table call outcomes = %#v, want return/0=0 and arity=1", result.Outcomes)
	}
}
