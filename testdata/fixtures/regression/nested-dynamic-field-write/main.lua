-- Writing a field of an entry reached through a variable key, t[k].f = v,
-- keeps the entries records whatever path the value comes from
-- (dataflow workflow_state: marking a child node cancelled from a status table).
local STATUS = { PENDING = "pending", CANCELLED = "cancelled" }
local methods = {}

function methods:cancel(pending: { [string]: string }, group_key: string)
    local function in_group(child_id: string): boolean
        local child = self.nodes[child_id]
        return child and type(child.metadata) == "table" and child.metadata[group_key] == true
    end
    for child_id, cached_status in pairs(pending) do
        if cached_status == STATUS.PENDING and in_group(child_id) then
            self.nodes[child_id].status = STATUS.CANCELLED
        end
    end
end

return methods
