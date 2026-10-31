local M = { ACCESS = { READ = 1, WRITE = 2 } }
function M.validate_access(_crm_id: string, _access: integer): (boolean?, string?)
    return true, nil
end
return M
