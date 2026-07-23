local function getData(ok: boolean): (string?, string?)
    if ok then
        return "data", nil
    end
    return nil, "failed"
end

local function use(ok: boolean)
    local data, err = getData(ok)
    if err then
        error(err)
    end
    local s: string = data
    return s
end

return use
