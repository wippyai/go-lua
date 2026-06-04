package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestFunctionRefs_ProductStateBeatsImportedFieldFallback(t *testing.T) {
	moduleSource := `
local M = {}

function M.f(): string
	return "old"
end

return M
`
	exported := testutil.CheckAndExport(moduleSource, "mod", testutil.WithStdlib())
	if exported.HasError() {
		t.Fatalf("unexpected module export errors: %v", testutil.ErrorMessages(exported.Errors))
	}

	consumer := `
local M = require("mod")

M.f = function(): number
	return 1
end

local n: number = M.f()
`
	result := testutil.Check(consumer,
		testutil.WithStdlib(),
		testutil.WithModule("mod", exported),
	)
	if result.HasError() {
		t.Fatalf("expected rebinding to use solved FunctionRefs before imported field fallback, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFunctionRefs_MethodSummaryUsesReceiverNotBareName(t *testing.T) {
	source := `
local A = {}
function A:get(): string
	return "a"
end

local B = {}
function B:get(): number
	return 1
end

local n: number = B:get()
`
	result := testutil.Check(source,
		testutil.WithStdlib(),
	)
	if result.HasError() {
		t.Fatalf("expected method resolution to use receiver field identity, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFunctionRefs_AnnotatedReceiverParameterUsesConcreteMethodTarget(t *testing.T) {
	source := `
type Row = {[string]: any}
type DB = {
	query: fun(self: DB, sql: string): ({Row}?, string?),
}

local M = {}

function M.mock(): DB
	local database: DB = {
		query = function(self: DB, sql: string): ({Row}?, string?)
			return {{ count = 1 }}, nil
		end,
	}
	return database
end

local function table_exists(database: DB): boolean
	local result, query_err = database:query("SELECT 1")
	if query_err then
		return false
	end
	if result and result[1] then
		return result[1].count and result[1].count > 0
	end
	return false
end

return table_exists(M.mock())
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected concrete receiver method target to refine map row fields, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
