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
    tags: {[string]: string},
    child: Child,
}

type Box = {
    label: string,
    items: {[string]: Item},
}

local M = {}
M.Meta = Meta
M.Child = Child
M.Item = Item
M.Box = Box

return M
