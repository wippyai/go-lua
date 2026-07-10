type Conn = {
    id: string,
}

type Result = {
    conn: Conn,
    label: string,
}

local M = {}
M.Conn = Conn
M.Result = Result

return M
