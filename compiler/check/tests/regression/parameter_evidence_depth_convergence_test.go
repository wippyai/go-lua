package regression

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
)

func numericAliasChain(depth int) string {
	var b strings.Builder
	b.WriteString("type N0 = number\n")
	for i := 1; i <= depth; i++ {
		fmt.Fprintf(&b, "type N%d = N%d\n", i, i-1)
	}
	return b.String()
}

func TestParameterEvidence_DeepAliasChain_NoInterprocNonConvergenceWarning(t *testing.T) {
	code := numericAliasChain(32) + `
		local function g(x)
			return x + 1
		end

		local function f(v: N32): number
			return g(v)
		end

		local n: number = f(1)
	`

	result := testutil.Check(code, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityWarning && strings.Contains(d.Message, "inter-function fixpoint did not converge") {
			t.Fatalf("unexpected non-convergence warning: %v", d.Message)
		}
	}
}

func TestParameterEvidence_RecordWrapperFeedback_NoInterprocNonConvergenceWarning(t *testing.T) {
	code := `
		local repo = require("kb_repo")

		local function config()
			return {
				retrieval_iterations = tonumber(nil) or 2,
				initial_vector_limit = tonumber(nil) or 4,
				followup_vector_limit = tonumber(nil) or 2,
			}
		end

		local function search(question, kb_id, vec_limit, seen)
			seen = seen or {}
			local rows = repo.hybrid_search(question, kb_id, { limit = vec_limit })
			if rows then
				for _, row in ipairs(rows) do
					if row.node_id and not seen[row.node_id] then
						seen[row.node_id] = true
					end
				end
			end
			return seen
		end

		local function run(kb_id, question)
			local cfg = config()
			local seen = search(question, kb_id, cfg.initial_vector_limit)
			for _ = 1, math.min(cfg.retrieval_iterations, 3) do
				search(question, kb_id, cfg.followup_vector_limit, seen)
			end
			return seen
		end

		return run("kb", "question")
	`

	result := testutil.Check(code, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityWarning && strings.Contains(d.Message, "inter-function fixpoint did not converge") {
			t.Fatalf("unexpected non-convergence warning: %v", d.Message)
		}
	}
}

func TestParameterEvidence_NestedWrapperFeedback_NoInterprocNonConvergenceWarning(t *testing.T) {
	code := `
		local repo = require("kb_repo")

		local function config()
			return {
				limit = tonumber(nil) or 4,
				iterations = tonumber(nil) or 2,
			}
		end

		local function search(question, kb_id, limit, seen)
			seen = seen or {}
			local rows = repo.hybrid_search(question, kb_id, {
				query = question,
				options = {
					limit = limit,
					window = { limit },
				},
			})
			if rows then
				for _, row in ipairs(rows) do
					if row.node_id and not seen[row.node_id] then
						seen[row.node_id] = true
					end
				end
			end
			return seen
		end

		local function run(kb_id, question)
			local cfg = config()
			local seen = nil
			for _ = 1, math.min(cfg.iterations, 3) do
				seen = search(question, kb_id, cfg.limit, seen)
			end
			return seen
		end

		return run("kb", "question")
	`

	result := testutil.Check(code, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityWarning && strings.Contains(d.Message, "inter-function fixpoint did not converge") {
			t.Fatalf("unexpected non-convergence warning: %v", d.Message)
		}
	}
}

func TestParameterEvidence_OptionalContextTableFeedback_NoInterprocNonConvergenceWarning(t *testing.T) {
	code := `
		local function merge_context(base, additions)
			local out = {}
			if base then
				for k, v in pairs(base) do
					out[k] = v
				end
			end
			if additions then
				for k, v in pairs(additions) do
					out[k] = v
				end
			end
			return out
		end

		local function call_func(func_id: string, data: any, context: {[string]: any}?)
			return data, nil
		end

		local function run(items)
			local result = {}
			for index, item in ipairs(items) do
				local ctx = merge_context(nil, {
					current_item = item,
					item_index = index,
				})
				result[index] = call_func("item", item, ctx)
			end
			call_func("done", result)
			return result
		end

		return run({ "a", "b" })
	`

	result := testutil.Check(code, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityWarning && strings.Contains(d.Message, "inter-function fixpoint did not converge") {
			t.Fatalf("unexpected non-convergence warning: %v", d.Message)
		}
	}
}

func TestReturnSummary_RecursiveDeepCopy_NoInterprocNonConvergenceWarning(t *testing.T) {
	code := `
		local function deep_copy_table(original)
			if type(original) ~= "table" then
				return original
			end

			local copy = {}
			for key, value in pairs(original) do
				if type(value) == "table" then
					copy[key] = deep_copy_table(value)
				else
					copy[key] = value
				end
			end
			return copy
		end

		local source = { api = { routes = { users = true } } }
		return deep_copy_table(source)
	`

	result := testutil.Check(code, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityWarning && strings.Contains(d.Message, "inter-function fixpoint did not converge") {
			t.Fatalf("unexpected non-convergence warning: %v", d.Message)
		}
	}
}
