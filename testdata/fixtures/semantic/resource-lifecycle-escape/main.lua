local resource = require("resource")

local escaped_state = {}

local function returns_open_connection()
    local conn = resource.connect()
    return conn
end

local function stores_connection_in_table()
    local conn = resource.connect()
    escaped_state.conn = conn
end

local function leaky_receiver(conn)
end

local function passes_to_non_closing_function()
    local conn = resource.connect()
    leaky_receiver(conn)
end

local function closing_receiver(conn)
    resource.close(conn)
end

local function passes_to_closing_function()
    local conn = resource.connect()
    closing_receiver(conn)
end
