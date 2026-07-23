local builder = require("builder")
local protocol = require("protocol")

local node = builder.make()
if node.kind == "group" then
    local first = node.children[1]
    if first and first.kind == "text" then
        local value: string = first.value
    end
end

local function inspect(candidate: protocol.Node): ()
    if candidate.kind == "text" then
        local children = candidate.children -- expect-error
        print(children)
    end
end

inspect(node)

return "ok"
