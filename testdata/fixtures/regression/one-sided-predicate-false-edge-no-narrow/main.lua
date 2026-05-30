local function is_number(value)
    return type(value) == "number"
end

-- The true edge narrows; the fallthrough (false edge) does not, so the argument
-- stays gradual and is not assignable to the tested type.
local function run(value: any): number
    if is_number(value) then
        return value
    end
    local n: number = value -- expect-error
    return n
end

return run
