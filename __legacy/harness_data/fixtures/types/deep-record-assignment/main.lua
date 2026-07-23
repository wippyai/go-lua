type Config = { server: { host: string, port: number, tls: { enabled: boolean } } }
local cfg: Config = { server = { host = "h", port = 80, tls = { enabled = true } } }
local p: number = cfg.server.port
return p
