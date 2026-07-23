type Request = {
    id: string,
    retries: number,
    tags: {[string]: string},
}

type Accepted = {
    id: string,
    attempt: number,
    source: string?,
}

type Rejected = {
    id: string,
    reason: string,
}

type Decision = Accepted | Rejected

local M = {}

M.Request = Request
M.Accepted = Accepted
M.Rejected = Rejected
M.Decision = Decision

return M
