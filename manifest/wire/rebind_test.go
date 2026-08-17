package wire

import (
	"testing"

	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/types/signature"
)

func TestRebound_RekeysSignaturesToConsumingPath(t *testing.T) {
	m := New("app.lib:assert")
	fn := typ.Func().Param("val", typ.Any).Returns(typ.Any).Build()
	m.DefineFunctionSignature("app.lib:assert.not_nil", signature.Function{Type: fn})
	m.DefineFunctionSignature("app.lib:assert.Helper.check", signature.Function{Type: fn})

	rebound := m.Rebound("assert2")
	if rebound.Path != "assert2" {
		t.Fatalf("Path = %q, want assert2", rebound.Path)
	}
	if _, ok := rebound.FunctionSignatures["assert2.not_nil"]; !ok {
		t.Fatalf("missing re-keyed assert2.not_nil; have %v", keys(rebound.FunctionSignatures))
	}
	if _, ok := rebound.FunctionSignatures["assert2.Helper.check"]; !ok {
		t.Fatalf("missing re-keyed assert2.Helper.check; have %v", keys(rebound.FunctionSignatures))
	}
	// The original is untouched.
	if _, ok := m.FunctionSignatures["app.lib:assert.not_nil"]; !ok {
		t.Fatalf("Rebound mutated the source manifest")
	}
}

func TestRebound_SamePathReturnsOriginal(t *testing.T) {
	m := New("contract")
	if m.Rebound("contract") != m {
		t.Fatalf("Rebound to the same path should return the original manifest")
	}
}

func keys(m map[string]signature.Function) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
