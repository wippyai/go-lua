local M = {}
function M.resolve(exec: any, actor_id: string, idea_id: string): (string?, string?)
    if idea_id == "missing" then return nil, "missing thread" end
    return "thread", nil
end
function M.append(exec: any, thread_id: string, event: string, payload: any, actor: string?): string?
    return nil
end
M.ensure = {} :: any
M.executor = {} :: any
return M
