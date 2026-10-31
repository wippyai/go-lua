local json = {}
function json.encode(value: any): (string, error?) return "", nil end
function json.decode(str: string): (any, error?) return nil, nil end
return json
