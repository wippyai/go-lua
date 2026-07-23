type Config = {name: string, enabled: boolean}

local cfg: Config = {
    name = "prod",
    enabled = true,
}

local frozen: Config = table.freeze(cfg)
local is_frozen: boolean = table.isfrozen(cfg)
local name: string = frozen.name
local enabled: boolean = cfg.enabled

if is_frozen then
    local checked: string = cfg.name
end
