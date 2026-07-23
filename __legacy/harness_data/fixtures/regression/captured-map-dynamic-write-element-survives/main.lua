local state = {sessions = {}}

local function add(id: string, started: number)
    state.sessions[id] = {created_at = started, last_activity = started}
end

local function sweep()
    for _, s in pairs(state.sessions) do
        local t: number = s.last_activity
        local c: number = s.created_at
    end
end

add("a", 1.0)
sweep()
