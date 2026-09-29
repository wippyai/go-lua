local function run(params)
    params = params or {}
    local action = params.action
    if type(action) ~= "string" or action == "" then
        return nil
    end
    local s: string = action
    local n: number = action -- expect-error: cannot assign string to number
    return s
end

return run
