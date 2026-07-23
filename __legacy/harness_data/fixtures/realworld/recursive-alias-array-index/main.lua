type Message = {
    _topic: string,
    topic: (self: Message) -> string,
}

local function make(): {Message}
    return {{
        _topic = "indexed",
        topic = function(self: Message): string
            return self._topic
        end,
    }}
end

-- make() returns the typed sequence {Message}: a typed sequence has unknown
-- runtime length, so make()[1] is Message? and calling :topic() on it without a
-- nil check is soundly rejected.
local topic: string = make()[1]:topic() -- expect-error: cannot call method on an optional value without a nil check
