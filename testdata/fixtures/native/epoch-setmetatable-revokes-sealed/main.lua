-- A sealed_table fact on a closed table literal is revoked by setmetatable: an installed
-- __index makes absent-key reads observable, so sealedness cannot survive meta.set.

local cfg = { host = "localhost", port = 8080 }
local host = cfg.host

local defaults = { scheme = "https" }
setmetatable(cfg, { __index = defaults })

return host .. tostring(cfg.port)
