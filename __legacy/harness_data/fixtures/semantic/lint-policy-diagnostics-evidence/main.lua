local unused_local = 1

local value = 1
value = 2

local flag = true
if flag then
    if flag then
        value = value + 1
    end
end

local function exit_or_replace(flag: boolean): number
    local exit_value = 1
    if flag then
        return 0
    end
    exit_value = 2
    return exit_value
end

value = value + exit_or_replace(flag)

return value
