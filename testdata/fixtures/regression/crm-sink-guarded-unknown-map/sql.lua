type DB = {
    release: (self: DB) -> (boolean, string?),
    begin: (self: DB) -> (any, string?),
    type: (self: DB) -> (string, string?),
}
local sql = {}
function sql.get(name: string): (DB?, string?)
    if name == "missing" then return nil, "missing" end
    return ({release = function() return true, nil end, begin = function() return {}, nil end,
        type = function() return "sqlite", nil end} :: DB), nil
end
return sql
