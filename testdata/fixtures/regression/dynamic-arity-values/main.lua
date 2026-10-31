-- A call to a dynamic function and a vararg expression produce an unknown
-- number of values: every target they fill is dynamic, not nil
-- (kb10 enrichment_service: a store loader taken from an any-typed deps table).
local function store_for_component(component_id: string): (any, any, any, string?)
    return component_id, { kb_type = "graph" }, nil, nil
end

local function run_enrichment(component_id: string, deps: any)
    local load_store = deps.store_for_component or store_for_component
    local store, store_data, engine, store_err = load_store(component_id)
    if store_err then
        return nil
    end
    if not (engine and engine.is_graph) then
        return store_data.kb_type
    end
    return store
end

local function method_values(client: any)
    local rows, total, err = client:query("x")
    if err then
        return 0
    end
    return total.count + #rows
end

local function forward(...: any)
    local first, second = ...
    return first.id, second.id
end

return { run_enrichment = run_enrichment, method_values = method_values, forward = forward }
