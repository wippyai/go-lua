-- Extracted from app:orders_events_test, lines 186-204.
-- Registry entries expose data as any; the fallback is the entry itself.
local function source_port_binding(port: { data: any, meta: { type: string } }): string
    local d = port.data or port
    return tostring(d.binding)
end

local function sink_port_operations(sink_port: { data: any, meta: { type: string } })
    local binding = tostring((sink_port.data or sink_port).binding)
    local upsert = ((sink_port.data or sink_port).operations or {}).upsert
    return binding, upsert
end

return { source_port_binding = source_port_binding, sink_port_operations = sink_port_operations }
