local M = {}
function M.get_connection(id: string)
    if id == "present" then
        return {
            user_profile = { provider_user_id = "user", username = "name", avatar_url = "url" },
            client_credentials = { client_id = "client" },
            provider_specific = { complex_nested = nil },
        }, nil
    end
    return nil, "missing connection"
end
return M
