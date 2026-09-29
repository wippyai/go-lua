local M = {}
M._security = {} :: any

local function validate(identity: any): (boolean, string?)
    if type(identity) ~= "table" then return false, "identity must be a table" end
    return true, nil
end

local function actor_and_scope(id: any): (any?, any?, string?)
    local actor = M._security.new_actor(id.actor_id)
    if not actor then return nil, nil, "could not build actor" end
    local scope, scope_err = M._security.named_scope(id.scope_id)
    if scope_err or not scope then
        return nil, nil, "could not recover scope: " .. tostring(scope_err)
    end
    return actor, scope, nil
end

-- reconstruct(identity) -> (actor?, scope?, err?, error_kind?)
function M.reconstruct(identity: any): (any?, any?, string?, string?)
    local ok, verr = validate(identity)
    if not ok then return nil, nil, verr, "invalid" end
    local actor, scope, err = actor_and_scope(identity)
    return actor, scope, err, nil
end

return M
