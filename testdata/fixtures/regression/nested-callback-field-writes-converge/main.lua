local M = {}

local function each(fn: () -> ())
    fn()
end

function M.run()
    local ctx = { depth = 0 }
    each(function()
        ctx.l0 = ctx.depth
        each(function()
            ctx.l1 = ctx.depth
            each(function()
                ctx.l2 = ctx.depth
                each(function()
                    ctx.l3 = ctx.depth
                end)
            end)
        end)
    end)
    return ctx
end

return M
