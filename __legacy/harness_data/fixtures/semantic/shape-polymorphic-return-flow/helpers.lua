local M = {}
local sink = {}

type Row = {
    meta: table,
}

function M.id(value: table): table
    return value
end

function M.default_or(value: table?, fallback: table): table
    return value or fallback
end

function M.get_meta(row: Row): table
    return row.meta
end

function M.touch(value: table): table
    value.late = 1
    return value
end

function M.maybe_store(value: table, flag: boolean): table
    if flag then
        sink.saved = value
    end
    return value
end

return M
