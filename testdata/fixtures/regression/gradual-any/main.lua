-- any is the dynamic type: a value typed any is accepted wherever a type is
-- expected, and any accepts every value. Under strict-any, any behaves like
-- unknown and must be narrowed before such a use.
type Map = { [string]: any }

local function take_map(m: Map): integer
    local n = 0
    for _ in pairs(m) do
        n = n + 1
    end
    return n
end

local function take_name(name: string): string
    return name
end

-- automation_engine execute_binding: a dynamic argument table passed on.
local function execute_run(binding_id: string, a: Map): Map
    return { binding_id = binding_id, input = a }
end

local function execute_binding(args: any): Map
    return execute_run("b-1", args) -- expect-error[strict-any]: argument 2
end

-- A row decoded from storage carries dynamic fields.
local function config_size(row: { cfg: any }): integer
    return take_map(row.cfg) -- expect-error[strict-any]: argument 1
end

-- dataflow node: keys collected from a dynamic table into a string-keyed map.
local function merge_context(ctx: any, extra: { dataflow_id: any, node_id: any }): Map
    local merged: { [any]: any } = { dataflow_id = extra.dataflow_id, node_id = extra.node_id }
    for k, v in pairs(ctx) do
        merged[k] = v
    end
    return merged -- expect-error[strict-any]: cannot return
end

local function label(v: any): string
    local text: string = v -- expect-error[strict-any]: cannot assign any to string
    return take_name(text)
end

-- unknown is not consistent with specific types in either mode: a type
-- argument nothing determines stays unknown and must be narrowed.
local function decode<T>(raw: string): T
    return raw :: T
end

local function first_key(raw: string): string
    return take_name(decode(raw)) -- expect-error: argument 1: expected string, got unknown
end

return {
    execute_binding = execute_binding,
    config_size = config_size,
    merge_context = merge_context,
    label = label,
    first_key = first_key,
}
