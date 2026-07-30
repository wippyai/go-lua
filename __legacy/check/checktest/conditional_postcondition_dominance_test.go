package checktest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/diagnostic"
)

// A helper's assert/error postcondition is only a valid caller-visible fact
// when it unconditionally dominates every normal return of the helper. Two
// fixtures pin both directions of this gap: wrapper-conditional-fails (the
// checker must NOT assume a postcondition gated by an unrelated boolean
// parameter) and inferred-conditional-error (the checker MUST narrow when the
// helper always terminates on the checked boolean expression). Distilled
// from the full-oracle fixtures narrowing/wrapper-conditional-fails and
// narrowing/inferred-conditional-error.

func TestConditionallyGatedAssertIsNotAnUnconditionalPostcondition(t *testing.T) {
	result := Check(`
local function conditionalAssert(val: any, check: boolean)
    if check then
        assert(val, "value is nil")
    end
end
function process(x: string?)
    conditionalAssert(x, true)
    local s: string = x
end
`)
	for _, d := range result.Diagnostics {
		if d.Severity == diagnostic.SeverityError {
			return
		}
	}
	t.Fatalf("diagnostics = %#v, want an error: the assert inside conditionalAssert is gated by `check`, a plain boolean parameter, not a proof that val is non-nil at every call site", result.Diagnostics)
}

func TestConditionalTerminationNarrowsCheckedBooleanExpressionOnNormalReturn(t *testing.T) {
	result := Check(`
local function maybeError(cond: boolean)
    if cond then
        error("condition was true")
    end
end
function process(x: string?)
    maybeError(x == nil)
    local s: string = x
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none: maybeError always terminates when its argument is true, so a normal return from maybeError(x == nil) proves x ~= nil", result.Diagnostics)
	}
}

// TestUnconditionallyDominatingAssertStillPublishesPostcondition is the
// companion proving no precision loss: when the assert is the only statement
// in the helper's body, it dominates every normal exit unconditionally, so the
// caller must still see val narrowed to non-nil after the call.
func TestUnconditionallyDominatingAssertStillPublishesPostcondition(t *testing.T) {
	result := Check(`
local function assertNotNil(val: any)
    assert(val, "value must not be nil")
end
function process(x: string?)
    assertNotNil(x)
    local s: string = x
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none: assertNotNil's assert unconditionally dominates its only normal exit, so x is proven non-nil after the call", result.Diagnostics)
	}
}

// TestErrorGuardedOnSameValueStillNarrowsThatValue is the companion proving
// real dominance still narrows: the guard here checks x itself (the value
// later read), not an unrelated parameter, so the exit it dominates is a
// genuine proof and must still narrow x.
func TestErrorGuardedOnSameValueStillNarrowsThatValue(t *testing.T) {
	result := Check(`
function process(x: string?)
    if x == nil then
        error("value is nil")
    end
    local s: string = x
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none: the error() guard on x == nil dominates the rest of the function, so x is proven non-nil afterward", result.Diagnostics)
	}
}
