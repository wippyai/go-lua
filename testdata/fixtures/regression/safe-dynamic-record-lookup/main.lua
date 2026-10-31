-- From spiralscout.estimation:consolidator in estimation/test/hub.
local OPEN_STATUSES = { planned = true, ready = true, in_progress = true }
local function make_live(raw: any): {[string]: {status: string}}
    return { first = { status = tostring(raw or "planned") } }
end
local live = make_live("done")

local function count_open(id: string): number
    return OPEN_STATUSES[live[id].status] and 1 or 0
end

local missing = OPEN_STATUSES["missing"]
local wrong_key = OPEN_STATUSES[1] -- expect-error: cannot index

return count_open("first")
