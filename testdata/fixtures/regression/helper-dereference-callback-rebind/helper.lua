local M = {}
function M.rows(value: any, callback: () -> ()): any
    local r = value:query("SELECT node_id FROM estimate_node", {})
    callback()
    return r
end
return M
