type DB = {release: fun(self)}

local real_db: DB = {release = function(self) end}

-- The inner function proves the (value, err) inverse: failure returns (nil, err);
-- the success path is a short return (error slot implicitly nil).
local function open_db(ok: boolean): (DB?, string?)
    if not ok then
        return nil, "failed"
    end
    return real_db
end

-- The wrapper's body is a sole tail call: it delegates the whole result vector to
-- open_db, so it carries open_db's proven correlation.
local function get_db(ok: boolean): (DB?, string?)
    return open_db(ok)
end

-- The caller's `if err` guard narrows db non-nil through the delegated correlation.
local function use(ok: boolean)
    local db, err = get_db(ok)
    if err then
        error(err)
    end
    db:release()
end

return use
