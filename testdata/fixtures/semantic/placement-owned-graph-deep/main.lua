local pool = { slots = {} }
local node = { id = "a", meta = { tag = "x", child = { route = "deep" } } }
ownership.store(node, pool)
local r: string = node.meta.child.route
print(r)
