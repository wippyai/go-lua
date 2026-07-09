type Meta = {
    route: string,
    shard: string,
}

type Child = {
    id: string,
    meta: Meta,
}

type Item = {
    id: string,
    child: Child,
}

local M = {}
M.Meta = Meta
M.Child = Child
M.Item = Item

return M
