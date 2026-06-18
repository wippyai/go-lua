type Err = {kind: string, message: string}
type User = {id: string, name: string, roles: {string}}
type Session = {id: string, user: User, expires_at: number}

local users: {[string]: User} = {
    ["u1"] = {id = "u1", name = "Ada", roles = ({"admin"} :: {string})},
}

local M = {}
M.Err = Err
M.User = User
M.Session = Session

function M.find_user(id: string): (User?, Err?)
    local user = users[id]
    if not user then
        return nil, {kind = "not_found", message = id}
    end
    return user, nil
end

function M.create_session(user: User, now: number): (Session?, Err?)
    if #user.roles == 0 then
        return nil, {kind = "forbidden", message = user.id}
    end
    return {id = user.id .. ":" .. tostring(now), user = user, expires_at = now + 3600}, nil
end

function M.with_user(id: string, now: number, fn: (User, number) -> (Session?, Err?)): (Session?, Err?)
    local user, err = M.find_user(id)
    if err then
        return nil, err
    end
    return fn(user, now)
end

function M.describe(id: string, now: number): (string?, Err?)
    local session, err = M.with_user(id, now, M.create_session)
    if err then
        return nil, err
    end
    return session.user.name .. ":" .. tostring(session.expires_at), nil -- expect-warning: may be nil
end

return M
