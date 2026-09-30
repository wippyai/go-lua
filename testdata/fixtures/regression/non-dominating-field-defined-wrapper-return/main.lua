local function run(flag: boolean)
    local M = {
        dep = {
            get = function()
                return nil
            end,
        },
    }

    function M.run()
        return M.dep.get()
    end

    if flag then
        M.dep = {
            get = function()
                return { answer = "ok" }
            end,
        }
    end

    local res = M.run()
    local answer: string = res.answer -- expect-error: cannot assign "ok"? to string
    return answer
end

return run
