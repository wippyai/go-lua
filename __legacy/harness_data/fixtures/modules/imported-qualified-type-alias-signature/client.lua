local atlassian_types = require("atlassian_types")

local M = {}

function M.describe(conn: atlassian_types.Conn): atlassian_types.Result
    return {
        conn = conn,
        label = conn.id,
    }
end

return M
