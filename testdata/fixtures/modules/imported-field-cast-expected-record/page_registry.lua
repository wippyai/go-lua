type PageInfo = {
    id: string,
    config_overrides: any?,
}

local M = {}

function M.find_all(): {PageInfo}
    return {
        {
            id = "p1",
            config_overrides = {
                theme = "dark",
            },
        },
    }
end

return M
