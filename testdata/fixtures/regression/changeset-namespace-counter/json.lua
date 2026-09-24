local json = {}

function json.encode(value: any): (string?, string?)
    return tostring(value), nil
end

return json
