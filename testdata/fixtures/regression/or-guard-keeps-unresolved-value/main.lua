-- On the branch where `err or not writer` holds, err is whatever it was when
-- it was truthy, or a falsy value: an unresolved err stays unresolved rather
-- than narrowing to its falsy part (automation_engine dispatch_to_sink).
local function unresolved<T>(): T
    return nil :: any
end

local function dispatch(): (any?, string?)
    local writer, err = unresolved(), unresolved()
    if err or not writer then
        return nil, err
    end
    return writer, nil
end

return dispatch
