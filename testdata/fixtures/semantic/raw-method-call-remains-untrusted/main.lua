local raw: any = {
    run = function()
        return 1
    end,
}

local text: string = raw:run() -- expect-error

local callable: () -> string = raw.run -- expect-error

return "ok"
