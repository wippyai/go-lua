local resource = require("resource")

local function use_after_close()
    local conn = resource.connect()
    resource.close(conn)
    resource.query(conn)
end

local function alias_close_propagates()
    local conn = resource.connect()
    local alias = conn
    resource.close(alias)
    resource.query(conn)
end

local function double_commit()
    local conn = resource.connect()
    local tx = resource.begin(conn)
    resource.commit(tx)
    resource.commit(tx)
    resource.close(conn)
end

local function leak_on_some_path(flag)
    local conn = resource.connect()
    if flag then
        resource.close(conn)
    end
end

local function correct_usage()
    local conn = resource.connect()
    local tx = resource.begin(conn)
    resource.commit(tx)
    resource.close(conn)
end

local function opaque_handoff(writer: (any) -> ())
    local conn = resource.connect()
    writer(conn)
end

local function final_only_on_pcall_error()
    local conn = resource.connect()
    pcall(function()
        resource.close(conn)
        error("boom")
    end)
end

local function leak_on_pcall_error_path()
    local conn = resource.connect()
    pcall(function()
        error("boom")
    end)
end

local function final_on_pcall_normal_and_error_paths(flag)
    local conn = resource.connect()
    pcall(function()
        resource.close(conn)
        if flag then
            error("boom")
        end
    end)
end

local function query_unknown_state(conn)
    resource.query(conn)
end
