-- Each branch reassigns `value` to a record wrapping its previous value.
-- SCC inference joins every branch into one type per round, and each round's
-- records share the previous round's type, so the inferred type is a DAG whose
-- tree expansion grows with branches^rounds.
local function read(view: string, source: any): any
    local value = nil
    local err: string? = nil
    if view == "brief" then
        value, err = source.brief()
        if value then value = { brief = value } end
    elseif view == "context" then
        value, err = source.context()
        if value then value = { context = value } end
    elseif view == "authority" then
        value, err = source.authority()
        if value then value = { authority = value } end
    elseif view == "search" then
        value, err = source.search()
        if value then value = { events = value } end
    elseif view == "references" then
        value, err = source.references()
        if value then value = { references = value } end
    elseif view == "tasks" then
        value, err = source.tasks()
        if value then value = { tasks = value } end
    elseif view == "frontier" then
        value, err = source.frontier()
        if value then value = { frontier = value } end
    elseif view == "changes" then
        value, err = source.changes()
        if value then value = { changes = value } end
    else
        err = "unknown view"
    end
    if err then return { success = false, error = err } end
    local out = { success = true, view = view }
    for key, item in pairs(value or {}) do out[key] = item end
    return out
end

return { read = read }
