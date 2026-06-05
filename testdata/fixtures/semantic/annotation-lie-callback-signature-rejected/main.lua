type Request = {
    id: string,
    attempt: number,
}

local raw_handler: any = function(req)
    return req.attempt + 1
end

local handler: (Request) -> string = raw_handler -- expect-error

local function dispatch(cb: (Request) -> string): string
    return cb({id = "r1", attempt = 1})
end

local result = dispatch(raw_handler) -- expect-error

return "ok"
