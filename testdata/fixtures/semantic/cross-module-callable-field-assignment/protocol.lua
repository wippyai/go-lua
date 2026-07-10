type Request = {
    id: string,
    retries: number,
}

type Accepted = {
    ok: true,
    label: string,
}

type Rejected = {
    ok: false,
    reason: string,
}

type Response = Accepted | Rejected
type Handler = (Request) -> Response

local M = {}

function M.accept(req: Request): Response
    return {
        ok = true,
        label = req.id .. ":" .. tostring(req.retries),
    }
end

function M.reject(req: Request): Response
    return {
        ok = false,
        reason = req.id,
    }
end

return M
