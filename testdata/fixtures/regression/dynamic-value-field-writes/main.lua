-- load_state returns a concrete table, so writes to that inferred record can
-- still establish head_seq as number. A dynamic field passed through a callee
-- remains usable as a list (kickside.workflows store_projection: edge_order).
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

-- A row read from any stays dynamic on both sides of a field write
-- (app:migration_lifecycle_test: row.decoded_body = json.decode(row.body)).
local function decode(source: string): any
    return source
end

local function decoded_row(rows: any): any
    local row = rows[1]
    row.decoded_body = decode(row.body)
    return row
end

return { project = project, decoded_row = decoded_row }
