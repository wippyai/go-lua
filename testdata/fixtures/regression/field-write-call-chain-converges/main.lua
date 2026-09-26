local M = {}

local function write_0(t)
    t.value = 1
end

local function write_1(t)
    write_0(t)
end

local function write_2(t)
    write_1(t)
end

local function write_3(t)
    write_2(t)
end

local function write_4(t)
    write_3(t)
end

local function write_5(t)
    write_4(t)
end

local function write_6(t)
    write_5(t)
end

local function write_7(t)
    write_6(t)
end

local function write_8(t)
    write_7(t)
end

local function write_9(t)
    write_8(t)
end

local function write_10(t)
    write_9(t)
end

local function write_11(t)
    write_10(t)
end

function M.run()
    local state = {}
    write_11(state)
    return state.value
end

return M
