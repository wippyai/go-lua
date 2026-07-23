local function getData(ok: boolean): (string?, string?)
    if ok then
        return "data", nil
    end
    return nil, "failed"
end

local function use(ok: boolean)
    local data, err = getData(ok)
    local s: string = data -- expect-error
end

return use
