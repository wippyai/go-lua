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

// A field read off a gradual `any` base yields `any`, which is admitted by the
// concatenation operator (and concatenation result is a string).
func TestAnyBaseFieldReadIsAnyInConcat(t *testing.T) {
	source := `
		local x = nil :: any
		return x.field .. "s"
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected any field read to concatenate, got %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// A nested field read off a gradual `any` base stays `any` through arithmetic.
func TestAnyBaseNestedFieldReadIsAnyInArithmetic(t *testing.T) {
	source := `
		local x = nil :: any
		return x.a.b + 1
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected any nested field read to admit arithmetic, got %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// An index read off a gradual `any` base yields `any`, usable in arithmetic.
func TestAnyBaseIndexReadIsAnyInArithmetic(t *testing.T) {
	source := `
		local x = nil :: any
		local k = "key"
		return x[k] + 1
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected any index read to admit arithmetic, got %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// A gradual `any` value derived through an and/or default chain stays `any`
// and remains usable as a numeric operand and orderable comparison operand.
func TestAnyAndOrDefaultChainStaysUsableNumeric(t *testing.T) {
	source := `
		local stats = nil :: any
		local d = (stats.a and stats.a.b or 0) - (stats.c and stats.c.d or 0)
		if d > 0 then end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected any and/or default arithmetic to type-check, got %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// An unannotated public method parameter reads as gradual `any`, so a field
// read on it concatenates to a string the declared return admits.
func TestAnyGradualParamFieldReadConcatReturnsString(t *testing.T) {
	source := `
		local M = {}
		function M.greet_user(u): string
			return "Hello, " .. u.name
		end
		return M
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected gradual param field concat to return string, got %v", testutil.ErrorMessages(result.Diagnostics))
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
