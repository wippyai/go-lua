local function read_id(value)
    return value.id
end

local function run(value: string | table)
    return read_id(value)
end

return run
