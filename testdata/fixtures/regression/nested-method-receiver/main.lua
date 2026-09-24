-- A method defined on a table inside another method takes that table as its
-- receiver, not the enclosing method's self (keeper task nodes writer:
-- ws:node() builds a node handle whose methods call each other).
local M = {}

function M.new(task_id: string)
    local ws = {}
    function ws:node(parent_node_id)
        local node = { task_id = task_id, parent_node_id = parent_node_id }
        function node:add(spec)
            spec = spec or {}
            spec.parent_node_id = parent_node_id
            return { node_id = "n" .. tostring(spec.parent_node_id) }, nil
        end
        function node:open(spec)
            local row, err = node:add(spec)
            if err then return nil, err end
            return ws:node(row.node_id), nil
        end
        return node
    end
    return ws
end

return M
