-- The (value, err) convention correlates optional value slots with a trailing
-- optional error slot. A slot that is never nil keeps its value on either
-- path, and a slot that is never nil is not an error slot.
local function split(s: string): (string?, string)
    local i = string.find(s, ":", 1, true)
    if not i then
        return nil, s
    end
    return string.sub(s, 1, i - 1), string.sub(s, i + 1)
end

local head, rest = split("a:b")
if head then
    local r: string = rest
    print(head, r)
end

local function encode(n: integer): (integer, string?, error?)
    if n < 0 then
        return n, nil, errors.new("negative")
    end
    return n + 1, tostring(n), nil
end

local count, text, err = encode(3)
if err then
    local kept: integer = count
    print(kept, err:message())
else
    local t: string = text
    print(count, t)
end
