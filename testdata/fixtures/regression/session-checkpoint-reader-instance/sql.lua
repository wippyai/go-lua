-- Typed stand-in for the runtime sql module: the builder and executor shapes
-- the session repositories use, with the runtime's result types.
type Row = {[string]: any}
type ExecResult = {rows_affected: integer, last_insert_id: integer?}

local Executor = {}
Executor.__index = Executor

function Executor:query(): ({Row}?, string?)
    return {}, nil
end

function Executor:exec(): (ExecResult?, string?)
    return { rows_affected = 0 }, nil
end

local Builder = {}
Builder.__index = Builder

function Builder:from(table_name: string) return self end
function Builder:where(...: any) return self end
function Builder:order_by(...: string) return self end
function Builder:limit(n: integer) return self end
function Builder:offset(n: integer) return self end
function Builder:set(column: string, value: any) return self end
function Builder:set_map(values: {[string]: any}) return self end
function Builder:run_with(db: any) return setmetatable({}, Executor) end

local DB = {}
DB.__index = DB
function DB:release() end
function DB:begin(): (any, string?) return {}, nil end

local function new_builder() return setmetatable({}, Builder) end

local sql = {
    builder = {
        select = function(...: string) return new_builder() end,
        insert = function(table_name: string) return new_builder() end,
        update = function(table_name: string) return new_builder() end,
        delete = function(table_name: string) return new_builder() end,
        expr = function(expr: string, ...: any) return { expr = expr } end,
        and_ = function(parts: {any}) return { parts = parts } end,
    },
    as = {
        null = function() return nil end,
        int = function(v: any) return v end,
        text = function(v: any) return v end,
    },
    get = function(resource: string) return setmetatable({}, DB), nil end,
}

return sql
