package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestCallbackWithoutReturnSatisfiesNilableReturnContract(t *testing.T) {
	source := `
		local function apply(fn: fun(changes: unknown?): unknown?)
			fn(nil)
		end

		apply(function(changes)
			local _ = changes
		end)
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected callback with missing return to satisfy nilable return contract, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestInferredCallbackWithoutReturnSatisfiesNilableReturnEvidence(t *testing.T) {
	source := `
		local function apply_changes(apply_fn)
			local changes = nil
			apply_fn(changes)
			return 1
		end

		local version = apply_changes(function(changes)
			local _ = changes
		end)

		local _: integer = version
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected inferred callback with missing return to satisfy nilable return evidence, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestInferredMethodCallbackWithoutReturnSatisfiesNilableReturnEvidence(t *testing.T) {
	source := `
		type Changes = {
			delete: (self: Changes, id: string) -> (),
			apply: (self: Changes) -> (integer, string?),
		}
		type Snapshot = {
			changes: (self: Snapshot) -> Changes,
		}

		local registry = {
			snapshot = function(): (Snapshot, string?)
				local changes: Changes = {} :: Changes
				local snap: Snapshot = {
					changes = function(_)
						return changes
					end,
				}
				return snap, nil
			end,
		}

		local function apply_changes(apply_fn)
			local snap, err = registry.snapshot()
			if err then
				return nil
			end
			local changes = snap:changes()
			apply_fn(changes)
			local version, apply_err = changes:apply()
			if apply_err then
				return nil
			end
			return version
		end

		local version = apply_changes(function(changes)
			changes:delete("dep")
		end)

		if version then
			local _: integer = version
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected inferred method callback with missing return to satisfy nilable return evidence, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
