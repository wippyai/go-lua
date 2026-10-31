local function take(id: string?)
    return id
end

local function read(): any
    return nil
end

local function logical()
    local primary, err = read()
    if err or type(primary) ~= "table" then
        return nil
    end
    local id = type(primary.artifact) == "table" and primary.artifact.artifact_id or nil
    take(id)
    take(type(primary.artifact) == "table" and primary.artifact.artifact_id or nil)
    return id
end

local function branch(row: any)
    if type(row) == "table" then
        take(row.artifact_id)
        local id = row.artifact_id
        take(id)
    end
end

local function unresolved(row: unknown)
    if type(row) == "table" then
        take(row.artifact_id)
    end
end

return logical, branch, unresolved
