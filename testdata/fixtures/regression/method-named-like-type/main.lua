type status = { code: integer }

local function run(conn: any): any
    local s, err = conn:status()
    if err then
        return nil
    end
    local rows = conn:query("select 1")
    return rows
end

local function describe(value: any): integer
    local shape = status(value)
    return shape.code
end

return { run = run, describe = describe }
