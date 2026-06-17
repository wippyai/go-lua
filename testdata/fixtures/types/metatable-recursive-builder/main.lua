type Builder = {
    parts: {string},
    add: (self: Builder, p: string) -> Builder,
    build: (self: Builder) -> string,
}

local Builder = {}
Builder.__index = Builder

function Builder.add(self: Builder, p: string): Builder
    self.parts[#self.parts + 1] = p
    return self
end

function Builder.build(self: Builder): string
    local out = ""
    for _, p in ipairs(self.parts) do
        out = out .. p
    end
    return out
end

local function new(): Builder
    local self: Builder = {
        parts = {},
        add = Builder.add,
        build = Builder.build,
    }
    return setmetatable(self, Builder)
end

return new():add("a"):add("b"):build()
