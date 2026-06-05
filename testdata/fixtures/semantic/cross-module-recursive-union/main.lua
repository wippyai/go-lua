local builder = require("builder")

local node = builder.make()
if node.kind == "group" then
    local first = node.children[1]
    if first and first.kind == "text" then
        local value: string = first.value
    end
end

if node.kind == "text" then
    local children = node.children -- expect-error
end

return "ok"
