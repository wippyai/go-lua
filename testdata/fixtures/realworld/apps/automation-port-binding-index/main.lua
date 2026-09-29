-- kickside/platform/automation/src/automation.lua: an index built into a
-- local annotated {[string]: any} is passed to a parameter with the same
-- annotation.
type RegistryEntry = { id: string?, meta: { [string]: any }? }

local function port_descriptor(port_entry: any, binding_index: { [string]: any }): ({ [string]: any }?, string?)
    local typed_port = port_entry :: RegistryEntry
    local binding = binding_index[tostring(typed_port.id or "")]
    if not binding then return nil, "unbound port" end
    return { id = typed_port.id, binding = binding }, nil
end

local function list_ports(bindings: {RegistryEntry}, ports: {RegistryEntry}): { { [string]: any } }
    local binding_index: { [string]: any } = {}
    for _, b in ipairs(bindings) do
        binding_index[tostring(b.id or "")] = b
    end

    local out: { { [string]: any } } = {}
    for _, port_entry in ipairs(ports) do
        local desc = port_descriptor(port_entry, binding_index)
        if desc then
            out[#out + 1] = desc
        end
    end
    return out
end

return list_ports
