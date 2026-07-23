local function make_async()
    local obj = {}
    coroutine.spawn(function()
        obj.get_value = function(self): number
            return 42
        end
    end)
    return obj
end

local async_obj = make_async()
local v: number = async_obj:get_value()
