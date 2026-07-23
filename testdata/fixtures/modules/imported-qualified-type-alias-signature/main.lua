local atlassian_types = require("atlassian_types")
local client = require("client")

local conn: atlassian_types.Conn = { id = "conn-1" }
local result: atlassian_types.Result = client.describe(conn)
local id: string = result.conn.id
local wrong_id: number = result.conn.id

return id
