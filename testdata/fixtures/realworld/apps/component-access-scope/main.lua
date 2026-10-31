-- kickside/core/component/src/persist/access_scope_test.lua: a test helper
-- whose optional path is passed as nil by most callers.
local next_id = 0
local sql = {
    builder = {
        insert = function(table_name: string)
            local query = {}
            function query:set_map(values: {[string]: any})
                return self
            end
            function query:run_with(db: any)
                return self
            end
            function query:exec(): (any, string?)
                return true, nil
            end
            return query
        end,
    },
}

local function make_component(db, path)
    next_id = next_id + 1
    local id = "component-" .. next_id
    local _, err = sql.builder.insert("components")
        :set_map({ component_id = id, impl_id = "test:scope", private_context = "{}", path = path })
        :run_with(db):exec()
    if err then error("component insert failed: " .. tostring(err)) end
    return id
end

local db = {}
local mine = make_component(db, nil)
local theirs = make_component(db, nil)
local nested = make_component(db, "/team/shared")
return { mine, theirs, nested }
