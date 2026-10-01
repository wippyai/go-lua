-- Bee hive telemetry: a guarded link state returns a LinkFields record with nil-typed fields.
type Direction = "outbound" | "inbound"
type LinkState = {connected: false} | {connected: true, direction: Direction, remote_address: string}
type LinkStates = {[string]: LinkState}
type LinkFields = {connected: false, direction: nil, remote_address: nil}
    | {connected: true, direction: Direction, remote_address: string}
local function link_fields(node_id: string, links: LinkStates): LinkFields
    local state = links[node_id]
    if not state or not state.connected then
        return {connected = false, direction = nil, remote_address = nil}
    end
    return {connected = true, direction = state.direction, remote_address = state.remote_address}
end
return link_fields
