-- Reduced from spiralscout.estimation:solver in kickside/estimation/test/hub.
local pin_of: { [string]: number } = {}
pin_of["milestone"] = 2460000
local snap: any = { type_of = { milestone = "milestone" }, node_ids = { { id = "milestone" } } }
local has_child: { [string]: boolean } = {}
local function is_milestone(id: string): boolean
    return (snap :: any).type_of[id] == "milestone" and pin_of[id] ~= nil
end
local function working_days_between(_start: number, pin: number): number
    return pin
end
for _, n in ipairs((snap :: any).node_ids) do
    local id = (n :: any).id
    if not has_child[id] and is_milestone(id) then
        local wd = working_days_between(2460000, pin_of[id])
    end
end

-- A write through the captured map invalidates the predicate's fact.
local id = "milestone"
if is_milestone(id) then
    pin_of[id] = nil
    local stale = working_days_between(2460000, pin_of[id]) -- expect-error
end

-- Creating an alias after the guard also ends the portable key fact.
local post_alias_id: string = ((snap :: any).node_ids[1] :: any).id
if is_milestone(post_alias_id) then
    local later_alias = pin_of
    later_alias[post_alias_id] = nil
    local stale_post_alias = working_days_between(2460000, pin_of[post_alias_id]) -- expect-error
end

-- A write through an alias invalidates the captured map fact as well.
local alias = pin_of
local alias_id: string = ((snap :: any).node_ids[1] :: any).id
if is_milestone(alias_id) then
    alias[alias_id] = nil
    local stale_alias = working_days_between(2460000, pin_of[alias_id]) -- expect-error
end

-- No predicate proof for unrelated keys.
local other = working_days_between(2460000, pin_of["other"]) -- expect-error

-- Passing the map to a call after the guard can expose a mutating alias.
local function clear_pin(map: { [string]: number }, key: string)
    map[key] = nil
end
local call_id: string = ((snap :: any).node_ids[1] :: any).id
if is_milestone(call_id) then
    clear_pin(pin_of, call_id)
    local stale_call = working_days_between(2460000, pin_of[call_id]) -- expect-error
end

local function clear_in_condition(map: { [string]: number }, key: string): boolean
    map[key] = nil
    return true
end
local condition_id: string = ((snap :: any).node_ids[1] :: any).id
if is_milestone(condition_id) then
    if clear_in_condition(pin_of, condition_id) then
    end
    local stale_condition = working_days_between(2460000, pin_of[condition_id]) -- expect-error
end

-- A call that received the map before the guard may retain it.
local saved: { [string]: number }? = nil
local function remember(map: { [string]: number })
    saved = map
end
remember(pin_of)
local escaped_id: string = ((snap :: any).node_ids[1] :: any).id
if is_milestone(escaped_id) then
    local escaped = saved
    if escaped then
        escaped[escaped_id] = nil
    end
    local stale_escape = working_days_between(2460000, pin_of[escaped_id]) -- expect-error
end
