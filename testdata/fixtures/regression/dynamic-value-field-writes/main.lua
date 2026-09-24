-- Field writes into a value typed any record the written fields, and every
-- other key stays dynamic: an unwritten field reads as any, so an index write
-- through a callee leaves it a value that passes where a list is expected
-- (kickside.workflows store_projection: state.head_seq, state.edge_order).
local function load_state(): (any?, string?)
    return { edge_order = {} }, nil
end

local function order_add(order: any, id: string)
    if id == "" then return end
    order[#order + 1] = id
end

local function project(id: string, seq: number, removed: boolean, edit: boolean): string?
    local state, err = load_state()
    if err or not state then return err end
    state.head_seq = seq
    if edit then state.edit_seq = seq end
    if removed then
        for i = #state.edge_order, 1, -1 do table.remove(state.edge_order, i) end
    else
        order_add(state.edge_order, id)
    end
    local head: string = state.head_seq -- expect-error: cannot assign number to string
    return nil
end

return project
