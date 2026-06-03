package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestAnyCastAllowsDynamicFieldAccess(t *testing.T) {
	source := `
		local maybe_false: false? = false
		local dyn = maybe_false :: any
		local v = dyn.usage
		local w = dyn["metadata"]
		local x = dyn:method_call()
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no checker errors, got %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// A field read off an `any` base stays opaque. It must be narrowed or asserted
// before concrete string concatenation; `any` is not proof of stringability.
func TestAnyBaseFieldReadRequiresProofBeforeConcat(t *testing.T) {
	source := `
		local x = nil :: any
		return x.field .. "s"
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatalf("expected any field read concat to require proof")
	}
}

// A nested field read off `any` also stays opaque. Arithmetic requires a numeric
// proof from a guard/assertion/cast rather than gradual-top compatibility.
func TestAnyBaseNestedFieldReadRequiresProofBeforeArithmetic(t *testing.T) {
	source := `
		local x = nil :: any
		return x.a.b + 1
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatalf("expected any nested field arithmetic to require proof")
	}
}

// An index read off `any` is still opaque. Numeric use must be proved first.
func TestAnyBaseIndexReadRequiresProofBeforeArithmetic(t *testing.T) {
	source := `
		local x = nil :: any
		local k = "key"
		return x[k] + 1
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatalf("expected any index arithmetic to require proof")
	}
}

// A fallback chain containing `any` does not fabricate numeric evidence. The
// whole expression must be guarded/asserted before arithmetic or ordering.
func TestAnyAndOrDefaultChainRequiresProofBeforeNumericUse(t *testing.T) {
	source := `
		local stats = nil :: any
		local d = (stats.a and stats.a.b or 0) - (stats.c and stats.c.d or 0)
		if d > 0 then end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatalf("expected any and/or default arithmetic to require proof")
	}
}

// An unannotated public method parameter may project to `any`, but the method
// body still must prove `u.name` is stringable before returning a string.
func TestAnyGradualParamFieldReadRequiresProofBeforeStringReturn(t *testing.T) {
	source := `
		local M = {}
		function M.greet_user(u): string
			return "Hello, " .. u.name
		end
		return M
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatalf("expected gradual param field concat to require proof")
	}
}

// An `unknown` base stays opaque: a field read off it remains `unknown` and is
// rejected by the concatenation operator.
func TestUnknownBaseFieldReadStaysOpaque(t *testing.T) {
	source := `
		local x = nil :: unknown
		return x.field .. "s"
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatalf("expected unknown field read to be rejected in concatenation")
	}
}

// A concrete record field read keeps its precise type: assigning a string field
// to a number target is rejected.
func TestConcreteRecordFieldReadStaysPrecise(t *testing.T) {
	source := `
		local r: {name: string} = {name = "a"}
		local n: number = r.name
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatalf("expected concrete record field read to keep precise string type")
	}
}
