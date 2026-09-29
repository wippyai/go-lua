package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestRecursiveStructuralSelfMethodAssignment(t *testing.T) {
	source := `
type I = { n: (self: I) -> number }
type J = { n: (self: J) -> number }

local j: J = {
    n = function(self: J): number return 1 end,
}
local i: I = j
local result: number = i:n()
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("structurally identical recursive self methods should assign: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestRecursiveStructuralMetatableBuilderChain(t *testing.T) {
	source := `
local QueryBuilder = {}
QueryBuilder.__index = QueryBuilder

type QueryOptions = {limit: integer?}
type QueryBuilderInstance = {
    filter: unknown?,
    with_filter: (self: QueryBuilderInstance, filter: unknown) -> QueryBuilderInstance,
    execute: (self: QueryBuilderInstance, options: QueryOptions) -> ({string}, string?),
    run: (self: QueryBuilderInstance, filter: unknown) -> number,
}

function QueryBuilder:with_filter(filter: unknown): QueryBuilderInstance
    self.filter = filter
    return self
end

function QueryBuilder:execute(options: QueryOptions): ({string}, string?)
    local out = { "row" }
    if options and options.limit then
        out[2] = tostring(options.limit)
    end
    return out, nil
end

function QueryBuilder:run(filter: unknown): number
    local first, err = self:execute({ limit = 10 })
    if err then return 0 end
    local chained = self:with_filter(filter)
    local second, err2 = chained:execute({ limit = 5 })
    if err2 then return 0 end
    return #first + #second
end

local function new_builder(): QueryBuilderInstance
    local self: QueryBuilderInstance = {
        filter = nil,
        with_filter = QueryBuilder.with_filter,
        execute = QueryBuilder.execute,
        run = QueryBuilder.run,
    }
    setmetatable(self, QueryBuilder)
    return self
end

local builder = new_builder()
local count: number = builder:run({ kind = "active" })
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("metatable builder chain should type-check: %v", testutil.ErrorMessages(result.Diagnostics))
	}
	strict := testutil.Check(source, testutil.WithStdlib(), testutil.WithCheckOptions(check.Options{Strict: true}))
	if messages := strings.Join(testutil.ErrorMessages(strict.Diagnostics), "\n"); !strings.Contains(messages, "cannot return QueryBuilder") {
		t.Fatalf("strict mode must reject a method that promises an instance for an arbitrary receiver: %v", testutil.ErrorMessages(strict.Diagnostics))
	}
}

func TestRecursiveStructuralBuilderMissingRequiredMethod(t *testing.T) {
	source := `
local Prototype = {}
Prototype.__index = Prototype

type Instance = {
    run: (self: Instance) -> number,
    execute: (self: Instance) -> number,
}

function Prototype:run(): number
    return 1
end

local function new_instance(): Instance
    local self: Instance = { run = Prototype.run }
    setmetatable(self, Prototype)
    return self
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatal("instance requiring a method absent from the prototype should be rejected")
	}
	if messages := strings.Join(testutil.ErrorMessages(result.Diagnostics), "\n"); !strings.Contains(messages, "execute") {
		t.Fatalf("expected a missing execute method error, got: %s", messages)
	}
}
