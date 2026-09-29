-- A value typed any already admits every write: an integer-key write into it,
-- directly or through a function it is passed to, leaves it any instead of
-- turning it into a map, so it still passes where a list is expected
-- (kickside.workflows store_projection: state.edge_order).
local function load_state(): any?
    return { edge_order = {} }
end

local function order_add(order: any, id: string)
    if id == "" then return end
    order[#order + 1] = id
end

local function project_through_call(id: string)
    local state = load_state()
    if not state then return end
    order_add(state.edge_order, id)
    for i = #state.edge_order, 1, -1 do
        table.remove(state.edge_order, i)
    end
end

local function project_direct(id: string)
    local order = load_state()
    if not order then return end
    order[#order + 1] = id
    table.remove(order, 1)
end

return { project_through_call = project_through_call, project_direct = project_direct }
