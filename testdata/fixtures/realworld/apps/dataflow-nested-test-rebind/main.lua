-- dataflow/src/runner/workflow_state_test.lua: a local query helper called from
-- test closures nested two levels deep returns the rewritten query string.
local function rebind(query, db_type)
    if db_type ~= "postgres" then return query end
    local i = 0
    return (query:gsub("%?", function()
        i = i + 1
        return "$" .. i
    end))
end

local function execute(q: string): boolean
    return q ~= ""
end

local function describe(name: string, fn: () -> ())
    fn()
end

describe("suite", function()
    local dt = "sqlite"
    describe("inner", function()
        execute(rebind("DELETE FROM a WHERE id = ?", dt))
    end)
    execute(rebind([[
        INSERT INTO b VALUES (?)
    ]]))
end)
return rebind
