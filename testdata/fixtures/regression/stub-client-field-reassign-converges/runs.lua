local M = {}

M._client_new = function(): (any?, string?)
    return nil, "engine unavailable"
end

function M.launch(id: string): (any?, string?)
    local cli, err = M._client_new()
    if err then
        return nil, err
    end
    return cli, nil
end

return M
