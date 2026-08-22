-- The whole declared debug surface. Every reader answers an optional, so each
-- absent answer is ruled out before the value is used; setmetatable answers the
-- object it was given, and traceback always answers a string.
local function inspect(): string
    local info = debug.getinfo(1, "Sl")
    if info == nil then
        return debug.traceback("no frame", 1)
    end

    local where = info.source .. ":" .. tostring(info.currentline) ..
        "/" .. info.what .. "/" .. tostring(info.nups) ..
        "/" .. tostring(info.linedefined) .. "/" .. tostring(info.lastlinedefined) ..
        "/" .. type(info.func)
    local named = info.name
    if named ~= nil then
        where = where .. "/" .. named
    end
    return where .. "\n" .. debug.traceback("frame", 1)
end

local function locals(): string
    local name, value = debug.getlocal(1, 1)
    if name == nil then
        return "no local"
    end
    local replaced = debug.setlocal(1, 1, value)
    if replaced == nil then
        return name
    end
    return name .. "=" .. replaced
end

local function upvalues(): string
    local captured = "captured"
    local reader = function(): string return captured end

    local name, value = debug.getupvalue(reader, 1)
    if name == nil then
        return "no upvalue"
    end
    local replaced = debug.setupvalue(reader, 1, value)
    if replaced == nil then
        return name .. "/" .. reader()
    end
    return name .. "=" .. replaced .. "/" .. reader()
end

local function metatables(): string
    local subject = {id = "subject"}
    local shaped = debug.setmetatable(subject, {__tostring = function(): string return "shaped" end})
    local read = debug.getmetatable(shaped)
    if read == nil then
        return shaped.id
    end
    return shaped.id .. "/" .. type(read)
end

return {inspect = inspect, locals = locals, upvalues = upvalues, metatables = metatables}
