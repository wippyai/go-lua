type Config = {
    host: string,
    port: number,
}

local function parse_config(raw: string?): (Config?, string?)
    if raw == nil then
        return nil, "missing config"
    end
    return {host = "localhost", port = 8080}, nil
end

local function unchecked(raw: string?): string
    local cfg, err = parse_config(raw)
    local host: string = cfg.host
    return host
end

local function checked(raw: string?): (string?, string?)
    local cfg, err = parse_config(raw)
    if err then
        return nil, err
    end
    local host: string = cfg.host
    local port: number = cfg.port
    return host .. ":" .. tostring(port), nil
end

return checked, unchecked
