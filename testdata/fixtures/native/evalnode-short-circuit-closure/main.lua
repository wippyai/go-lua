-- Contract: the right-hand point of a short-circuit or publishes an evaluation
-- node carrying the closure construction it performs, not a noop.

type Config = { encode: ((string) -> string)? }

local function encoder(cfg: Config): (string) -> string
    return cfg.encode or function(s: string): string
        return s
    end
end

return encoder
