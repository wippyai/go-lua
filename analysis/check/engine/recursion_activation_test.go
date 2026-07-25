package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
)

func TestRecursiveLexicalTableCallActivatesDemand(t *testing.T) {
	const source = `
local function walk(n)
  if n == 0 then return { value = 0 } end
  local node = { value = 1 }
  return walk(0)
end
local result = walk(1)
local value: number = result.value

return value`
	result, err := Check(source)
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

	compilation, err := front.Compile(source)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	lexical := newLexicalEvaluator(compilation)
	closure, _, err := lexical.evaluate(compilation, []byte(entryValue))
	if err != nil {
		t.Fatalf("evaluate recursive lexical closure: %v", err)
	}
	returns, err := childReturnValues(closure, true)
	if err != nil {
		t.Fatalf("recursive lexical closure returns: %v", err)
	}
	if len(returns) != 1 || string(returns[0]) != "scalar/number/0" {
		t.Fatalf("recursive lexical closure return = %#v, want exact scalar/number/0", returns)
	}
	metrics := lexical.coordinator.Metrics()
	// walk(1) discovers its recursive walk(0) instance, so resolving only the
	// root would leave this test unable to distinguish the demand bridge from a
	// normal one-body evaluation.
	if metrics.Groups < 2 || metrics.AtomicCommits < 2 || metrics.Failures != 0 {
		t.Fatalf("recursive lexical closure did not resolve through SCC coordinator: %#v", metrics)
	}
}
