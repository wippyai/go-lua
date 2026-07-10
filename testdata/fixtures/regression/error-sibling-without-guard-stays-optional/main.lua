type DB = {release: fun(self)}

local real_db: DB = {release = function(self) end}

local function fetch(ok: boolean): (DB?, string?)
    if not ok then
        return nil, "failed"
    end
    return real_db
end

-- No `if err` guard: db may still be nil, so the method call is rejected.
local function use(ok: boolean)
    local db, err = fetch(ok)
    db:release() -- expect-error
end

return use
