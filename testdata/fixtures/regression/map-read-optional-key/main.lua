-- Reading a map with a key that may be nil yields the value or nil: t[nil]
-- reads nil in Lua (keeper hub service: by_name[summary.component]).
type Module = {name: string, used_by_count: integer}

local function usage(by_name: {[string]: Module}, component: string?): integer
    local m = by_name[component]
    if m then
        return m.used_by_count
    end
    return 0
end

return { usage = usage }
