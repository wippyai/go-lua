package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestOrderedComparison_DefaultedNumberRemainsNumericAfterIfJoin(t *testing.T) {
	source := `
		local repo = {
			list = function(limit: number)
				return limit
			end,
		}

		local function load(limit)
			limit = limit or 500
			if limit < 1 then
				limit = 1
			end
			return repo.list(limit + 1)
		end

		local a: number = load(nil)
		local b: number = load(10)
		return a + b
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected ordered comparison to keep defaulted limit numeric after join, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestOrderedComparison_RejectsKnownNonNumericOperand(t *testing.T) {
	source := `
		local limit: string = "bad"
		if limit < 1 then
			return limit
		end
		return limit
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatal("expected known string-to-number comparison to be rejected")
	}
	if !strings.Contains(strings.Join(testutil.ErrorMessages(result.Diagnostics), "\n"), "expected both operands") {
		t.Fatalf("expected numeric diagnostic, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestOrderedComparison_StringOperandIsRefinedOnBothEdges(t *testing.T) {
	source := `
		local function classify(value)
			if value < "m" then
				local s: string = value
				return s
			end
			local s: string = value
			return s
		end

		local a: string = classify("abc")
		local b: string = classify("xyz")
		return a .. b
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected ordered comparison to refine string operand on true and false edges, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
