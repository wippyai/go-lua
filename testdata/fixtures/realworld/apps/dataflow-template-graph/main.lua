-- dataflow/src/template/graph.lua: the metatable is built before the
-- methods are added to the class table, and instances call those methods.
local default_deps = { node_reader = {} }

local template_graph = {}

local TemplateGraph = {}
local template_graph_mt = { __index = TemplateGraph }

function template_graph.new(deps)
    local instance = {
        nodes = {},
        edges = {},
        roots = {},
        _deps = deps or default_deps
    }
    return setmetatable(instance, template_graph_mt)
end

function TemplateGraph:is_empty()
    return next(self.nodes) == nil
end

function TemplateGraph:has_cycles()
    for node_id, _ in pairs(self.nodes) do
        if self.edges[node_id] == node_id then
            return true, "self edge on " .. tostring(node_id)
        end
    end
    return false, nil
end

function template_graph.build_for_node(parent_node, deps)
    deps = deps or default_deps
    if not parent_node then
        return nil, "missing parent node"
    end
    local graph = template_graph.new(deps)
    graph.nodes["a"] = { node_id = "a" }
    local has_cycles, cycle_desc = graph:has_cycles()
    if has_cycles then
        return nil, "cycle: " .. (cycle_desc :: string)
    end
    if graph:is_empty() then
        return nil, "empty"
    end
    return graph, nil
end

return template_graph
