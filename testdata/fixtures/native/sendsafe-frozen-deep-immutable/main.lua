-- SEND SAFETY: a closed literal frozen before the send is deeply immutable, so
-- the exact identity is shared with no copy and the fact has no revoker.
type Config = {
    name: string,
    retries: number,
}

local pid: string = "collector"
local cfg: Config = { name = "ingest", retries = 3 }
table.freeze(cfg)

process.send(pid, "config", cfg)

return cfg.name
