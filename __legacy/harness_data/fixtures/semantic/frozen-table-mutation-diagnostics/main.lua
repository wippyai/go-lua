type Child = { tag: string }
type Config = { name: string, child: Child }
local root: Config = { name = "prod", child = { tag = "old" } }
table.freeze(root)
root.name = "staging"
root.child.tag = "mutating child is ok"
root.child = { tag = "replacement" }

local dyn = { x = 1 }
local key = "x"
if table.isfrozen(dyn) then
    dyn[key] = 1
end

local flag: boolean = table.isfrozen({})
local one = { x = 0 }
if flag then table.freeze(one) end
one.x = 1

local both = { x = 0 }
if flag then table.freeze(both) else table.freeze(both) end
both.x = 1

local items = { "a" }
table.freeze(items)
table.insert(items, "b")
