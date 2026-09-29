package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// A constructor that returns setmetatable(literal, mt) exports a record that
// carries its metatable, so an importer resolves the builder methods on the
// result and on every copy a method returns.
func TestConstructorExportCarriesMetatable(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "methods_table_index",
			source: `
local reader = {}
local methods = {}
local reader_mt = { __index = methods }

function methods:_copy()
    local new = {}
    for k, v in pairs(self) do new[k] = v end
    return setmetatable(new, reader_mt)
end

function reader.tasks()
    return setmetatable({
        _status = nil,
        _limit = nil,
        _order = "created_at DESC",
    }, reader_mt)
end

function methods:with_status(status)
    local c = self:_copy(); c._status = status; return c
end

function methods:limit(n)
    local c = self:_copy(); c._limit = n; return c
end

function methods:all()
    return {}, nil
end

return reader
`,
		},
		{
			name: "self_index",
			source: `
local reader = {}
local Query = {}
Query.__index = Query

function Query:_copy()
    local new = {}
    for k, v in pairs(self) do new[k] = v end
    return setmetatable(new, Query)
end

function reader.tasks()
    return setmetatable({
        _status = nil,
        _limit = nil,
        _order = "created_at DESC",
    }, Query)
end

function Query:with_status(status)
    local c = self:_copy(); c._status = status; return c
end

function Query:limit(n)
    local c = self:_copy(); c._limit = n; return c
end

function Query:all()
    return {}, nil
end

return reader
`,
		},
	}

	consumer := `
local reader = require("task_reader")

local function handler(status: string?)
    local q = reader.tasks()
    if status then q = q:with_status(status) end
    q = q:limit(100)
    local tasks, err = q:all()
    return tasks, err
end

return handler
`

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mod := testutil.CheckAndExport(tt.source, "task_reader", testutil.WithStdlib())
			if mod.HasError() {
				t.Fatalf("module errors: %v", testutil.ErrorMessages(mod.Errors))
			}
			export := unwrap.Record(mod.Manifest.EnrichedExport())
			if export == nil {
				t.Fatalf("export is not a record: %v", mod.Manifest.Export)
			}
			tasks := export.GetField("tasks")
			if tasks == nil {
				t.Fatal("export lacks tasks")
			}
			fn := unwrap.Function(tasks.Type)
			if fn == nil || len(fn.Returns) == 0 {
				t.Fatalf("tasks is not a function with returns: %v", tasks.Type)
			}
			ret := unwrap.Record(fn.Returns[0])
			if ret == nil || ret.Metatable == nil {
				t.Fatalf("tasks return lacks a metatable: %s", typ.FormatShort(fn.Returns[0]))
			}

			result := testutil.Check(consumer, testutil.WithStdlib(), testutil.WithModule("task_reader", mod))
			if result.HasError() {
				t.Fatalf("consumer errors: %v", testutil.ErrorMessages(result.Diagnostics))
			}
		})
	}
}
