type Registry = {
    name: string,
    children: {[string]: Registry},
    add: (self: Registry, child: Registry) -> Registry,
    get: (self: Registry, name: string) -> Registry?,
}

local function new_registry(name: string): Registry
    local registry: Registry = {
        name = name,
        children = {},
        add = function(self: Registry, child: Registry): Registry
            self.children[child.name] = child
            return self
        end,
        get = function(self: Registry, name: string): Registry?
            return self.children[name]
        end,
    }
    return registry
end

local root = new_registry("root")
root:add(new_registry("child"))

local child = root:get("child")
if child then
    local name: string = child.name
end

local missing: Registry = root:get("missing") -- expect-error

return "ok"
