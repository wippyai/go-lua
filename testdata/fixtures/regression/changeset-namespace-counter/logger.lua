type Logger = {
    named: (self: Logger, name: string) -> Logger,
    debug: (self: Logger, msg: string, fields: any?) -> (),
    info: (self: Logger, msg: string, fields: any?) -> (),
    warn: (self: Logger, msg: string, fields: any?) -> (),
}

local logger: Logger = nil :: any
return logger
