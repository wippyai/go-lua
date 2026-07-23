local M = {}

function M.next_interval_run(expression: string, base: string?, now: string): string
    if base then
        return base
    end
    return now .. "+" .. expression
end

return M
