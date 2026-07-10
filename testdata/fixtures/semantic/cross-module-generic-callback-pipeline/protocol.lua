type User = {
    id: string,
    retries: number,
}

type Audit = {
    user_id: string,
    event: string,
}

local M = {}

M.User = User
M.Audit = Audit

return M
