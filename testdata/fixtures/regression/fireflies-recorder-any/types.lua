local M = {}

M.PROVIDER = "fireflies"
M.CONNECTION_CLASS = "connection"
M.API_URL = "https://api.fireflies.ai/graphql"

type Result = {
    success: boolean,
    data: any?,
    error: string?,
    status_code: integer?,
}

type Conn = {
    component_id: string?,
    api_key: string,
}

M.Result = Result
M.Conn = Conn

return M
