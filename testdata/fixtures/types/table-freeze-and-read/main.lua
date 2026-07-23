type Config = { name: string, retries: number }
local cfg: Config = { name = "prod", retries = 3 }
local frozen: Config = table.freeze(cfg)
local frozen_flag: boolean = table.isfrozen(cfg)
local n: string = frozen.name
return n
